// Package proxy implements an LLM API proxy that captures requests/responses
// and creates observability spans. Supports OpenAI, Google Gemini, and Anthropic (via Vertex AI).
//
// Usage:
//   client = OpenAI(base_url="http://localhost:8080/proxy/openai/v1")
//   client = anthropic.Anthropic(base_url="http://localhost:8080/proxy/anthropic")
//
// The proxy forwards requests transparently, captures the full exchange,
// extracts token usage, calculates cost, and stores as a trace.
package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"log/slog"

	"github.com/candelahq/candela/pkg/costcalc"
	"github.com/candelahq/candela/pkg/storage"
)

// SpanSubmitter is the interface for submitting spans to the processing pipeline.
type SpanSubmitter interface {
	SubmitBatch(spans []storage.Span)
}

// Provider defines an LLM API provider configuration.
type Provider struct {
	Name      string `yaml:"name"`      // "openai", "google", "anthropic"
	UpstreamURL string `yaml:"upstream"` // e.g. "https://api.openai.com"
}

// Proxy handles LLM API proxying with observability.
type Proxy struct {
	providers map[string]Provider
	submitter SpanSubmitter
	calc      *costcalc.Calculator
	client    *http.Client
	projectID string
}

// Config holds proxy configuration.
type Config struct {
	Providers []Provider `yaml:"providers"`
	ProjectID string     `yaml:"project_id"`
}

// DefaultProviders returns the standard LLM provider configurations.
// Anthropic uses Vertex AI endpoint — set VERTEX_REGION and GCP_PROJECT env vars.
func DefaultProviders() []Provider {
	return []Provider{
		{Name: "openai", UpstreamURL: "https://api.openai.com"},
		{Name: "google", UpstreamURL: "https://generativelanguage.googleapis.com"},
		// Anthropic via Vertex AI. Override upstream via config for your region/project:
		// https://{REGION}-aiplatform.googleapis.com/v1/projects/{PROJECT}/locations/{REGION}/publishers/anthropic/models
		{Name: "anthropic", UpstreamURL: "https://us-central1-aiplatform.googleapis.com"},
	}
}

// New creates a new LLM proxy.
func New(cfg Config, submitter SpanSubmitter, calc *costcalc.Calculator) *Proxy {
	providers := make(map[string]Provider)
	for _, p := range cfg.Providers {
		providers[p.Name] = p
	}

	return &Proxy{
		providers: providers,
		submitter: submitter,
		calc:      calc,
		projectID: cfg.ProjectID,
		client: &http.Client{
			Timeout: 5 * time.Minute, // LLM calls can be slow
		},
	}
}

// RegisterRoutes registers proxy routes on the given mux.
// Pattern: /proxy/{provider}/...
func (p *Proxy) RegisterRoutes(mux *http.ServeMux) {
	for name := range p.providers {
		prefix := "/proxy/" + name + "/"
		mux.HandleFunc(prefix, p.handleProxy)
		slog.Info("registered proxy route", "path", prefix)
	}
}

func (p *Proxy) handleProxy(w http.ResponseWriter, r *http.Request) {
	startTime := time.Now()

	// Extract provider from path: /proxy/{provider}/v1/...
	parts := strings.SplitN(strings.TrimPrefix(r.URL.Path, "/proxy/"), "/", 2)
	if len(parts) < 2 {
		http.Error(w, "invalid proxy path", http.StatusBadRequest)
		return
	}
	providerName := parts[0]
	upstreamPath := "/" + parts[1]

	provider, ok := p.providers[providerName]
	if !ok {
		http.Error(w, fmt.Sprintf("unknown provider: %s", providerName), http.StatusBadRequest)
		return
	}

	// Read the request body.
	reqBody, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "failed to read request body", http.StatusBadRequest)
		return
	}
	r.Body.Close()

	// Check if this is a streaming request.
	isStreaming := isStreamingRequest(providerName, reqBody)

	// Build the upstream request.
	upstreamURL := provider.UpstreamURL + upstreamPath
	if r.URL.RawQuery != "" {
		upstreamURL += "?" + r.URL.RawQuery
	}

	upstreamReq, err := http.NewRequestWithContext(r.Context(), r.Method, upstreamURL, bytes.NewReader(reqBody))
	if err != nil {
		http.Error(w, "failed to create upstream request", http.StatusInternalServerError)
		return
	}

	// Forward headers (auth, content-type, etc).
	forwardHeaders(r, upstreamReq, providerName)

	// Execute the upstream request.
	resp, err := p.client.Do(upstreamReq)
	if err != nil {
		http.Error(w, fmt.Sprintf("upstream error: %v", err), http.StatusBadGateway)
		return
	}
	ttfb := time.Since(startTime)
	defer resp.Body.Close()

	if isStreaming && resp.StatusCode == http.StatusOK {
		p.handleStreamingResponse(w, r, resp, provider, reqBody, startTime, ttfb)
	} else {
		p.handleStandardResponse(w, r, resp, provider, reqBody, startTime, ttfb)
	}
}

func (p *Proxy) handleStandardResponse(
	w http.ResponseWriter, r *http.Request,
	resp *http.Response, provider Provider,
	reqBody []byte, startTime time.Time,
	ttfb time.Duration,
) {
	// Read the full response.
	respBody, err := io.ReadAll(resp.Body)
	if err != nil {
		http.Error(w, "failed to read upstream response", http.StatusBadGateway)
		return
	}

	endTime := time.Now()

	// Forward response headers.
	for k, vv := range resp.Header {
		for _, v := range vv {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	w.Write(respBody)

	// Create observability span (async — don't block the response).
	go p.createSpan(r.Context(), provider, reqBody, respBody, startTime, endTime, resp.StatusCode, ttfb)
}

func (p *Proxy) handleStreamingResponse(
	w http.ResponseWriter, r *http.Request,
	resp *http.Response, provider Provider,
	reqBody []byte, startTime time.Time,
	ttfb time.Duration,
) {
	flusher, ok := w.(http.Flusher)
	if !ok {
		http.Error(w, "streaming not supported", http.StatusInternalServerError)
		return
	}

	// Forward response headers for SSE.
	for k, vv := range resp.Header {
		for _, v := range vv {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)

	// Tee the stream: forward to client AND buffer for observability.
	var streamBuffer bytes.Buffer
	buf := make([]byte, 4096)
	var ttft time.Duration
	isFirstChunk := true

	for {
		n, err := resp.Body.Read(buf)
		if n > 0 {
			if isFirstChunk {
				ttft = time.Since(startTime)
				isFirstChunk = false
			}
			// Forward to client.
			w.Write(buf[:n])
			flusher.Flush()

			// Buffer for span creation.
			streamBuffer.Write(buf[:n])
		}
		if err != nil {
			break
		}
	}

	endTime := time.Now()

	// Parse the accumulated stream to extract usage data.
	go p.createStreamingSpan(r.Context(), provider, reqBody, streamBuffer.Bytes(), startTime, endTime, ttfb, ttft)
}

func (p *Proxy) createSpan(
	ctx context.Context, provider Provider,
	reqBody, respBody []byte,
	startTime, endTime time.Time,
	statusCode int,
	ttfb time.Duration,
) {
	model, inputContent := extractRequestInfo(provider.Name, reqBody)
	outputContent, inputTokens, outputTokens := extractResponseInfo(provider.Name, respBody)
	totalTokens := inputTokens + outputTokens

	cost := p.calc.Calculate(provider.Name, model, inputTokens, outputTokens)

	status := storage.SpanStatusOK
	if statusCode >= 400 {
		status = storage.SpanStatusError
	}

	span := storage.Span{
		SpanID:    generateSpanID(),
		TraceID:   generateTraceID(),
		Name:      fmt.Sprintf("%s.chat", provider.Name),
		Kind:      storage.SpanKindLLM,
		Status:    status,
		StartTime: startTime,
		EndTime:   endTime,
		Duration:  endTime.Sub(startTime),
		ProjectID: p.projectID,
		GenAI: &storage.GenAIAttributes{
			Model:         model,
			Provider:      provider.Name,
			InputTokens:   inputTokens,
			OutputTokens:  outputTokens,
			TotalTokens:   totalTokens,
			CostUSD:       cost,
			InputContent:  truncate(inputContent, 10000),
			OutputContent: truncate(outputContent, 10000),
		},
		Attributes: map[string]string{
			"proxy.upstream": provider.UpstreamURL,
			"http.status":    fmt.Sprintf("%d", statusCode),
			"http.ttfb_ms":   fmt.Sprintf("%d", ttfb.Milliseconds()),
		},
	}

	p.submitter.SubmitBatch([]storage.Span{span})
	slog.Debug("proxied LLM call",
		"provider", provider.Name,
		"model", model,
		"tokens", totalTokens,
		"cost_usd", cost,
		"latency", endTime.Sub(startTime),
	)
}

func (p *Proxy) createStreamingSpan(
	ctx context.Context, provider Provider,
	reqBody, streamData []byte,
	startTime, endTime time.Time,
	ttfb time.Duration,
	ttft time.Duration,
) {
	model, inputContent := extractRequestInfo(provider.Name, reqBody)
	outputContent, inputTokens, outputTokens := extractStreamingUsage(provider.Name, streamData)
	totalTokens := inputTokens + outputTokens

	cost := p.calc.Calculate(provider.Name, model, inputTokens, outputTokens)

	span := storage.Span{
		SpanID:    generateSpanID(),
		TraceID:   generateTraceID(),
		Name:      fmt.Sprintf("%s.chat.stream", provider.Name),
		Kind:      storage.SpanKindLLM,
		Status:    storage.SpanStatusOK,
		StartTime: startTime,
		EndTime:   endTime,
		Duration:  endTime.Sub(startTime),
		ProjectID: p.projectID,
		GenAI: &storage.GenAIAttributes{
			Model:         model,
			Provider:      provider.Name,
			InputTokens:   inputTokens,
			OutputTokens:  outputTokens,
			TotalTokens:   totalTokens,
			CostUSD:       cost,
			InputContent:  truncate(inputContent, 10000),
			OutputContent: truncate(outputContent, 10000),
		},
		Attributes: map[string]string{
			"proxy.upstream":  provider.UpstreamURL,
			"proxy.streaming": "true",
			"http.ttfb_ms":    fmt.Sprintf("%d", ttfb.Milliseconds()),
			"llm.ttft_ms":     fmt.Sprintf("%d", ttft.Milliseconds()),
		},
	}

	p.submitter.SubmitBatch([]storage.Span{span})
	slog.Debug("proxied streaming LLM call",
		"provider", provider.Name,
		"model", model,
		"tokens", totalTokens,
		"cost_usd", cost,
		"latency", endTime.Sub(startTime),
	)
}

// --- Header forwarding ---

func forwardHeaders(src *http.Request, dst *http.Request, provider string) {
	// Always forward these.
	for _, h := range []string{"Authorization", "Content-Type", "Accept"} {
		if v := src.Header.Get(h); v != "" {
			dst.Header.Set(h, v)
		}
	}

	// Provider-specific headers.
	switch provider {
	case "openai":
		// OpenAI uses Authorization: Bearer — already forwarded above.
	case "anthropic":
		// Anthropic via Vertex AI uses Authorization: Bearer (ADC token).
		// Direct API uses X-Api-Key. Forward both for flexibility.
		for _, h := range []string{"X-Api-Key", "Anthropic-Version"} {
			if v := src.Header.Get(h); v != "" {
				dst.Header.Set(h, v)
			}
		}
	case "google":
		// Google uses API key in query params or Authorization header — both forwarded.
	}
}

// --- Request parsing ---

func isStreamingRequest(provider string, body []byte) bool {
	var req map[string]interface{}
	if err := json.Unmarshal(body, &req); err != nil {
		return false
	}

	switch provider {
	case "openai":
		if v, ok := req["stream"].(bool); ok {
			return v
		}
	case "anthropic":
		if v, ok := req["stream"].(bool); ok {
			return v
		}
	case "google":
		// Google uses a different endpoint for streaming, not a body param.
		return false
	}
	return false
}

func extractRequestInfo(provider string, body []byte) (model, content string) {
	var req map[string]interface{}
	if err := json.Unmarshal(body, &req); err != nil {
		return "", ""
	}

	switch provider {
	case "openai":
		model, _ = req["model"].(string)
		if messages, ok := req["messages"].([]interface{}); ok {
			b, _ := json.Marshal(messages)
			content = string(b)
		}
	case "anthropic":
		model, _ = req["model"].(string)
		if messages, ok := req["messages"].([]interface{}); ok {
			b, _ := json.Marshal(messages)
			content = string(b)
		}
	case "google":
		// Model is in the URL path for Google.
		if contents, ok := req["contents"].([]interface{}); ok {
			b, _ := json.Marshal(contents)
			content = string(b)
		}
	}
	return
}

func extractResponseInfo(provider string, body []byte) (content string, inputTokens, outputTokens int64) {
	var resp map[string]interface{}
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", 0, 0
	}

	switch provider {
	case "openai":
		if usage, ok := resp["usage"].(map[string]interface{}); ok {
			inputTokens = toInt64(usage["prompt_tokens"])
			outputTokens = toInt64(usage["completion_tokens"])
		}
		if choices, ok := resp["choices"].([]interface{}); ok && len(choices) > 0 {
			if choice, ok := choices[0].(map[string]interface{}); ok {
				if msg, ok := choice["message"].(map[string]interface{}); ok {
					content, _ = msg["content"].(string)
				}
			}
		}
	case "anthropic":
		// Works for both direct Anthropic API and Vertex AI Anthropic.
		if usage, ok := resp["usage"].(map[string]interface{}); ok {
			inputTokens = toInt64(usage["input_tokens"])
			outputTokens = toInt64(usage["output_tokens"])
		}
		if contentArr, ok := resp["content"].([]interface{}); ok && len(contentArr) > 0 {
			if block, ok := contentArr[0].(map[string]interface{}); ok {
				content, _ = block["text"].(string)
			}
		}
	case "google":
		if meta, ok := resp["usageMetadata"].(map[string]interface{}); ok {
			inputTokens = toInt64(meta["promptTokenCount"])
			outputTokens = toInt64(meta["candidatesTokenCount"])
		}
		if candidates, ok := resp["candidates"].([]interface{}); ok && len(candidates) > 0 {
			if c, ok := candidates[0].(map[string]interface{}); ok {
				if cont, ok := c["content"].(map[string]interface{}); ok {
					if parts, ok := cont["parts"].([]interface{}); ok && len(parts) > 0 {
						if part, ok := parts[0].(map[string]interface{}); ok {
							content, _ = part["text"].(string)
						}
					}
				}
			}
		}
	}
	return
}

func extractStreamingUsage(provider string, data []byte) (content string, inputTokens, outputTokens int64) {
	// Parse SSE data lines to find the final usage chunk.
	var contentBuilder strings.Builder

	lines := strings.Split(string(data), "\n")
	for _, line := range lines {
		line = strings.TrimSpace(line)
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		payload := strings.TrimPrefix(line, "data: ")
		if payload == "[DONE]" {
			continue
		}

		var chunk map[string]interface{}
		if err := json.Unmarshal([]byte(payload), &chunk); err != nil {
			continue
		}

		switch provider {
		case "openai":
			if choices, ok := chunk["choices"].([]interface{}); ok && len(choices) > 0 {
				if choice, ok := choices[0].(map[string]interface{}); ok {
					if delta, ok := choice["delta"].(map[string]interface{}); ok {
						if c, ok := delta["content"].(string); ok {
							contentBuilder.WriteString(c)
						}
					}
				}
			}
			if usage, ok := chunk["usage"].(map[string]interface{}); ok {
				inputTokens = toInt64(usage["prompt_tokens"])
				outputTokens = toInt64(usage["completion_tokens"])
			}
		case "anthropic":
			if delta, ok := chunk["delta"].(map[string]interface{}); ok {
				if text, ok := delta["text"].(string); ok {
					contentBuilder.WriteString(text)
				}
			}
			if usage, ok := chunk["usage"].(map[string]interface{}); ok {
				if v := toInt64(usage["output_tokens"]); v > 0 {
					outputTokens = v
				}
			}
			if msg, ok := chunk["message"].(map[string]interface{}); ok {
				if usage, ok := msg["usage"].(map[string]interface{}); ok {
					inputTokens = toInt64(usage["input_tokens"])
				}
			}
		}
	}

	content = contentBuilder.String()
	return
}

// --- Helpers ---

func toInt64(v interface{}) int64 {
	switch n := v.(type) {
	case float64:
		return int64(n)
	case int64:
		return n
	case json.Number:
		i, _ := n.Int64()
		return i
	}
	return 0
}

func truncate(s string, maxLen int) string {
	if len(s) <= maxLen {
		return s
	}
	return s[:maxLen] + "...[truncated]"
}

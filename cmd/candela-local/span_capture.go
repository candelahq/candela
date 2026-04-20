package main

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"github.com/candelahq/candela/pkg/processor"
	"github.com/candelahq/candela/pkg/storage"
)

// spanCapture wraps an HTTP handler (typically the local proxy) and captures
// request/response data to emit observability spans via the span processor.
// Handles both streaming (SSE) and non-streaming OpenAI-format responses.
type spanCapture struct {
	next http.Handler
	proc *processor.SpanProcessor
}

// newSpanCapture creates a span-capturing middleware.
func newSpanCapture(next http.Handler, proc *processor.SpanProcessor) http.Handler {
	if proc == nil {
		return next // no processor → passthrough
	}
	return &spanCapture{next: next, proc: proc}
}

func (s *spanCapture) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	// Only capture chat completions.
	if r.URL.Path != "/v1/chat/completions" || r.Method != http.MethodPost {
		s.next.ServeHTTP(w, r)
		return
	}

	start := time.Now()

	// Read and replay request body.
	reqBody, err := io.ReadAll(r.Body)
	if err != nil {
		s.next.ServeHTTP(w, r)
		return
	}
	_ = r.Body.Close()
	r.Body = io.NopCloser(bytes.NewReader(reqBody))
	r.ContentLength = int64(len(reqBody))

	// Parse request for model name.
	var chatReq struct {
		Model    string `json:"model"`
		Stream   *bool  `json:"stream"`
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
	}
	_ = json.Unmarshal(reqBody, &chatReq)

	// Capture response.
	rec := &responseCapture{ResponseWriter: w}
	s.next.ServeHTTP(rec, r)

	duration := time.Since(start)

	// Build the span asynchronously to not block the response.
	go s.buildSpan(chatReq.Model, chatReq.Stream, reqBody, rec.body.Bytes(), rec.statusCode, start, duration)
}

func (s *spanCapture) buildSpan(model string, stream *bool, reqBody, respBody []byte, statusCode int, start time.Time, duration time.Duration) {
	var inputTokens, outputTokens, totalTokens int64

	isStreaming := stream != nil && *stream

	if isStreaming {
		// Parse SSE stream — usage is in the final data chunk.
		inputTokens, outputTokens, totalTokens = parseSSEUsage(respBody)
	} else {
		// Parse JSON response for usage.
		var resp struct {
			Usage struct {
				PromptTokens     int64 `json:"prompt_tokens"`
				CompletionTokens int64 `json:"completion_tokens"`
				TotalTokens      int64 `json:"total_tokens"`
			} `json:"usage"`
		}
		if err := json.Unmarshal(respBody, &resp); err == nil {
			inputTokens = resp.Usage.PromptTokens
			outputTokens = resp.Usage.CompletionTokens
			totalTokens = resp.Usage.TotalTokens
		}
	}

	if totalTokens == 0 {
		totalTokens = inputTokens + outputTokens
	}

	// Build input content summary.
	var inputContent string
	var chatReq struct {
		Messages []struct {
			Role    string `json:"role"`
			Content string `json:"content"`
		} `json:"messages"`
	}
	if err := json.Unmarshal(reqBody, &chatReq); err == nil && len(chatReq.Messages) > 0 {
		last := chatReq.Messages[len(chatReq.Messages)-1]
		inputContent = last.Content
		if len(inputContent) > 500 {
			inputContent = inputContent[:500] + "..."
		}
	}

	spanStatus := storage.SpanStatusOK
	if statusCode >= 400 {
		spanStatus = storage.SpanStatusError
	}

	span := storage.Span{
		SpanID:    generateID(),
		TraceID:   generateID(),
		Name:      "chat " + model,
		Kind:      storage.SpanKindLLM,
		Status:    spanStatus,
		StartTime: start,
		EndTime:   start.Add(duration),
		Duration:  duration,
		ProjectID: "local",
		GenAI: &storage.GenAIAttributes{
			Model:        model,
			Provider:     "local",
			InputTokens:  inputTokens,
			OutputTokens: outputTokens,
			TotalTokens:  totalTokens,
			InputContent: inputContent,
		},
	}

	s.proc.Submit(span)
	slog.Debug("span captured", "model", model, "tokens", totalTokens, "duration", duration.Round(time.Millisecond))
}

// parseSSEUsage extracts token usage from the final SSE data chunk.
// Ollama and OpenAI-compatible servers include usage in the last chunk.
func parseSSEUsage(body []byte) (input, output, total int64) {
	lines := strings.Split(string(body), "\n")
	// Walk backwards to find the last data line with usage.
	for i := len(lines) - 1; i >= 0; i-- {
		line := strings.TrimSpace(lines[i])
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		data := strings.TrimPrefix(line, "data: ")
		if data == "[DONE]" {
			continue
		}
		var chunk struct {
			Usage struct {
				PromptTokens     int64 `json:"prompt_tokens"`
				CompletionTokens int64 `json:"completion_tokens"`
				TotalTokens      int64 `json:"total_tokens"`
			} `json:"usage"`
		}
		if err := json.Unmarshal([]byte(data), &chunk); err == nil && chunk.Usage.TotalTokens > 0 {
			return chunk.Usage.PromptTokens, chunk.Usage.CompletionTokens, chunk.Usage.TotalTokens
		}
	}
	return 0, 0, 0
}

// responseCapture wraps ResponseWriter to buffer the response body.
type responseCapture struct {
	http.ResponseWriter
	body       bytes.Buffer
	statusCode int
}

func (r *responseCapture) WriteHeader(code int) {
	r.statusCode = code
	r.ResponseWriter.WriteHeader(code)
}

func (r *responseCapture) Write(b []byte) (int, error) {
	r.body.Write(b) // buffer for span extraction
	return r.ResponseWriter.Write(b)
}

func (r *responseCapture) Flush() {
	if f, ok := r.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// generateID returns a random 16-byte hex string for span/trace IDs.
func generateID() string {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	return hex.EncodeToString(b)
}

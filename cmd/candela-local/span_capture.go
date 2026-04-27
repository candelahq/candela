package main

import (
	"bytes"
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"io"
	"log/slog"
	"net/http"
	"time"

	"github.com/candelahq/candela/pkg/processor"
	"github.com/candelahq/candela/pkg/session"
	"github.com/candelahq/candela/pkg/storage"
)

// spanCapture wraps an HTTP handler (typically the local proxy) and captures
// request/response data to emit observability spans via the span processor.
// Handles both streaming (SSE) and non-streaming OpenAI-format responses.
type spanCapture struct {
	next     http.Handler
	proc     *processor.SpanProcessor
	resolver session.SessionResolver
}

// newSpanCapture creates a span-capturing middleware.
func newSpanCapture(next http.Handler, proc *processor.SpanProcessor, resolver session.SessionResolver) http.Handler {
	if proc == nil {
		return next // no processor → passthrough
	}
	return &spanCapture{next: next, proc: proc, resolver: resolver}
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

	// Parse request for model name, input content, and raw messages.
	var chatReq struct {
		Model    string          `json:"model"`
		Stream   *bool           `json:"stream"`
		Messages json.RawMessage `json:"messages"`
	}
	_ = json.Unmarshal(reqBody, &chatReq)

	// Decode messages for input content extraction.
	var msgs []struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}
	_ = json.Unmarshal(chatReq.Messages, &msgs)

	// Extract input content now to avoid re-parsing later.
	var inputContent string
	if len(msgs) > 0 {
		last := msgs[len(msgs)-1]
		inputContent = last.Content
		if runeCount := len([]rune(inputContent)); runeCount > 500 {
			runes := []rune(inputContent)
			inputContent = string(runes[:500]) + "..."
		}
	}

	// Resolve session ID.
	var sessionID string
	if s.resolver != nil {
		sessionID = s.resolver.Resolve(session.SessionInfo{
			UserID:   r.Header.Get("X-User-Id"),
			Model:    chatReq.Model,
			Messages: chatReq.Messages, // pass raw JSON directly, no re-marshal
			Headers:  r.Header,
		})
		// Inject header so downstream (lm_handler → remote proxy) can see it.
		if sessionID != "" {
			r.Header.Set("X-Session-Id", sessionID)
		}
	}

	// Capture response.
	rec := &responseCapture{ResponseWriter: w}
	s.next.ServeHTTP(rec, r)

	duration := time.Since(start)

	// Build the span asynchronously to not block the response.
	go s.buildSpan(chatReq.Model, chatReq.Stream, inputContent, rec.body.Bytes(), rec.statusCode, start, duration, sessionID)
}

func (s *spanCapture) buildSpan(model string, stream *bool, inputContent string, respBody []byte, statusCode int, start time.Time, duration time.Duration, sessionID string) {
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

	spanStatus := storage.SpanStatusOK
	if statusCode >= 400 {
		spanStatus = storage.SpanStatusError
	}

	span := storage.Span{
		SpanID:    generateID(8),
		TraceID:   generateID(16),
		Name:      "chat " + model,
		Kind:      storage.SpanKindLLM,
		Status:    spanStatus,
		StartTime: start,
		EndTime:   start.Add(duration),
		Duration:  duration,
		ProjectID: "local",
		SessionID: sessionID,
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
// Iterates backwards using bytes.LastIndex to avoid allocating slices
// for the entire response body.
func parseSSEUsage(body []byte) (input, output, total int64) {
	prefix := []byte("data: ")
	done := []byte("[DONE]")

	// Walk backwards through newline-delimited lines.
	for end := len(body); end > 0; {
		nl := bytes.LastIndexByte(body[:end], '\n')
		var line []byte
		if nl >= 0 {
			line = body[nl+1 : end]
			end = nl
		} else {
			line = body[:end]
			end = 0
		}
		line = bytes.TrimSpace(line)
		if !bytes.HasPrefix(line, prefix) {
			continue
		}
		data := bytes.TrimPrefix(line, prefix)
		if bytes.Equal(data, done) {
			continue
		}
		var chunk struct {
			Usage struct {
				PromptTokens     int64 `json:"prompt_tokens"`
				CompletionTokens int64 `json:"completion_tokens"`
				TotalTokens      int64 `json:"total_tokens"`
			} `json:"usage"`
		}
		if err := json.Unmarshal(data, &chunk); err == nil && chunk.Usage.TotalTokens > 0 {
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

// generateID returns a random hex string of the given byte size.
// Use 8 for OTel SpanIDs (16 hex chars) and 16 for TraceIDs (32 hex chars).
func generateID(size int) string {
	b := make([]byte, size)
	if _, err := rand.Read(b); err != nil {
		// crypto/rand should never fail on supported platforms.
		panic("crypto/rand failed: " + err.Error())
	}
	return hex.EncodeToString(b)
}

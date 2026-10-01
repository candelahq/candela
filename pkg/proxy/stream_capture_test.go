package proxy

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/candelahq/candela/pkg/costcalc"
	"github.com/candelahq/candela/pkg/storage"
)

type captureTestUserStore struct {
	budgetUserStore
	mu            sync.Mutex
	remainingUSD  float64
	deductedCalls []deductCall
}

type deductCall struct {
	userID  string
	costUSD float64
	tokens  int64
}

func (s *captureTestUserStore) CheckBudget(_ context.Context, _ string, estimatedCostUSD float64) (*storage.BudgetCheckResult, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	allowed := s.remainingUSD >= estimatedCostUSD
	return &storage.BudgetCheckResult{
		Allowed:      allowed,
		RemainingUSD: s.remainingUSD,
	}, nil
}

func (s *captureTestUserStore) DeductSpend(_ context.Context, userID string, costUSD float64, tokens int64) error {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.deductedCalls = append(s.deductedCalls, deductCall{userID: userID, costUSD: costUSD, tokens: tokens})
	s.remainingUSD -= costUSD
	return nil
}

// TestStreamCapture_Over10MB_OpenAI_NoDataLoss verifies that when an OpenAI
// streaming response exceeds the 10MB capture limit, the final usage chunk
// is still preserved in the rolling tail buffer, ensuring full token and cost
// attribution without revenue leakage (#525).
func TestStreamCapture_Over10MB_OpenAI_NoDataLoss(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher, _ := w.(http.Flusher)

		// Stream 11MB of text deltas (110 chunks × 100KB)
		chunk100KB := bytes.Repeat([]byte("a"), 100*1024)
		for i := 0; i < 110; i++ {
			msg, _ := json.Marshal(map[string]interface{}{
				"choices": []interface{}{
					map[string]interface{}{
						"delta": map[string]interface{}{"content": string(chunk100KB)},
					},
				},
			})
			_, _ = fmt.Fprintf(w, "data: %s\n\n", msg)
			flusher.Flush()
		}

		// Final usage chunk with exact token counts
		finalMsg, _ := json.Marshal(map[string]interface{}{
			"choices": []interface{}{
				map[string]interface{}{
					"delta":         map[string]interface{}{},
					"finish_reason": "stop",
				},
			},
			"usage": map[string]interface{}{
				"prompt_tokens":     125,
				"completion_tokens": 2750000,
				"total_tokens":      2750125,
				"prompt_tokens_details": map[string]interface{}{
					"cached_tokens": 50,
				},
			},
		})
		_, _ = fmt.Fprintf(w, "data: %s\n\n", finalMsg)
		_, _ = fmt.Fprintf(w, "data: [DONE]\n\n")
		flusher.Flush()
	}))
	defer upstream.Close()

	submitter := &mockSubmitter{}
	calc := costcalc.New()
	p, _ := New(Config{
		Providers: []Provider{{Name: "openai", UpstreamURL: upstream.URL}},
		ProjectID: "test",
	}, submitter, calc)

	store := &captureTestUserStore{remainingUSD: 100.0}
	p.SetUserStore(store)

	mux := http.NewServeMux()
	p.RegisterRoutes(mux)
	srv := httptest.NewServer(withTestAuth(mux))
	defer srv.Close()

	reqBody := `{"model":"gpt-4o","stream":true,"messages":[{"role":"user","content":"generate big text"}]}`
	req, _ := http.NewRequest("POST", srv.URL+"/proxy/openai/v1/chat/completions", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer tok")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, want 200; body = %s", resp.StatusCode, body)
	}

	// Consume entire response stream
	readBytes, err := io.Copy(io.Discard, resp.Body)
	if err != nil {
		t.Fatalf("failed reading response body: %v", err)
	}
	if readBytes < 10*1024*1024 {
		t.Fatalf("streamed bytes = %d, want >= 10MB", readBytes)
	}

	// Wait for span creation
	var spans []storage.Span
	for i := 0; i < 50; i++ {
		spans = submitter.getSpans()
		if len(spans) > 0 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if len(spans) == 0 {
		t.Fatal("expected span to be submitted, got none")
	}

	span := spans[0]
	if span.GenAI == nil {
		t.Fatal("span.GenAI is nil")
	}

	if span.GenAI.OutputTokens != 2750000 {
		t.Errorf("span OutputTokens = %d, want 2750000 (final chunk preserved across >10MB stream)", span.GenAI.OutputTokens)
	}
	// 125 prompt tokens with 50 cached tokens (50% discount) normalizes to 100
	if span.GenAI.InputTokens != 100 {
		t.Errorf("span InputTokens = %d, want 100 (normalized with 50 cached tokens)", span.GenAI.InputTokens)
	}
	if span.GenAI.CostUSD <= 0 {
		t.Errorf("span CostUSD = %f, want > 0", span.GenAI.CostUSD)
	}
	if span.Attributes["proxy.stream_truncated"] != "true" {
		t.Errorf("expected proxy.stream_truncated = true, got %s", span.Attributes["proxy.stream_truncated"])
	}
	if span.Attributes["proxy.streaming"] != "true" {
		t.Errorf("expected proxy.streaming = true, got %s", span.Attributes["proxy.streaming"])
	}

	// Verify budget deduction was recorded
	store.mu.Lock()
	defer store.mu.Unlock()
	if len(store.deductedCalls) == 0 {
		t.Fatal("expected DeductSpend to be called, got 0 calls")
	}
	if store.deductedCalls[0].costUSD <= 0 {
		t.Errorf("deducted costUSD = %f, want > 0", store.deductedCalls[0].costUSD)
	}
}

// TestStreamCapture_Over10MB_Anthropic_NoDataLoss verifies that Anthropic streaming
// preserves both message_start input tokens (from head buffer) and message_delta
// output tokens (from tail buffer) on responses >10MB (#525).
func TestStreamCapture_Over10MB_Anthropic_NoDataLoss(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher, _ := w.(http.Flusher)

		// 1. Initial chunk: message_start with input tokens
		startMsg := `data: {"type":"message_start","message":{"id":"msg_1","type":"message","role":"assistant","model":"claude-sonnet-4-20250514","usage":{"input_tokens":200,"output_tokens":0}}}` + "\n\n"
		_, _ = fmt.Fprint(w, startMsg)
		flusher.Flush()

		// 2. Stream >10MB of content_block_delta chunks
		chunk100KB := bytes.Repeat([]byte("b"), 100*1024)
		for i := 0; i < 110; i++ {
			msg, _ := json.Marshal(map[string]interface{}{
				"type":  "content_block_delta",
				"index": 0,
				"delta": map[string]interface{}{
					"type": "text_delta",
					"text": string(chunk100KB),
				},
			})
			_, _ = fmt.Fprintf(w, "data: %s\n\n", msg)
			flusher.Flush()
		}

		// 3. Final chunk: message_delta with output tokens
		deltaMsg := `data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":3000000}}` + "\n\n"
		_, _ = fmt.Fprint(w, deltaMsg)
		_, _ = fmt.Fprint(w, "data: {\"type\":\"message_stop\"}\n\n")
		flusher.Flush()
	}))
	defer upstream.Close()

	submitter := &mockSubmitter{}
	calc := costcalc.New()
	p, _ := New(Config{
		Providers: []Provider{{Name: "anthropic-direct", UpstreamURL: upstream.URL}},
		ProjectID: "test",
	}, submitter, calc)

	store := &captureTestUserStore{remainingUSD: 100.0}
	p.SetUserStore(store)

	mux := http.NewServeMux()
	p.RegisterRoutes(mux)
	srv := httptest.NewServer(withTestAuth(mux))
	defer srv.Close()

	reqBody := `{"model":"claude-sonnet-4-20250514","stream":true,"messages":[{"role":"user","content":"hi"}]}`
	req, _ := http.NewRequest("POST", srv.URL+"/proxy/anthropic-direct/v1/messages", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer tok")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, want 200; body = %s", resp.StatusCode, body)
	}

	_, err = io.Copy(io.Discard, resp.Body)
	if err != nil {
		t.Fatalf("failed reading response body: %v", err)
	}

	var spans []storage.Span
	for i := 0; i < 50; i++ {
		spans = submitter.getSpans()
		if len(spans) > 0 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if len(spans) == 0 {
		t.Fatal("expected span to be submitted, got none")
	}

	span := spans[0]
	if span.GenAI == nil {
		t.Fatal("span.GenAI is nil")
	}

	if span.GenAI.InputTokens != 200 {
		t.Errorf("span InputTokens = %d, want 200 (from head buffer message_start)", span.GenAI.InputTokens)
	}
	if span.GenAI.OutputTokens != 3000000 {
		t.Errorf("span OutputTokens = %d, want 3000000 (from tail buffer message_delta)", span.GenAI.OutputTokens)
	}
	if span.GenAI.CostUSD <= 0 {
		t.Errorf("span CostUSD = %f, want > 0", span.GenAI.CostUSD)
	}
	if span.Attributes["proxy.stream_truncated"] != "true" {
		t.Errorf("expected proxy.stream_truncated = true, got %s", span.Attributes["proxy.stream_truncated"])
	}
}

// TestStreamCapture_Over10MB_Google_NoDataLoss verifies that Google/Gemini streaming
// preserves usageMetadata in the final chunk on responses >10MB (#525).
func TestStreamCapture_Over10MB_Google_NoDataLoss(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		flusher, _ := w.(http.Flusher)

		// Stream >10MB of JSON array objects
		chunk100KB := bytes.Repeat([]byte("c"), 100*1024)
		_, _ = fmt.Fprint(w, "[\n")
		for i := 0; i < 110; i++ {
			chunkJSON, _ := json.Marshal(map[string]interface{}{
				"candidates": []interface{}{
					map[string]interface{}{
						"content": map[string]interface{}{
							"parts": []interface{}{
								map[string]interface{}{"text": string(chunk100KB)},
							},
						},
					},
				},
			})
			_, _ = fmt.Fprintf(w, "%s,\n", chunkJSON)
			flusher.Flush()
		}

		// Final chunk with usageMetadata
		finalJSON, _ := json.Marshal(map[string]interface{}{
			"candidates": []interface{}{
				map[string]interface{}{
					"content": map[string]interface{}{
						"parts": []interface{}{
							map[string]interface{}{"text": "done"},
						},
					},
				},
			},
			"usageMetadata": map[string]interface{}{
				"promptTokenCount":        150,
				"candidatesTokenCount":    2400000,
				"totalTokenCount":         2400150,
				"cachedContentTokenCount": 40,
			},
			"modelVersion": "gemini-2.5-flash",
		})
		_, _ = fmt.Fprintf(w, "%s\n]", finalJSON)
		flusher.Flush()
	}))
	defer upstream.Close()

	submitter := &mockSubmitter{}
	calc := costcalc.New()
	p, _ := New(Config{
		Providers: []Provider{{Name: "google", UpstreamURL: upstream.URL}},
		ProjectID: "test",
	}, submitter, calc)

	store := &captureTestUserStore{remainingUSD: 100.0}
	p.SetUserStore(store)

	mux := http.NewServeMux()
	p.RegisterRoutes(mux)
	srv := httptest.NewServer(withTestAuth(mux))
	defer srv.Close()

	reqBody := `{"contents":[{"parts":[{"text":"hi"}]}]}`
	req, _ := http.NewRequest("POST", srv.URL+"/proxy/google/v1beta/models/gemini-2.5-flash:streamGenerateContent", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer tok")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, want 200; body = %s", resp.StatusCode, body)
	}

	_, err = io.Copy(io.Discard, resp.Body)
	if err != nil {
		t.Fatalf("failed reading response body: %v", err)
	}

	var spans []storage.Span
	for i := 0; i < 50; i++ {
		spans = submitter.getSpans()
		if len(spans) > 0 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if len(spans) == 0 {
		t.Fatal("expected span to be submitted, got none")
	}

	span := spans[0]
	if span.GenAI == nil {
		t.Fatal("span.GenAI is nil")
	}

	// 150 prompt tokens with 40 cached read tokens normalizes to 114
	if span.GenAI.InputTokens != 114 {
		t.Errorf("span InputTokens = %d, want 114 (normalized with 40 cached tokens)", span.GenAI.InputTokens)
	}
	if span.GenAI.OutputTokens != 2400000 {
		t.Errorf("span OutputTokens = %d, want 2400000 (from tail usageMetadata)", span.GenAI.OutputTokens)
	}
	if span.GenAI.CacheReadTokens != 40 {
		t.Errorf("span CacheReadTokens = %d, want 40", span.GenAI.CacheReadTokens)
	}
}

// TestStreamCapture_TruncatedStream_GracefulBillingReconciliation verifies that
// when an upstream stream is truncated/aborted before emitting the final usage
// chunk, output tokens are estimated gracefully from the delivered content so
// billing deduction prevents revenue leakage (#525).
func TestStreamCapture_TruncatedStream_GracefulBillingReconciliation(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher, _ := w.(http.Flusher)

		// Send 10KB of text deltas
		chunk := bytes.Repeat([]byte("test content "), 800) // ~10KB
		msg, _ := json.Marshal(map[string]interface{}{
			"choices": []interface{}{
				map[string]interface{}{
					"delta": map[string]interface{}{"content": string(chunk)},
				},
			},
		})
		_, _ = fmt.Fprintf(w, "data: %s\n\n", msg)
		flusher.Flush()

		// Abruptly terminate connection without final usage or [DONE] chunk
	}))
	defer upstream.Close()

	submitter := &mockSubmitter{}
	calc := costcalc.New()
	p, _ := New(Config{
		Providers: []Provider{{Name: "openai", UpstreamURL: upstream.URL}},
		ProjectID: "test",
	}, submitter, calc)

	store := &captureTestUserStore{remainingUSD: 100.0}
	p.SetUserStore(store)

	mux := http.NewServeMux()
	p.RegisterRoutes(mux)
	srv := httptest.NewServer(withTestAuth(mux))
	defer srv.Close()

	reqBody := `{"model":"gpt-4o","stream":true,"messages":[{"role":"user","content":"generate something"}]}`
	req, _ := http.NewRequest("POST", srv.URL+"/proxy/openai/v1/chat/completions", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer tok")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	_, _ = io.Copy(io.Discard, resp.Body)

	var spans []storage.Span
	for i := 0; i < 50; i++ {
		spans = submitter.getSpans()
		if len(spans) > 0 {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if len(spans) == 0 {
		t.Fatal("expected span to be submitted, got none")
	}

	span := spans[0]
	if span.GenAI == nil {
		t.Fatal("span.GenAI is nil")
	}

	// Because final usage chunk was missing, tokens should be estimated from content
	if span.GenAI.OutputTokens <= 0 {
		t.Errorf("expected estimated OutputTokens > 0, got %d", span.GenAI.OutputTokens)
	}
	if span.Attributes["proxy.usage_estimated"] != "true" {
		t.Errorf("expected proxy.usage_estimated = true, got %s", span.Attributes["proxy.usage_estimated"])
	}
	if span.GenAI.CostUSD <= 0 {
		t.Errorf("span CostUSD = %f, want > 0", span.GenAI.CostUSD)
	}

	// Verify billing reconciliation deducted spend for the delivered output
	store.mu.Lock()
	defer store.mu.Unlock()
	if len(store.deductedCalls) == 0 {
		t.Fatal("expected DeductSpend to be called for truncated stream, got 0 calls")
	}
	if store.deductedCalls[0].costUSD <= 0 {
		t.Errorf("deducted costUSD = %f, want > 0", store.deductedCalls[0].costUSD)
	}
}

// TestBudgetReservation_ReasonableMinimumFloor verifies that budget pre-flight
// uses a reasonable floor ($0.05, not $0.001) for unknown requests, preventing
// users with sub-floor balances from overdrafting (#525).
func TestBudgetReservation_ReasonableMinimumFloor(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		t.Error("upstream should not be called when balance is below the $0.05 reservation floor")
		w.WriteHeader(500)
	}))
	defer upstream.Close()

	submitter := &mockSubmitter{}
	calc := costcalc.New()
	p, _ := New(Config{
		Providers: []Provider{{Name: "openai", UpstreamURL: upstream.URL}},
		ProjectID: "test",
	}, submitter, calc)

	// User has $0.02 remaining — greater than old $0.001 floor, but less than $0.05 reservation floor.
	store := &captureTestUserStore{remainingUSD: 0.02}
	p.SetUserStore(store)

	mux := http.NewServeMux()
	p.RegisterRoutes(mux)
	srv := httptest.NewServer(withTestAuth(mux))
	defer srv.Close()

	reqBody := `{"model":"gpt-4o-unknown","messages":[{"role":"user","content":"hi"}]}`
	req, _ := http.NewRequest("POST", srv.URL+"/proxy/openai/v1/chat/completions", strings.NewReader(reqBody))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Authorization", "Bearer tok")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusPaymentRequired {
		body, _ := io.ReadAll(resp.Body)
		t.Errorf("status = %d, want 402 Payment Required; body = %s", resp.StatusCode, body)
	}
}

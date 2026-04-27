package main

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/candelahq/candela/pkg/costcalc"
	"github.com/candelahq/candela/pkg/processor"
	"github.com/candelahq/candela/pkg/storage"
	sqlitestore "github.com/candelahq/candela/pkg/storage/sqlite"
)

func TestSpanCapture_NonStreaming(t *testing.T) {
	// Mock upstream returns an OpenAI-format response.
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":    "chatcmpl-123",
			"model": "llama3.2:3b",
			"choices": []map[string]any{
				{"message": map[string]string{"role": "assistant", "content": "Hello!"}},
			},
			"usage": map[string]int{
				"prompt_tokens":     10,
				"completion_tokens": 5,
				"total_tokens":      15,
			},
		})
	}))
	defer upstream.Close()

	// Set up SQLite store + processor.
	store, err := sqlitestore.New(sqlitestore.Config{Path: ":memory:"})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()

	calc := costcalc.New()
	proc := processor.New([]storage.SpanWriter{store}, calc, 10)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go proc.Run(ctx)
	defer proc.Stop()

	// Wrap upstream with span capture.
	handler := newSpanCapture(proxyTo(upstream.URL), proc, nil)
	srv := httptest.NewServer(handler)
	defer srv.Close()

	// Send chat completion request.
	body := `{"model": "llama3.2:3b", "messages": [{"role": "user", "content": "hi"}]}`
	resp, err := http.Post(srv.URL+"/v1/chat/completions", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	// Verify the response was passed through.
	var result map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&result)
	if result["model"] != "llama3.2:3b" {
		t.Errorf("response model = %v, want llama3.2:3b", result["model"])
	}

	// Wait for the processor to flush.
	time.Sleep(3 * time.Second)

	// Verify span was captured.
	spans, err := store.SearchSpans(ctx, storage.SpanQuery{
		ProjectID: "local",
		StartTime: time.Now().Add(-1 * time.Minute),
		EndTime:   time.Now().Add(1 * time.Minute),
		Kind:      storage.SpanKindLLM,
		PageSize:  10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(spans.Spans) != 1 {
		t.Fatalf("got %d spans, want 1", len(spans.Spans))
	}

	s := spans.Spans[0]
	if s.GenAI == nil {
		t.Fatal("span has nil GenAI")
	}
	if s.GenAI.Model != "llama3.2:3b" {
		t.Errorf("span model = %q, want llama3.2:3b", s.GenAI.Model)
	}
	if s.GenAI.InputTokens != 10 {
		t.Errorf("input tokens = %d, want 10", s.GenAI.InputTokens)
	}
	if s.GenAI.OutputTokens != 5 {
		t.Errorf("output tokens = %d, want 5", s.GenAI.OutputTokens)
	}
}

func TestSpanCapture_Passthrough(t *testing.T) {
	// Non chat/completions paths should pass through without capture.
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte(`{"ok": true}`))
	}))
	defer upstream.Close()

	handler := newSpanCapture(proxyTo(upstream.URL), nil, nil)
	srv := httptest.NewServer(handler)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/v1/models")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Errorf("status = %d, want 200", resp.StatusCode)
	}
}

func TestSpanCapture_SSEUsageParsing(t *testing.T) {
	input, output, total := parseSSEUsage([]byte(
		"data: {\"choices\":[{\"delta\":{\"content\":\"Hello\"}}]}\n\n" +
			"data: {\"choices\":[{\"delta\":{\"content\":\" world\"}}]}\n\n" +
			"data: {\"choices\":[],\"usage\":{\"prompt_tokens\":20,\"completion_tokens\":10,\"total_tokens\":30}}\n\n" +
			"data: [DONE]\n\n",
	))

	if input != 20 {
		t.Errorf("input = %d, want 20", input)
	}
	if output != 10 {
		t.Errorf("output = %d, want 10", output)
	}
	if total != 30 {
		t.Errorf("total = %d, want 30", total)
	}
}

func TestTracesHandler_ReturnsSpans(t *testing.T) {
	store, err := sqlitestore.New(sqlitestore.Config{Path: ":memory:"})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()

	// Insert a test span.
	now := time.Now()
	_ = store.IngestSpans(context.Background(), []storage.Span{
		{
			SpanID:    "test-span-1",
			TraceID:   "test-trace-1",
			Name:      "chat llama3.2:3b",
			Kind:      storage.SpanKindLLM,
			Status:    storage.SpanStatusOK,
			StartTime: now,
			EndTime:   now.Add(200 * time.Millisecond),
			Duration:  200 * time.Millisecond,
			ProjectID: "local",
			GenAI: &storage.GenAIAttributes{
				Model:        "llama3.2:3b",
				Provider:     "local",
				InputTokens:  50,
				OutputTokens: 25,
				TotalTokens:  75,
			},
		},
	})

	handler := newTracesHandler(store)
	srv := httptest.NewServer(handler)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "?limit=10")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	var result recentSpanResponse
	_ = json.NewDecoder(resp.Body).Decode(&result)

	if result.Count != 1 {
		t.Fatalf("count = %d, want 1", result.Count)
	}

	s := result.Spans[0]
	if s.Model != "llama3.2:3b" {
		t.Errorf("model = %q, want llama3.2:3b", s.Model)
	}
	if s.InputTokens != 50 {
		t.Errorf("input_tokens = %d, want 50", s.InputTokens)
	}
	if s.Status != "ok" {
		t.Errorf("status = %q, want ok", s.Status)
	}
}

func TestTracesHandler_NilReader(t *testing.T) {
	handler := newTracesHandler(nil)
	srv := httptest.NewServer(handler)
	defer srv.Close()

	resp, err := http.Get(srv.URL)
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusServiceUnavailable {
		t.Errorf("status = %d, want 503", resp.StatusCode)
	}
}

func TestSpanCapture_ErrorStatus(t *testing.T) {
	// Upstream returns 500 — span should record error status.
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusInternalServerError)
		_ = json.NewEncoder(w).Encode(map[string]string{"error": "model overloaded"})
	}))
	defer upstream.Close()

	store, err := sqlitestore.New(sqlitestore.Config{Path: ":memory:"})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()

	calc := costcalc.New()
	proc := processor.New([]storage.SpanWriter{store}, calc, 10)
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go proc.Run(ctx)
	defer proc.Stop()

	handler := newSpanCapture(proxyTo(upstream.URL), proc, nil)
	srv := httptest.NewServer(handler)
	defer srv.Close()

	body := `{"model": "llama3.2:3b", "messages": [{"role": "user", "content": "hi"}]}`
	resp, err := http.Post(srv.URL+"/v1/chat/completions", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusInternalServerError {
		t.Fatalf("status = %d, want 500", resp.StatusCode)
	}

	// Wait for flush.
	time.Sleep(3 * time.Second)

	spans, err := store.SearchSpans(ctx, storage.SpanQuery{
		ProjectID: "local",
		StartTime: time.Now().Add(-1 * time.Minute),
		EndTime:   time.Now().Add(1 * time.Minute),
		Kind:      storage.SpanKindLLM,
		PageSize:  10,
	})
	if err != nil {
		t.Fatal(err)
	}
	if len(spans.Spans) != 1 {
		t.Fatalf("got %d spans, want 1", len(spans.Spans))
	}
	if spans.Spans[0].Status != storage.SpanStatusError {
		t.Errorf("span status = %v, want Error", spans.Spans[0].Status)
	}
}

func TestSpanCapture_SSENoUsage(t *testing.T) {
	// SSE stream without usage info should return zeros.
	input, output, total := parseSSEUsage([]byte(
		"data: {\"choices\":[{\"delta\":{\"content\":\"Hello\"}}]}\n\n" +
			"data: [DONE]\n\n",
	))

	if input != 0 || output != 0 || total != 0 {
		t.Errorf("expected zeros for SSE without usage, got input=%d output=%d total=%d", input, output, total)
	}
}

func TestTracesHandler_MethodNotAllowed(t *testing.T) {
	store, err := sqlitestore.New(sqlitestore.Config{Path: ":memory:"})
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = store.Close() }()

	handler := newTracesHandler(store)
	srv := httptest.NewServer(handler)
	defer srv.Close()

	resp, err := http.Post(srv.URL, "application/json", strings.NewReader("{}"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusMethodNotAllowed {
		t.Errorf("status = %d, want 405", resp.StatusCode)
	}
}

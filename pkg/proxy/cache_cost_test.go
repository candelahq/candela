package proxy

import (
	"encoding/json"
	"fmt"
	"io"
	"math"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"testing"

	"github.com/candelahq/candela/pkg/costcalc"
	"github.com/candelahq/candela/pkg/storage"
)

// assertCacheCostSpan checks the two values a client needs for the cache hit
// share and the cache spend, on a response with a 5-minute cache read and write.
// The normalized input is 100 + 8 + 13 = 121, the true prompt is
// 100 + 80 + 10 = 190, and the cache charge is 20.5 × $3/M = $0.0000615.
func assertCacheCostSpan(t *testing.T, span storage.Span) {
	t.Helper()

	if span.GenAI == nil {
		t.Fatal("span.GenAI is nil")
	}
	if span.GenAI.InputTokens != 121 {
		t.Errorf("InputTokens = %d, want 121 (cost-normalized)", span.GenAI.InputTokens)
	}
	if span.GenAI.PromptTokens != 190 {
		t.Errorf("PromptTokens = %d, want 190 (100 fresh + 80 read + 10 written)", span.GenAI.PromptTokens)
	}
	if diff := span.GenAI.CacheCostUSD - 6.15e-5; math.Abs(diff) > 1e-12 {
		t.Errorf("CacheCostUSD = %g, want 6.15e-05", span.GenAI.CacheCostUSD)
	}

	if got := span.Attributes["gen_ai.usage.prompt_tokens"]; got != "190" {
		t.Errorf("attribute gen_ai.usage.prompt_tokens = %q, want \"190\"", got)
	}
	rawAttr, ok := span.Attributes["gen_ai.usage.cache_cost_usd"]
	if !ok {
		t.Fatal("attribute gen_ai.usage.cache_cost_usd is missing")
	}
	attrCost, err := strconv.ParseFloat(rawAttr, 64)
	if err != nil {
		t.Fatalf("attribute gen_ai.usage.cache_cost_usd = %q, not a float: %v", rawAttr, err)
	}
	if diff := attrCost - 6.15e-5; math.Abs(diff) > 1e-12 {
		t.Errorf("attribute gen_ai.usage.cache_cost_usd = %g, want 6.15e-05", attrCost)
	}
}

func firstSpan(t *testing.T, submitter *mockSubmitter) storage.Span {
	t.Helper()
	if !waitForSpan(submitter) {
		t.Fatal("expected span to be submitted")
	}
	return submitter.getSpans()[0]
}

func TestSpanCarriesPromptTokensAndCacheCost_Standard(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":   "msg_cache",
			"type": "message",
			"role": "assistant",
			"content": []map[string]any{
				{"type": "text", "text": "cached reply"},
			},
			"usage": map[string]any{
				"input_tokens":                100,
				"output_tokens":               12,
				"cache_read_input_tokens":     80,
				"cache_creation_input_tokens": 10,
			},
		})
	}))
	defer upstream.Close()

	submitter := &mockSubmitter{}
	p, _ := New(Config{
		Providers: []Provider{{Name: "anthropic-direct", UpstreamURL: upstream.URL}},
		ProjectID: "test",
	}, submitter, costcalc.New())

	mux := http.NewServeMux()
	p.RegisterRoutes(mux)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	req, _ := http.NewRequest("POST",
		srv.URL+"/proxy/anthropic-direct/v1/messages",
		strings.NewReader(`{"model":"claude-sonnet-4-20250514","messages":[{"role":"user","content":"hi"}],"max_tokens":64}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Api-Key", "sk-ant-test")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, resp.Body)

	assertCacheCostSpan(t, firstSpan(t, submitter))
}

func TestSpanCarriesPromptTokensAndCacheCost_Streaming(t *testing.T) {
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.Header().Set("Content-Type", "text/event-stream")
		w.WriteHeader(http.StatusOK)
		flusher := w.(http.Flusher)

		events := []string{
			`data: {"type":"message_start","message":{"id":"msg_cache","role":"assistant","content":[],"usage":{"input_tokens":100,"output_tokens":0,"cache_read_input_tokens":80,"cache_creation_input_tokens":10}}}`,
			`data: {"type":"content_block_delta","index":0,"delta":{"type":"text_delta","text":"cached"}}`,
			`data: {"type":"message_delta","delta":{"stop_reason":"end_turn"},"usage":{"output_tokens":12}}`,
			`data: {"type":"message_stop"}`,
		}
		for _, e := range events {
			_, _ = fmt.Fprintf(w, "%s\n\n", e)
			flusher.Flush()
		}
	}))
	defer upstream.Close()

	submitter := &mockSubmitter{}
	p, _ := New(Config{
		Providers: []Provider{{Name: "anthropic-direct", UpstreamURL: upstream.URL}},
		ProjectID: "test",
	}, submitter, costcalc.New())

	mux := http.NewServeMux()
	p.RegisterRoutes(mux)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	req, _ := http.NewRequest("POST",
		srv.URL+"/proxy/anthropic-direct/v1/messages",
		strings.NewReader(`{"model":"claude-sonnet-4-20250514","messages":[{"role":"user","content":"hi"}],"stream":true}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("X-Api-Key", "sk-ant-test")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.Copy(io.Discard, resp.Body)

	assertCacheCostSpan(t, firstSpan(t, submitter))
}

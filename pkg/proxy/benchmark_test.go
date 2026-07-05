package proxy

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"sync"
	"testing"

	"github.com/candelahq/candela/pkg/costcalc"
	"github.com/candelahq/candela/pkg/storage"
)

// benchSubmitter is a no-op span submitter for benchmarks.
type benchSubmitter struct {
	mu    sync.Mutex
	spans []storage.Span
}

func (s *benchSubmitter) SubmitBatch(spans []storage.Span) {
	s.mu.Lock()
	s.spans = append(s.spans, spans...)
	s.mu.Unlock()
}

// setupBenchProxy creates a mock upstream, proxy, and test server for benchmarking.
// Returns the proxy URL for the given provider. Cleanup is handled via b.Cleanup.
func setupBenchProxy(b *testing.B, providerName, upstreamBody string) string {
	b.Helper()

	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_, _ = w.Write([]byte(upstreamBody))
	}))
	b.Cleanup(upstream.Close)

	p, err := New(Config{
		Providers: []Provider{{Name: providerName, UpstreamURL: upstream.URL}},
		ProjectID: "bench-project",
	}, &benchSubmitter{}, costcalc.New())
	if err != nil {
		b.Fatal(err)
	}

	mux := http.NewServeMux()
	p.RegisterRoutes(mux)
	srv := httptest.NewServer(mux)
	b.Cleanup(srv.Close)

	return srv.URL
}

const benchOpenAIBody = `{
	"id": "chatcmpl-bench",
	"object": "chat.completion",
	"model": "gpt-4o",
	"choices": [{"index": 0, "message": {"role": "assistant", "content": "Hello!"}, "finish_reason": "stop"}],
	"usage": {"prompt_tokens": 10, "completion_tokens": 5, "total_tokens": 15}
}`

const benchAnthropicBody = `{
	"id": "msg-bench",
	"type": "message",
	"role": "assistant",
	"content": [{"type": "text", "text": "Hello!"}],
	"model": "claude-sonnet-4-20250514",
	"stop_reason": "end_turn",
	"usage": {"input_tokens": 10, "output_tokens": 5}
}`

// BenchmarkHandleProxy_OpenAI benchmarks the full proxy hot path
// for an OpenAI-compatible request (no format translation, no path rewriting).
func BenchmarkHandleProxy_OpenAI(b *testing.B) {
	proxyURL := setupBenchProxy(b, "openai", benchOpenAIBody)
	url := fmt.Sprintf("%s/proxy/openai/v1/chat/completions", proxyURL)
	body := `{"model": "gpt-4o", "messages": [{"role": "user", "content": "Hi"}]}`

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		req, _ := http.NewRequest("POST", url, strings.NewReader(body))
		req.Header.Set("Authorization", "Bearer sk-bench")
		req.Header.Set("Content-Type", "application/json")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			b.Fatal(err)
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()

		if resp.StatusCode != 200 {
			b.Fatalf("status = %d, want 200", resp.StatusCode)
		}
	}
}

// BenchmarkHandleProxy_AnthropicDirect benchmarks the proxy hot path
// for Anthropic direct (native passthrough, no format translation).
func BenchmarkHandleProxy_AnthropicDirect(b *testing.B) {
	proxyURL := setupBenchProxy(b, "anthropic-direct", benchAnthropicBody)
	url := fmt.Sprintf("%s/proxy/anthropic-direct/v1/messages", proxyURL)
	body := `{"model": "claude-sonnet-4-20250514", "messages": [{"role": "user", "content": "Hi"}], "max_tokens": 100}`

	b.ResetTimer()
	b.ReportAllocs()
	for i := 0; i < b.N; i++ {
		req, _ := http.NewRequest("POST", url, strings.NewReader(body))
		req.Header.Set("X-Api-Key", "sk-ant-bench")
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Anthropic-Version", "2023-06-01")

		resp, err := http.DefaultClient.Do(req)
		if err != nil {
			b.Fatal(err)
		}
		_, _ = io.Copy(io.Discard, resp.Body)
		_ = resp.Body.Close()

		if resp.StatusCode != 200 {
			b.Fatalf("status = %d, want 200", resp.StatusCode)
		}
	}
}

// BenchmarkHandleProxy_Parallel measures throughput under concurrent load.
func BenchmarkHandleProxy_Parallel(b *testing.B) {
	proxyURL := setupBenchProxy(b, "openai", benchOpenAIBody)
	url := fmt.Sprintf("%s/proxy/openai/v1/chat/completions", proxyURL)
	body := `{"model": "gpt-4o", "messages": [{"role": "user", "content": "Hi"}]}`

	b.ResetTimer()
	b.ReportAllocs()
	b.RunParallel(func(pb *testing.PB) {
		for pb.Next() {
			req, _ := http.NewRequest("POST", url, strings.NewReader(body))
			req.Header.Set("Authorization", "Bearer sk-bench")
			req.Header.Set("Content-Type", "application/json")

			resp, err := http.DefaultClient.Do(req)
			if err != nil {
				// b.Error is safe from goroutines (unlike b.Fatal).
				b.Error(err)
				return
			}
			_, _ = io.Copy(io.Discard, resp.Body)
			_ = resp.Body.Close()
		}
	})
}

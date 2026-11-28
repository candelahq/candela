package proxy

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"golang.org/x/oauth2"

	"github.com/candelahq/candela/pkg/costcalc"
)

func newEnterpriseGatewayTestProxy(t *testing.T, upstreamURL string) (*Proxy, *mockSubmitter) {
	t.Helper()
	submitter := &mockSubmitter{}
	calc := costcalc.New()

	p, err := New(Config{
		Providers: []Provider{
			{
				Name:        "gemini-vertex",
				UpstreamURL: upstreamURL,
				TokenSource: oauth2.StaticTokenSource(&oauth2.Token{AccessToken: "sa-token"}),
				PathRewriter: &VertexAIGooglePathRewriter{
					ProjectID: "test-proj",
					Region:    "global",
				},
			},
		},
		ProjectID: "test-proj",
	}, submitter, calc)
	if err != nil {
		t.Fatalf("proxy.New failed: %v", err)
	}
	return p, submitter
}

func TestEnterpriseGateway_GeminiVertex_SSE_HeadersAndSpan(t *testing.T) {
	var gotPath, gotQuery, gotAuth, gotUA, gotClient, gotCostCenter string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery
		gotAuth = r.Header.Get("Authorization")
		gotUA = r.Header.Get("User-Agent")
		gotClient = r.Header.Get("X-Goog-Api-Client")
		gotCostCenter = r.Header.Get("X-Cost-Center")

		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w,
			"data: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"Hello \"}]}}],\"modelVersion\":\"gemini-4-neon\"}\n\n"+
				"data: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"gateway\"}]}}],\"modelVersion\":\"gemini-4-neon\",\"usageMetadata\":{\"promptTokenCount\":100,\"cachedContentTokenCount\":80,\"candidatesTokenCount\":10,\"thoughtsTokenCount\":20,\"totalTokenCount\":130}}\n\n",
		)
	}))
	defer upstream.Close()

	p, submitter := newEnterpriseGatewayTestProxy(t, upstream.URL)
	mux := http.NewServeMux()
	p.RegisterRoutes(mux)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	req, _ := http.NewRequest(http.MethodPost,
		srv.URL+"/proxy/gemini-vertex/v1beta/models/gemini-4-neon:streamGenerateContent?alt=sse&key=client-key",
		strings.NewReader(`{"contents":[{"role":"user","parts":[{"text":"hi"}]}]}`))
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("User-Agent", "antigravity/1.0")
	req.Header.Set("X-Goog-Api-Client", "agy-gateway/1.0")
	req.Header.Set("X-Cost-Center", "eng-core")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, body = %s", resp.StatusCode, body)
	}
	if !strings.Contains(string(body), "data: ") || !strings.Contains(string(body), "gateway") {
		t.Errorf("unexpected SSE response body: %s", body)
	}

	wantPath := "/v1/projects/test-proj/locations/global/publishers/google/models/gemini-4-neon:streamGenerateContent"
	if gotPath != wantPath {
		t.Errorf("upstream path = %q, want %q", gotPath, wantPath)
	}
	if gotQuery != "alt=sse" {
		t.Errorf("upstream query = %q, want %q", gotQuery, "alt=sse")
	}
	if gotAuth != "Bearer sa-token" {
		t.Errorf("upstream Authorization = %q, want %q", gotAuth, "Bearer sa-token")
	}
	if gotUA != "antigravity/1.0" {
		t.Errorf("upstream User-Agent = %q, want %q", gotUA, "antigravity/1.0")
	}
	if gotClient != "agy-gateway/1.0" {
		t.Errorf("upstream X-Goog-Api-Client = %q, want %q", gotClient, "agy-gateway/1.0")
	}
	if gotCostCenter != "eng-core" {
		t.Errorf("upstream X-Cost-Center = %q, want %q", gotCostCenter, "eng-core")
	}

	for i := 0; i < 50; i++ {
		if len(submitter.getSpans()) > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}
	spans := submitter.getSpans()
	if len(spans) != 1 {
		t.Fatalf("spans count = %d, want 1", len(spans))
	}
	span := spans[0]
	if span.GenAI == nil {
		t.Fatal("span.GenAI is nil")
	}
	if span.GenAI.Model != "gemini-4-neon" {
		t.Errorf("span model = %q, want gemini-4-neon", span.GenAI.Model)
	}
	if span.GenAI.OutputContent != "Hello gateway" {
		t.Errorf("span output = %q, want %q", span.GenAI.OutputContent, "Hello gateway")
	}
	if span.GenAI.OutputTokens != 30 {
		t.Errorf("span outputTokens = %d, want 30", span.GenAI.OutputTokens)
	}
	if span.GenAI.CacheReadTokens != 80 {
		t.Errorf("span cacheReadTokens = %d, want 80", span.GenAI.CacheReadTokens)
	}
	if span.GenAI.PromptTokens != 100 {
		t.Errorf("span promptTokens = %d, want 100", span.GenAI.PromptTokens)
	}
}

func TestEnterpriseGateway_GeminiVertex_V1Beta1Preserved(t *testing.T) {
	var gotPath, gotQuery string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		gotQuery = r.URL.RawQuery
		w.Header().Set("Content-Type", "text/event-stream")
		_, _ = fmt.Fprint(w, "data: {\"candidates\":[{\"content\":{\"parts\":[{\"text\":\"ok\"}]}}]}\n\n")
	}))
	defer upstream.Close()

	p, _ := newEnterpriseGatewayTestProxy(t, upstream.URL)
	mux := http.NewServeMux()
	p.RegisterRoutes(mux)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	resp, err := http.Post(
		srv.URL+"/proxy/gemini-vertex/v1beta1/projects/other/locations/global/publishers/google/models/gemini-4-neon:streamGenerateContent?alt=sse",
		"application/json",
		strings.NewReader(`{"contents":[{"role":"user","parts":[{"text":"hi"}]}]}`),
	)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()
	_, _ = io.ReadAll(resp.Body)

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}
	wantPath := "/v1beta1/projects/test-proj/locations/global/publishers/google/models/gemini-4-neon:streamGenerateContent"
	if gotPath != wantPath {
		t.Errorf("upstream path = %q, want %q", gotPath, wantPath)
	}
	if gotQuery != "alt=sse" {
		t.Errorf("upstream query = %q, want alt=sse", gotQuery)
	}
}

func TestEnterpriseGateway_GeminiVertex_CountTokens(t *testing.T) {
	var gotPath string
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		gotPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_, _ = fmt.Fprint(w, `{"totalTokens":42}`)
	}))
	defer upstream.Close()

	p, submitter := newEnterpriseGatewayTestProxy(t, upstream.URL)
	mux := http.NewServeMux()
	p.RegisterRoutes(mux)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	resp, err := http.Post(
		srv.URL+"/proxy/gemini-vertex/v1beta/models/gemini-4-neon:countTokens",
		"application/json",
		strings.NewReader(`{"contents":[{"role":"user","parts":[{"text":"hello"}]}]}`),
	)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, body = %s", resp.StatusCode, body)
	}
	wantPath := "/v1/projects/test-proj/locations/global/publishers/google/models/gemini-4-neon:countTokens"
	if gotPath != wantPath {
		t.Errorf("upstream path = %q, want %q", gotPath, wantPath)
	}
	if strings.TrimSpace(string(body)) != `{"totalTokens":42}` {
		t.Errorf("body = %s, want {\"totalTokens\":42}", body)
	}

	time.Sleep(50 * time.Millisecond)
	if spans := submitter.getSpans(); len(spans) != 0 {
		t.Errorf("countTokens should submit 0 spans, got %d", len(spans))
	}
}

func TestEnterpriseGateway_GeminiVertex_TelemetryStub(t *testing.T) {
	upstreamCalled := false
	upstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		upstreamCalled = true
		w.WriteHeader(http.StatusNotFound)
	}))
	defer upstream.Close()

	p, submitter := newEnterpriseGatewayTestProxy(t, upstream.URL)
	mux := http.NewServeMux()
	p.RegisterRoutes(mux)
	srv := httptest.NewServer(mux)
	defer srv.Close()

	resp, err := http.Post(
		srv.URL+"/proxy/gemini-vertex/v1beta:recordMetrics",
		"application/json",
		strings.NewReader(`{"events":[]}`),
	)
	if err != nil {
		t.Fatalf("request failed: %v", err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, _ := io.ReadAll(resp.Body)
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, body = %s", resp.StatusCode, body)
	}
	if upstreamCalled {
		t.Error("upstream should not be called for telemetry stub")
	}
	if strings.TrimSpace(string(body)) != "{}" {
		t.Errorf("body = %q, want {}", body)
	}
	if resp.Header.Get("X-Request-ID") == "" {
		t.Error("expected X-Request-ID header on telemetry stub response")
	}

	time.Sleep(50 * time.Millisecond)
	if spans := submitter.getSpans(); len(spans) != 0 {
		t.Errorf("telemetry stub should submit 0 spans, got %d", len(spans))
	}
}

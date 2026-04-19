package main

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"net/http/httputil"
	"net/url"
	"strings"
	"testing"

	"github.com/candelahq/candela/pkg/runtime"
)

// mockRemoteServer returns an httptest.Server that responds to GET /v1/models
// with a canned OpenAI-format response.
func mockRemoteServer(t *testing.T, models []openaiModel) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/models" && r.Method == http.MethodGet {
			w.Header().Set("Content-Type", "application/json")
			_ = json.NewEncoder(w).Encode(openaiModelList{
				Object: "list",
				Data:   models,
			})
			return
		}
		if r.URL.Path == "/v1/chat/completions" && r.Method == http.MethodPost {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]string{"source": "remote"})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)
	return srv
}

// mockLocalServer returns an httptest.Server that responds to POST /v1/chat/completions.
func mockLocalServer(t *testing.T) *httptest.Server {
	t.Helper()
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.URL.Path == "/v1/chat/completions" && r.Method == http.MethodPost {
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			_ = json.NewEncoder(w).Encode(map[string]string{"source": "local"})
			return
		}
		w.WriteHeader(http.StatusNotFound)
	}))
	t.Cleanup(srv.Close)
	return srv
}

func proxyTo(target string) *httputil.ReverseProxy {
	u, _ := url.Parse(target)
	return httputil.NewSingleHostReverseProxy(u)
}

// lmMockRuntime is a minimal mock that returns canned models.
type lmMockRuntime struct {
	runtime.Runtime
	models []runtime.Model
}

func (m *lmMockRuntime) Name() string { return "ollama" }
func (m *lmMockRuntime) ListModels(_ context.Context) ([]runtime.Model, error) {
	return m.models, nil
}
func (m *lmMockRuntime) Health(_ context.Context) (*runtime.Health, error) {
	return &runtime.Health{Status: runtime.StatusRunning}, nil
}
func (m *lmMockRuntime) Start(_ context.Context) error { return nil }
func (m *lmMockRuntime) Stop(_ context.Context) error  { return nil }
func (m *lmMockRuntime) Endpoint() string              { return "http://127.0.0.1:11434" }
func (m *lmMockRuntime) PullModel(_ context.Context, _ string, _ chan<- runtime.PullProgress) error {
	return nil
}
func (m *lmMockRuntime) LoadModel(_ context.Context, _ string) error   { return nil }
func (m *lmMockRuntime) UnloadModel(_ context.Context, _ string) error { return nil }
func (m *lmMockRuntime) DeleteModel(_ context.Context, _ string) error { return nil }

func setupLMHandler(t *testing.T, localModels []runtime.Model, remoteModels []openaiModel) (*lmHandler, *httptest.Server) {
	t.Helper()

	remoteSrv := mockRemoteServer(t, remoteModels)
	localSrv := mockLocalServer(t)

	mock := &lmMockRuntime{models: localModels}
	mgr := runtime.NewManager(mock, runtime.ManagerConfig{HealthCheck: 10 * 1e9}) // 10s
	if err := mgr.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = mgr.Stop(context.Background()) })

	h := newLMHandler(mgr, proxyTo(remoteSrv.URL), proxyTo(localSrv.URL))

	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return h, srv
}

func TestLMHandler_Models_MergesLocalAndRemote(t *testing.T) {
	localModels := []runtime.Model{
		{ID: "llama3.2:3b"},
		{ID: "mistral:7b"},
	}
	remoteModels := []openaiModel{
		{ID: "gpt-4o", Object: "model", OwnedBy: "openai"},
		{ID: "claude-3.5-sonnet", Object: "model", OwnedBy: "anthropic"},
	}

	_, srv := setupLMHandler(t, localModels, remoteModels)

	resp, err := http.Get(srv.URL + "/v1/models")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	var result openaiModelList
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		t.Fatal(err)
	}

	if len(result.Data) != 4 {
		t.Fatalf("got %d models, want 4", len(result.Data))
	}

	// Check local models are first, owned by "ollama".
	if result.Data[0].ID != "llama3.2:3b" || result.Data[0].OwnedBy != "ollama" {
		t.Errorf("first model = %+v, want llama3.2:3b owned by ollama", result.Data[0])
	}
	if result.Data[1].ID != "mistral:7b" || result.Data[1].OwnedBy != "ollama" {
		t.Errorf("second model = %+v, want mistral:7b owned by ollama", result.Data[1])
	}

	// Check remote models follow.
	if result.Data[2].ID != "gpt-4o" || result.Data[2].OwnedBy != "openai" {
		t.Errorf("third model = %+v, want gpt-4o owned by openai", result.Data[2])
	}
	if result.Data[3].ID != "claude-3.5-sonnet" || result.Data[3].OwnedBy != "anthropic" {
		t.Errorf("fourth model = %+v, want claude-3.5-sonnet owned by anthropic", result.Data[3])
	}
}

func TestLMHandler_Models_NoRuntime(t *testing.T) {
	remoteModels := []openaiModel{
		{ID: "gpt-4o", Object: "model", OwnedBy: "openai"},
	}

	remoteSrv := mockRemoteServer(t, remoteModels)

	h := newLMHandler(nil, proxyTo(remoteSrv.URL), nil)
	srv := httptest.NewServer(h)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/v1/models")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()

	var result openaiModelList
	_ = json.NewDecoder(resp.Body).Decode(&result)

	if len(result.Data) != 1 {
		t.Fatalf("got %d models, want 1 (remote only)", len(result.Data))
	}
	if result.Data[0].ID != "gpt-4o" {
		t.Errorf("model = %q, want gpt-4o", result.Data[0].ID)
	}
}

func TestLMHandler_Models_RemoteDown(t *testing.T) {
	localModels := []runtime.Model{{ID: "llama3.2:3b"}}

	// Remote server that always fails.
	failSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, _ *http.Request) {
		w.WriteHeader(http.StatusInternalServerError)
	}))
	defer failSrv.Close()

	localSrv := mockLocalServer(t)

	mock := &lmMockRuntime{models: localModels}
	mgr := runtime.NewManager(mock, runtime.ManagerConfig{HealthCheck: 10 * 1e9})
	_ = mgr.Start(context.Background())
	defer func() { _ = mgr.Stop(context.Background()) }()

	h := newLMHandler(mgr, proxyTo(failSrv.URL), proxyTo(localSrv.URL))
	srv := httptest.NewServer(h)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/v1/models")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()

	var result openaiModelList
	_ = json.NewDecoder(resp.Body).Decode(&result)

	// Should still return local models even when remote is down.
	if len(result.Data) != 1 {
		t.Fatalf("got %d models, want 1 (local only)", len(result.Data))
	}
	if result.Data[0].ID != "llama3.2:3b" {
		t.Errorf("model = %q, want llama3.2:3b", result.Data[0].ID)
	}
}

func TestLMHandler_Chat_RoutesLocalModel(t *testing.T) {
	localModels := []runtime.Model{{ID: "llama3.2:3b"}}
	remoteModels := []openaiModel{{ID: "gpt-4o", Object: "model", OwnedBy: "openai"}}

	_, srv := setupLMHandler(t, localModels, remoteModels)

	// First, call /v1/models to populate the local model cache.
	resp, _ := http.Get(srv.URL + "/v1/models")
	_ = resp.Body.Close()

	// Now send a chat completion with a local model.
	body := `{"model": "llama3.2:3b", "messages": [{"role": "user", "content": "hi"}]}`
	resp, err := http.Post(srv.URL+"/v1/chat/completions", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()

	var result map[string]string
	_ = json.NewDecoder(resp.Body).Decode(&result)

	if result["source"] != "local" {
		t.Errorf("source = %q, want local", result["source"])
	}
}

func TestLMHandler_Chat_RoutesRemoteModel(t *testing.T) {
	localModels := []runtime.Model{{ID: "llama3.2:3b"}}
	remoteModels := []openaiModel{{ID: "gpt-4o", Object: "model", OwnedBy: "openai"}}

	_, srv := setupLMHandler(t, localModels, remoteModels)

	// Populate cache.
	resp, _ := http.Get(srv.URL + "/v1/models")
	_ = resp.Body.Close()

	// Send chat completion with a remote model.
	body := `{"model": "gpt-4o", "messages": [{"role": "user", "content": "hi"}]}`
	resp, err := http.Post(srv.URL+"/v1/chat/completions", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()

	var result map[string]string
	_ = json.NewDecoder(resp.Body).Decode(&result)

	if result["source"] != "remote" {
		t.Errorf("source = %q, want remote", result["source"])
	}
}

func TestLMHandler_Chat_UnknownModelDefaultsToRemote(t *testing.T) {
	localModels := []runtime.Model{{ID: "llama3.2:3b"}}
	remoteModels := []openaiModel{{ID: "gpt-4o", Object: "model", OwnedBy: "openai"}}

	_, srv := setupLMHandler(t, localModels, remoteModels)

	// Populate cache.
	resp, _ := http.Get(srv.URL + "/v1/models")
	_ = resp.Body.Close()

	// Send chat completion with unknown model — should go to remote.
	body := `{"model": "some-unknown-model", "messages": [{"role": "user", "content": "hi"}]}`
	resp, err := http.Post(srv.URL+"/v1/chat/completions", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()

	var result map[string]string
	_ = json.NewDecoder(resp.Body).Decode(&result)

	if result["source"] != "remote" {
		t.Errorf("source = %q, want remote (default for unknown models)", result["source"])
	}
}

func TestLMHandler_Passthrough(t *testing.T) {
	remoteSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]string{"path": r.URL.Path})
	}))
	defer remoteSrv.Close()

	h := newLMHandler(nil, proxyTo(remoteSrv.URL), nil)
	srv := httptest.NewServer(h)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/api/v0/models")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()

	body, _ := io.ReadAll(resp.Body)
	if !strings.Contains(string(body), "/api/v0/models") {
		t.Errorf("passthrough failed, got: %s", body)
	}
}

func TestLMHandler_Chat_TaglessModelMatch(t *testing.T) {
	// Local model is "llama3.2:latest", request uses "llama3.2" (no tag).
	localModels := []runtime.Model{{ID: "llama3.2:latest"}}
	remoteModels := []openaiModel{{ID: "gpt-4o", Object: "model", OwnedBy: "openai"}}

	_, srv := setupLMHandler(t, localModels, remoteModels)

	// Populate cache.
	resp, _ := http.Get(srv.URL + "/v1/models")
	_ = resp.Body.Close()

	// Request with tag-less model name should match local.
	body := `{"model": "llama3.2", "messages": [{"role": "user", "content": "hi"}]}`
	resp, err := http.Post(srv.URL+"/v1/chat/completions", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()

	var result map[string]string
	_ = json.NewDecoder(resp.Body).Decode(&result)

	if result["source"] != "local" {
		t.Errorf("source = %q, want local (tag-less match)", result["source"])
	}
}

func TestLMHandler_Chat_MalformedBody(t *testing.T) {
	localModels := []runtime.Model{{ID: "llama3.2:3b"}}
	remoteModels := []openaiModel{{ID: "gpt-4o", Object: "model", OwnedBy: "openai"}}

	_, srv := setupLMHandler(t, localModels, remoteModels)

	// Send malformed JSON — should default to remote without crashing.
	resp, err := http.Post(srv.URL+"/v1/chat/completions", "application/json", strings.NewReader("not json"))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()

	var result map[string]string
	_ = json.NewDecoder(resp.Body).Decode(&result)

	// Should fallback to remote (model field is empty → not local).
	if result["source"] != "remote" {
		t.Errorf("source = %q, want remote (malformed body fallback)", result["source"])
	}
}

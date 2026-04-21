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

	"github.com/candelahq/candela/pkg/costcalc"
	"github.com/candelahq/candela/pkg/proxy"
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

	h := newLMHandler(mgr, proxyTo(remoteSrv.URL), proxyTo(localSrv.URL), nil, nil, nil, nil)

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

	h := newLMHandler(nil, proxyTo(remoteSrv.URL), nil, nil, nil, nil, nil)
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

	h := newLMHandler(mgr, proxyTo(failSrv.URL), proxyTo(localSrv.URL), nil, nil, nil, nil)
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

	h := newLMHandler(nil, proxyTo(remoteSrv.URL), nil, nil, nil, nil, nil)
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

func setupSoloLMHandler(t *testing.T, localModels []runtime.Model) *httptest.Server {
	t.Helper()

	localSrv := mockLocalServer(t)

	mock := &lmMockRuntime{models: localModels}
	mgr := runtime.NewManager(mock, runtime.ManagerConfig{HealthCheck: 10 * 1e9})
	if err := mgr.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = mgr.Stop(context.Background()) })

	// Solo mode: nil remote proxy.
	h := newLMHandler(mgr, nil, proxyTo(localSrv.URL), nil, nil, nil, nil)

	srv := httptest.NewServer(h)
	t.Cleanup(srv.Close)
	return srv
}

func TestLMHandler_SoloMode_ModelsLocalOnly(t *testing.T) {
	localModels := []runtime.Model{
		{ID: "llama3.2:3b"},
		{ID: "mistral:7b"},
	}

	srv := setupSoloLMHandler(t, localModels)

	resp, err := http.Get(srv.URL + "/v1/models")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()

	var result openaiModelList
	_ = json.NewDecoder(resp.Body).Decode(&result)

	// Should return only local models, no remote.
	if len(result.Data) != 2 {
		t.Fatalf("got %d models, want 2 (local only)", len(result.Data))
	}
	if result.Data[0].ID != "llama3.2:3b" {
		t.Errorf("first model = %q, want llama3.2:3b", result.Data[0].ID)
	}
}

func TestLMHandler_SoloMode_ChatLocalRoutes(t *testing.T) {
	localModels := []runtime.Model{{ID: "llama3.2:3b"}}
	srv := setupSoloLMHandler(t, localModels)

	// Populate cache.
	resp, _ := http.Get(srv.URL + "/v1/models")
	_ = resp.Body.Close()

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

func TestLMHandler_SoloMode_ChatUnknownModel404(t *testing.T) {
	localModels := []runtime.Model{{ID: "llama3.2:3b"}}
	srv := setupSoloLMHandler(t, localModels)

	body := `{"model": "gpt-4o", "messages": [{"role": "user", "content": "hi"}]}`
	resp, err := http.Post(srv.URL+"/v1/chat/completions", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404 (no remote in solo mode)", resp.StatusCode)
	}
}

func TestLMHandler_SoloMode_PassthroughNoPanic(t *testing.T) {
	localModels := []runtime.Model{{ID: "llama3.2:3b"}}
	srv := setupSoloLMHandler(t, localModels)

	// Unknown path in solo mode should return 404, not panic.
	resp, err := http.Get(srv.URL + "/api/v0/whatever")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404 (solo mode passthrough)", resp.StatusCode)
	}

	// Verify it's proper JSON.
	var result map[string]string
	_ = json.NewDecoder(resp.Body).Decode(&result)
	if result["error"] == "" {
		t.Error("expected JSON error body")
	}
}

func TestLMHandler_CloudModelsIncluded(t *testing.T) {
	// Cloud models should appear in /v1/models alongside local models.
	cloudModels := map[string]string{
		"gemini-2.5-pro":           "gemini-oai",
		"claude-sonnet-4-20250514": "anthropic",
	}

	h := newLMHandler(nil, nil, nil, nil, nil, cloudModels, costcalc.New())
	srv := httptest.NewServer(h)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/v1/models")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	var result openaiModelList
	_ = json.NewDecoder(resp.Body).Decode(&result)

	if len(result.Data) != 2 {
		t.Fatalf("got %d models, want 2", len(result.Data))
	}

	found := make(map[string]bool)
	for _, m := range result.Data {
		found[m.ID] = true
	}
	if !found["gemini-2.5-pro"] {
		t.Error("missing gemini-2.5-pro in model list")
	}
	if !found["claude-sonnet-4-20250514"] {
		t.Error("missing claude-sonnet-4-20250514 in model list")
	}
}

func TestLMHandler_CloudChatRouting(t *testing.T) {
	// Cloud model chat should route to the cloud proxy.
	var receivedPath string
	cloudUpstream := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		receivedPath = r.URL.Path
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":    "chatcmpl-cloud",
			"model": "gemini-2.5-pro",
			"choices": []map[string]any{
				{"message": map[string]string{"role": "assistant", "content": "Hello from cloud!"}},
			},
			"usage": map[string]int{"prompt_tokens": 10, "completion_tokens": 5, "total_tokens": 15},
		})
	}))
	defer cloudUpstream.Close()

	// Build a minimal cloud proxy pointing at our mock.
	cloudModels := map[string]string{"gemini-2.5-pro": "gemini-oai"}

	// Create a proxy.Proxy with the mock upstream.
	calc := costcalc.New()
	cp := proxy.New(proxy.Config{
		Providers: []proxy.Provider{
			{Name: "gemini-oai", UpstreamURL: cloudUpstream.URL},
		},
		ProjectID: "local",
	}, nil, calc)

	h := newLMHandler(nil, nil, nil, nil, cp, cloudModels, calc)
	srv := httptest.NewServer(h)
	defer srv.Close()

	body := `{"model": "gemini-2.5-pro", "messages": [{"role": "user", "content": "hi"}]}`
	resp, err := http.Post(srv.URL+"/v1/chat/completions", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		t.Fatalf("status = %d, want 200, body: %s", resp.StatusCode, respBody)
	}

	// Verify the proxy received the rewritten path.
	if !strings.Contains(receivedPath, "/v1/chat/completions") {
		t.Errorf("cloud proxy received path = %q, want .../v1/chat/completions", receivedPath)
	}
}

func TestLMHandler_LocalPreference(t *testing.T) {
	// If a model exists both locally and in cloud, local should take priority.
	localSrv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		_ = json.NewEncoder(w).Encode(map[string]any{
			"id":      "chatcmpl-local",
			"model":   "llama3.2:3b",
			"choices": []map[string]any{{"message": map[string]string{"role": "assistant", "content": "local response"}}},
			"usage":   map[string]int{"prompt_tokens": 5, "completion_tokens": 3, "total_tokens": 8},
		})
	}))
	defer localSrv.Close()

	cloudModels := map[string]string{"llama3.2:3b": "gemini-oai"}

	mock := &lmMockRuntime{models: []runtime.Model{
		{ID: "llama3.2:3b"},
	}}
	mgr := runtime.NewManager(mock, runtime.ManagerConfig{HealthCheck: 10 * 1e9})
	_ = mgr.Start(context.Background())
	defer func() { _ = mgr.Stop(context.Background()) }()

	localProxy := proxyTo(localSrv.URL)
	h := newLMHandler(mgr, nil, localProxy, localProxy, nil, cloudModels, costcalc.New())
	srv := httptest.NewServer(h)
	defer srv.Close()

	// Prime the local model cache by calling /v1/models first.
	modelsResp, err := http.Get(srv.URL + "/v1/models")
	if err != nil {
		t.Fatal(err)
	}
	_ = modelsResp.Body.Close()

	body := `{"model": "llama3.2:3b", "messages": [{"role": "user", "content": "hi"}]}`
	resp, err := http.Post(srv.URL+"/v1/chat/completions", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		t.Fatalf("status = %d, want 200", resp.StatusCode)
	}

	var result map[string]any
	_ = json.NewDecoder(resp.Body).Decode(&result)
	if result["id"] != "chatcmpl-local" {
		t.Errorf("got id = %v, want chatcmpl-local (local should take priority)", result["id"])
	}
}

func TestLMHandler_CloudAndLocalModelsMerged(t *testing.T) {
	// Verify local + cloud models all appear in /v1/models together.
	localSrv := mockLocalServer(t)
	cloudModels := map[string]string{
		"gemini-2.5-pro": "gemini-oai",
	}

	localModels := []runtime.Model{{ID: "llama3.2:3b"}}
	mock := &lmMockRuntime{models: localModels}
	mgr := runtime.NewManager(mock, runtime.ManagerConfig{HealthCheck: 10 * 1e9})
	_ = mgr.Start(context.Background())
	defer func() { _ = mgr.Stop(context.Background()) }()

	localProxy := proxyTo(localSrv.URL)
	h := newLMHandler(mgr, nil, localProxy, nil, nil, cloudModels, costcalc.New())
	srv := httptest.NewServer(h)
	defer srv.Close()

	resp, err := http.Get(srv.URL + "/v1/models")
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()

	var result openaiModelList
	_ = json.NewDecoder(resp.Body).Decode(&result)

	if len(result.Data) != 2 {
		t.Fatalf("got %d models, want 2 (1 local + 1 cloud)", len(result.Data))
	}

	found := make(map[string]bool)
	for _, m := range result.Data {
		found[m.ID] = true
	}
	if !found["llama3.2:3b"] {
		t.Error("missing local model llama3.2:3b")
	}
	if !found["gemini-2.5-pro"] {
		t.Error("missing cloud model gemini-2.5-pro")
	}
}

func TestLMHandler_CloudModelWhenProxyNil(t *testing.T) {
	// If cloud models are configured but cloudProxy is nil (e.g. ADC failed),
	// models should still appear in /v1/models but chat should 404.
	cloudModels := map[string]string{"gemini-2.5-pro": "gemini-oai"}

	h := newLMHandler(nil, nil, nil, nil, nil, cloudModels, costcalc.New()) // no cloudProxy
	srv := httptest.NewServer(h)
	defer srv.Close()

	// Models should still list.
	resp, err := http.Get(srv.URL + "/v1/models")
	if err != nil {
		t.Fatal(err)
	}
	_ = resp.Body.Close()
	if resp.StatusCode != http.StatusOK {
		t.Fatalf("models status = %d, want 200", resp.StatusCode)
	}

	// Chat should fall through to 404 since cloudProxy is nil.
	body := `{"model": "gemini-2.5-pro", "messages": [{"role": "user", "content": "hi"}]}`
	chatResp, err := http.Post(srv.URL+"/v1/chat/completions", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = chatResp.Body.Close() }()

	if chatResp.StatusCode != http.StatusNotFound {
		t.Errorf("chat status = %d, want 404 (cloud proxy is nil)", chatResp.StatusCode)
	}
}

func TestLMHandler_UnknownModelSoloCloudMode(t *testing.T) {
	// In solo+cloud mode, requesting a model that's neither local nor in
	// cloudModels should return 404 (not panic or route incorrectly).
	cloudModels := map[string]string{"gemini-2.5-pro": "gemini-oai"}

	h := newLMHandler(nil, nil, nil, nil, nil, cloudModels, costcalc.New())
	srv := httptest.NewServer(h)
	defer srv.Close()

	body := `{"model": "gpt-4o", "messages": [{"role": "user", "content": "hi"}]}`
	resp, err := http.Post(srv.URL+"/v1/chat/completions", "application/json", strings.NewReader(body))
	if err != nil {
		t.Fatal(err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusNotFound {
		t.Errorf("status = %d, want 404 for unknown model in solo+cloud mode", resp.StatusCode)
	}

	var result map[string]string
	_ = json.NewDecoder(resp.Body).Decode(&result)
	if result["error"] == "" {
		t.Error("expected error message in response body")
	}
}

func TestBuildCloudProxy_NoProject(t *testing.T) {
	// buildCloudProxy should return nil when no project is configured
	// and gcloud is unavailable.
	cfg := Config{
		Providers: []LocalProvider{{Name: "google", Models: []string{"gemini-2.5-pro"}}},
		VertexAI:  VertexAIConfig{}, // no project
	}

	// This may return nil due to missing project or ADC — either is acceptable.
	// The key test is that it doesn't panic.
	cp, models := buildCloudProxy(cfg, nil)

	// Without a valid project, we expect nil.
	// (If running in a CI with gcloud configured, it may succeed — that's OK too.)
	if cp == nil && len(models) > 0 {
		t.Error("models returned without a proxy — should both be nil")
	}
}

func TestBuildCloudProxy_UnknownProvider(t *testing.T) {
	// Unknown provider names should be skipped without panic.
	cfg := Config{
		Providers: []LocalProvider{
			{Name: "openai", Models: []string{"gpt-4o"}},         // unknown — skipped
			{Name: "google", Models: []string{"gemini-2.5-pro"}}, // valid
		},
		VertexAI: VertexAIConfig{Project: "test-project"},
	}

	// This may fail on ADC in CI, but should not panic.
	// The point is: unknown provider doesn't crash.
	cp, models := buildCloudProxy(cfg, nil)

	// If ADC not available, both nil — OK.
	if cp != nil {
		// openai should have been skipped, google kept.
		if _, ok := models["gpt-4o"]; ok {
			t.Error("openai provider should have been skipped")
		}
		if _, ok := models["gemini-2.5-pro"]; !ok {
			t.Error("google provider should have been kept")
		}
	}
}

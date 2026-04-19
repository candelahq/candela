package main

import (
	"context"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"sync"
	"testing"
	"time"

	connect "connectrpc.com/connect"

	v1 "github.com/candelahq/candela/gen/go/candela/v1"
	"github.com/candelahq/candela/gen/go/candela/v1/candelav1connect"
	"github.com/candelahq/candela/pkg/runtime"

	// Register backends for ListBackends.
	_ "github.com/candelahq/candela/pkg/runtime/lmstudio"
	_ "github.com/candelahq/candela/pkg/runtime/ollama"
	_ "github.com/candelahq/candela/pkg/runtime/vllm"
)

// rpcMockRuntime implements runtime.Runtime for handler tests.
type rpcMockRuntime struct {
	mu           sync.Mutex
	healthy      bool
	models       []runtime.Model
	loadCalled   []string
	unloadCalled []string
	pullCalled   []string
	healthErr    error
	listErr      error
}

func (m *rpcMockRuntime) Name() string     { return "mock" }
func (m *rpcMockRuntime) Endpoint() string { return "http://127.0.0.1:9999/v1" }

func (m *rpcMockRuntime) Start(_ context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.healthy = true
	return nil
}

func (m *rpcMockRuntime) Stop(_ context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.healthy = false
	return nil
}

func (m *rpcMockRuntime) Health(_ context.Context) (*runtime.Health, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.healthErr != nil {
		return nil, m.healthErr
	}
	status := runtime.StatusStopped
	if m.healthy {
		status = runtime.StatusRunning
	}
	return &runtime.Health{
		Status:    status,
		Endpoint:  "http://127.0.0.1:9999/v1",
		Models:    m.models,
		CheckedAt: time.Now(),
	}, nil
}

func (m *rpcMockRuntime) ListModels(_ context.Context) ([]runtime.Model, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	if m.listErr != nil {
		return nil, m.listErr
	}
	return m.models, nil
}

func (m *rpcMockRuntime) PullModel(_ context.Context, modelID string, progress chan<- runtime.PullProgress) error {
	m.mu.Lock()
	m.pullCalled = append(m.pullCalled, modelID)
	m.mu.Unlock()
	if progress != nil {
		progress <- runtime.PullProgress{Status: "done", Percent: 100}
	}
	return nil
}

func (m *rpcMockRuntime) LoadModel(_ context.Context, modelID string) error {
	m.mu.Lock()
	m.loadCalled = append(m.loadCalled, modelID)
	m.mu.Unlock()
	return nil
}

func (m *rpcMockRuntime) UnloadModel(_ context.Context, modelID string) error {
	m.mu.Lock()
	m.unloadCalled = append(m.unloadCalled, modelID)
	m.mu.Unlock()
	return nil
}

func setupRPCHandler(t *testing.T) (candelav1connect.RuntimeServiceClient, *rpcMockRuntime) {
	t.Helper()

	mock := &rpcMockRuntime{
		healthy: true,
		models: []runtime.Model{
			{ID: "llama3.2:8b", SizeBytes: 4_700_000_000, Family: "llama", Loaded: true},
			{ID: "codellama:13b", SizeBytes: 7_300_000_000, Family: "llama", Loaded: false},
		},
	}

	mgr := runtime.NewManager(mock, runtime.ManagerConfig{
		HealthCheck: 10 * time.Second,
	})
	if err := mgr.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = mgr.Stop(context.Background()) })

	// Wait for health to populate.
	for i := 0; i < 10; i++ {
		if mgr.Health().Status == runtime.StatusRunning {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	mux := http.NewServeMux()
	path, handler := candelav1connect.NewRuntimeServiceHandler(
		newRuntimeHandler(mgr, nil, context.Background()))
	mux.Handle(path, handler)

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	client := candelav1connect.NewRuntimeServiceClient(
		http.DefaultClient, srv.URL)
	return client, mock
}

func TestRPC_GetHealth(t *testing.T) {
	client, _ := setupRPCHandler(t)

	resp, err := client.GetHealth(context.Background(),
		connect.NewRequest(&v1.GetHealthRequest{}))
	if err != nil {
		t.Fatal(err)
	}
	if resp.Msg.Status.Status != "running" {
		t.Errorf("status = %q, want running", resp.Msg.Status.Status)
	}
	if resp.Msg.Status.Backend != "mock" {
		t.Errorf("backend = %q, want mock", resp.Msg.Status.Backend)
	}
	if len(resp.Msg.Models) != 2 {
		t.Errorf("got %d models, want 2", len(resp.Msg.Models))
	}
}

func TestRPC_ListModels(t *testing.T) {
	client, _ := setupRPCHandler(t)

	resp, err := client.ListModels(context.Background(),
		connect.NewRequest(&v1.ListModelsRequest{}))
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Msg.Models) != 2 {
		t.Fatalf("got %d models, want 2", len(resp.Msg.Models))
	}
	if resp.Msg.Models[0].Id != "llama3.2:8b" {
		t.Errorf("models[0].Id = %q, want llama3.2:8b", resp.Msg.Models[0].Id)
	}
	if !resp.Msg.Models[0].Loaded {
		t.Error("models[0].Loaded = false, want true")
	}
}

func TestRPC_LoadModel(t *testing.T) {
	client, mock := setupRPCHandler(t)

	resp, err := client.LoadModel(context.Background(),
		connect.NewRequest(&v1.LoadModelRequest{Model: "llama3.2:8b"}))
	if err != nil {
		t.Fatal(err)
	}
	if resp.Msg.Status != "loaded" {
		t.Errorf("status = %q, want loaded", resp.Msg.Status)
	}

	mock.mu.Lock()
	loaded := mock.loadCalled
	mock.mu.Unlock()
	if len(loaded) != 1 || loaded[0] != "llama3.2:8b" {
		t.Errorf("loadCalled = %v, want [llama3.2:8b]", loaded)
	}
}

func TestRPC_LoadModel_EmptyModel(t *testing.T) {
	client, _ := setupRPCHandler(t)

	_, err := client.LoadModel(context.Background(),
		connect.NewRequest(&v1.LoadModelRequest{Model: ""}))
	if err == nil {
		t.Fatal("expected error for empty model")
	}
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("code = %v, want InvalidArgument", connect.CodeOf(err))
	}
}

func TestRPC_UnloadModel(t *testing.T) {
	client, mock := setupRPCHandler(t)

	_, err := client.UnloadModel(context.Background(),
		connect.NewRequest(&v1.UnloadModelRequest{Model: "llama3.2:8b"}))
	if err != nil {
		t.Fatal(err)
	}

	mock.mu.Lock()
	unloaded := mock.unloadCalled
	mock.mu.Unlock()
	if len(unloaded) != 1 || unloaded[0] != "llama3.2:8b" {
		t.Errorf("unloadCalled = %v, want [llama3.2:8b]", unloaded)
	}
}

func TestRPC_PullModel(t *testing.T) {
	client, mock := setupRPCHandler(t)

	resp, err := client.PullModel(context.Background(),
		connect.NewRequest(&v1.PullModelRequest{Model: "mistral:7b"}))
	if err != nil {
		t.Fatal(err)
	}
	if resp.Msg.Status != "pulling" {
		t.Errorf("status = %q, want pulling", resp.Msg.Status)
	}

	// Wait for async pull to complete.
	for i := 0; i < 20; i++ {
		mock.mu.Lock()
		n := len(mock.pullCalled)
		mock.mu.Unlock()
		if n > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	mock.mu.Lock()
	pulled := mock.pullCalled
	mock.mu.Unlock()
	if len(pulled) != 1 || pulled[0] != "mistral:7b" {
		t.Errorf("pullCalled = %v, want [mistral:7b]", pulled)
	}
}

func TestRPC_PullModel_EmptyModel(t *testing.T) {
	client, _ := setupRPCHandler(t)

	_, err := client.PullModel(context.Background(),
		connect.NewRequest(&v1.PullModelRequest{Model: ""}))
	if err == nil {
		t.Fatal("expected error for empty model")
	}
	if connect.CodeOf(err) != connect.CodeInvalidArgument {
		t.Errorf("code = %v, want InvalidArgument", connect.CodeOf(err))
	}
}

func TestRPC_ListBackends(t *testing.T) {
	client, _ := setupRPCHandler(t)

	resp, err := client.ListBackends(context.Background(),
		connect.NewRequest(&v1.ListBackendsRequest{}))
	if err != nil {
		t.Fatal(err)
	}
	if len(resp.Msg.Backends) < 3 {
		t.Errorf("got %d backends, want >= 3", len(resp.Msg.Backends))
	}
	if resp.Msg.Active != "mock" {
		t.Errorf("active = %q, want mock", resp.Msg.Active)
	}
}

func TestRPC_StartStopRuntime(t *testing.T) {
	client, _ := setupRPCHandler(t)

	// Stop.
	stopResp, err := client.StopRuntime(context.Background(),
		connect.NewRequest(&v1.StopRuntimeRequest{}))
	if err != nil {
		t.Fatal(err)
	}
	if stopResp.Msg.Status.Status != "stopped" {
		t.Errorf("after stop: status = %q, want stopped", stopResp.Msg.Status.Status)
	}

	// Start.
	startResp, err := client.StartRuntime(context.Background(),
		connect.NewRequest(&v1.StartRuntimeRequest{}))
	if err != nil {
		t.Fatal(err)
	}
	// After start, the mock sets healthy=true, but the health loop may not
	// have run yet. The runtime itself reports running.
	if startResp.Msg.Status.Backend != "mock" {
		t.Errorf("backend = %q, want mock", startResp.Msg.Status.Backend)
	}
}

// setupRPCHandlerWithState is like setupRPCHandler but includes a real state DB.
func setupRPCHandlerWithState(t *testing.T) (candelav1connect.RuntimeServiceClient, *StateDB) {
	t.Helper()

	mock := &rpcMockRuntime{
		healthy: true,
		models:  []runtime.Model{{ID: "test-model", Loaded: true}},
	}

	mgr := runtime.NewManager(mock, runtime.ManagerConfig{
		HealthCheck: 10 * time.Second,
	})
	if err := mgr.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = mgr.Stop(context.Background()) })

	// Wait for health.
	for i := 0; i < 10; i++ {
		if mgr.Health().Status == runtime.StatusRunning {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	// Create a temp state DB.
	dbPath := filepath.Join(t.TempDir(), "state.db")
	stateDB, err := openStateDB(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	t.Cleanup(func() { _ = stateDB.Close() })

	mux := http.NewServeMux()
	path, handler := candelav1connect.NewRuntimeServiceHandler(
		newRuntimeHandler(mgr, stateDB, context.Background()))
	mux.Handle(path, handler)

	srv := httptest.NewServer(mux)
	t.Cleanup(srv.Close)

	client := candelav1connect.NewRuntimeServiceClient(
		http.DefaultClient, srv.URL)
	return client, stateDB
}

func TestRPC_ResetState(t *testing.T) {
	client, stateDB := setupRPCHandlerWithState(t)

	// Populate state.
	if err := stateDB.SetSetting("theme", "dark"); err != nil {
		t.Fatal(err)
	}
	if err := stateDB.RecordPull("llama3.2:8b", "ollama", 4_700_000_000); err != nil {
		t.Fatal(err)
	}

	// Verify state exists.
	if got := stateDB.GetSetting("theme"); got != "dark" {
		t.Fatalf("pre-reset: theme = %q, want dark", got)
	}

	// Reset via RPC.
	_, err := client.ResetState(context.Background(),
		connect.NewRequest(&v1.ResetStateRequest{}))
	if err != nil {
		t.Fatal(err)
	}

	// Verify state is cleared.
	if got := stateDB.GetSetting("theme"); got != "" {
		t.Errorf("after reset: theme = %q, want empty", got)
	}
	records, _ := stateDB.RecentPulls(10)
	if len(records) != 0 {
		t.Errorf("after reset: %d pull records, want 0", len(records))
	}
}

func TestRPC_ResetState_NoStateDB(t *testing.T) {
	// Setup handler with nil state DB.
	mock := &rpcMockRuntime{healthy: true}
	mgr := runtime.NewManager(mock, runtime.ManagerConfig{
		HealthCheck: 10 * time.Second,
	})
	if err := mgr.Start(context.Background()); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = mgr.Stop(context.Background()) }()

	mux := http.NewServeMux()
	path, handler := candelav1connect.NewRuntimeServiceHandler(
		newRuntimeHandler(mgr, nil, context.Background())) // nil state DB
	mux.Handle(path, handler)

	srv := httptest.NewServer(mux)
	defer srv.Close()

	client := candelav1connect.NewRuntimeServiceClient(
		http.DefaultClient, srv.URL)

	_, err := client.ResetState(context.Background(),
		connect.NewRequest(&v1.ResetStateRequest{}))
	if err == nil {
		t.Fatal("expected error when state DB is nil")
	}
	if connect.CodeOf(err) != connect.CodeFailedPrecondition {
		t.Errorf("code = %v, want FailedPrecondition", connect.CodeOf(err))
	}
}

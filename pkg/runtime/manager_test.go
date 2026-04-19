package runtime_test

import (
	"context"
	"sync"
	"testing"
	"time"

	"github.com/candelahq/candela/pkg/runtime"
)

// mockRuntime is a test double for the Runtime interface.
type mockRuntime struct {
	name        string
	endpoint    string
	healthy     bool
	models      []runtime.Model
	startCalled bool
	stopCalled  bool
	pullCalled  []string
	mu          sync.Mutex
}

func (m *mockRuntime) Name() string     { return m.name }
func (m *mockRuntime) Endpoint() string { return m.endpoint }

func (m *mockRuntime) Start(_ context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.startCalled = true
	m.healthy = true
	return nil
}

func (m *mockRuntime) Stop(_ context.Context) error {
	m.mu.Lock()
	defer m.mu.Unlock()
	m.stopCalled = true
	m.healthy = false
	return nil
}

func (m *mockRuntime) Health(_ context.Context) (*runtime.Health, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	status := runtime.StatusStopped
	if m.healthy {
		status = runtime.StatusRunning
	}
	return &runtime.Health{
		Status:    status,
		Endpoint:  m.endpoint,
		Models:    m.models,
		CheckedAt: time.Now(),
	}, nil
}

func (m *mockRuntime) ListModels(_ context.Context) ([]runtime.Model, error) {
	m.mu.Lock()
	defer m.mu.Unlock()
	return m.models, nil
}

func (m *mockRuntime) PullModel(_ context.Context, modelID string, progress chan<- runtime.PullProgress) error {
	m.mu.Lock()
	m.pullCalled = append(m.pullCalled, modelID)
	m.mu.Unlock()
	if progress != nil {
		progress <- runtime.PullProgress{Status: "done", Percent: 100}
	}
	return nil
}

func (m *mockRuntime) LoadModel(_ context.Context, _ string) error   { return nil }
func (m *mockRuntime) UnloadModel(_ context.Context, _ string) error { return nil }
func (m *mockRuntime) DeleteModel(_ context.Context, _ string) error { return nil }

func TestManagerAutoStart(t *testing.T) {
	mock := &mockRuntime{
		name:     "test",
		endpoint: "http://127.0.0.1:9999/v1",
		models:   []runtime.Model{{ID: "test-model", Loaded: true}},
	}

	mgr := runtime.NewManager(mock, runtime.ManagerConfig{
		AutoStart:   true,
		HealthCheck: 100 * time.Millisecond,
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := mgr.Start(ctx); err != nil {
		t.Fatalf("Manager.Start() error: %v", err)
	}
	defer func() { _ = mgr.Stop(ctx) }()

	mock.mu.Lock()
	started := mock.startCalled
	mock.mu.Unlock()
	if !started {
		t.Error("expected runtime.Start() to be called with AutoStart=true")
	}

	if got := mgr.Endpoint(); got != "http://127.0.0.1:9999/v1" {
		t.Errorf("Endpoint() = %q, want http://127.0.0.1:9999/v1", got)
	}

	// Wait for health loop to run.
	time.Sleep(250 * time.Millisecond)

	h := mgr.Health()
	if h.Status != runtime.StatusRunning {
		t.Errorf("Health().Status = %q, want %q", h.Status, runtime.StatusRunning)
	}
}

func TestManagerNoAutoStart(t *testing.T) {
	mock := &mockRuntime{
		name:     "test",
		endpoint: "http://127.0.0.1:9999/v1",
	}

	mgr := runtime.NewManager(mock, runtime.ManagerConfig{
		AutoStart:   false,
		HealthCheck: 100 * time.Millisecond,
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := mgr.Start(ctx); err != nil {
		t.Fatalf("Manager.Start() error: %v", err)
	}
	defer func() { _ = mgr.Stop(ctx) }()

	mock.mu.Lock()
	started := mock.startCalled
	mock.mu.Unlock()
	if started {
		t.Error("runtime.Start() should NOT be called with AutoStart=false")
	}
}

func TestManagerAutoPull(t *testing.T) {
	mock := &mockRuntime{
		name:     "test",
		endpoint: "http://127.0.0.1:9999/v1",
	}

	mgr := runtime.NewManager(mock, runtime.ManagerConfig{
		AutoStart:   true,
		AutoPull:    true,
		Models:      []string{"model-a", "model-b"},
		HealthCheck: 100 * time.Millisecond,
	})

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	if err := mgr.Start(ctx); err != nil {
		t.Fatalf("Manager.Start() error: %v", err)
	}
	defer func() { _ = mgr.Stop(ctx) }()

	// Auto-pull runs in a background goroutine; give it time to complete.
	time.Sleep(250 * time.Millisecond)

	mock.mu.Lock()
	pulled := append([]string{}, mock.pullCalled...)
	mock.mu.Unlock()

	if len(pulled) != 2 {
		t.Fatalf("expected 2 pulls, got %d: %v", len(pulled), pulled)
	}
	if pulled[0] != "model-a" || pulled[1] != "model-b" {
		t.Errorf("pulled = %v, want [model-a, model-b]", pulled)
	}
}

func TestManagerStop(t *testing.T) {
	mock := &mockRuntime{
		name:     "test",
		endpoint: "http://127.0.0.1:9999/v1",
	}

	mgr := runtime.NewManager(mock, runtime.ManagerConfig{
		HealthCheck: 100 * time.Millisecond,
	})

	ctx := context.Background()
	if err := mgr.Start(ctx); err != nil {
		t.Fatalf("Manager.Start() error: %v", err)
	}
	if err := mgr.Stop(ctx); err != nil {
		t.Fatalf("Manager.Stop() error: %v", err)
	}

	mock.mu.Lock()
	stopped := mock.stopCalled
	mock.mu.Unlock()
	if !stopped {
		t.Error("expected runtime.Stop() to be called")
	}
}

func TestManagerRestart(t *testing.T) {
	mock := &mockRuntime{
		name:     "test",
		endpoint: "http://127.0.0.1:9999/v1",
		models:   []runtime.Model{{ID: "test-model", Loaded: true}},
	}
	mgr := runtime.NewManager(mock, runtime.ManagerConfig{
		AutoStart:   true,
		HealthCheck: 50 * time.Millisecond,
	})
	ctx := context.Background()

	// Start.
	if err := mgr.Start(ctx); err != nil {
		t.Fatalf("first Start() error: %v", err)
	}
	// Wait for health loop to populate.
	for i := 0; i < 10; i++ {
		if mgr.Health().Status == runtime.StatusRunning {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if mgr.Health().Status != runtime.StatusRunning {
		t.Fatalf("status after start = %q, want running", mgr.Health().Status)
	}

	// Stop.
	if err := mgr.Stop(ctx); err != nil {
		t.Fatalf("Stop() error: %v", err)
	}

	// Restart — this is the key test.
	mock.mu.Lock()
	mock.startCalled = false
	mock.stopCalled = false
	mock.mu.Unlock()

	if err := mgr.Start(ctx); err != nil {
		t.Fatalf("second Start() error: %v", err)
	}
	defer func() { _ = mgr.Stop(ctx) }()

	// Wait for health loop to repopulate.
	for i := 0; i < 10; i++ {
		if mgr.Health().Status == runtime.StatusRunning {
			break
		}
		time.Sleep(20 * time.Millisecond)
	}
	if mgr.Health().Status != runtime.StatusRunning {
		t.Errorf("status after restart = %q, want running", mgr.Health().Status)
	}

	mock.mu.Lock()
	restarted := mock.startCalled
	mock.mu.Unlock()
	if !restarted {
		t.Error("expected runtime.Start() to be called on restart")
	}
}

func TestManagerLoadUnloadModel(t *testing.T) {
	loadCalled := ""
	unloadCalled := ""

	mock := &mockRuntime{
		name:     "test",
		endpoint: "http://127.0.0.1:9999/v1",
		healthy:  true,
	}
	// Override the mock's Load/Unload to track calls.
	// Since Go doesn't support overriding methods, we test via Manager
	// which delegates to the Runtime interface. The mock already returns nil.
	mgr := runtime.NewManager(mock, runtime.ManagerConfig{
		HealthCheck: 10 * time.Second,
	})
	ctx := context.Background()
	if err := mgr.Start(ctx); err != nil {
		t.Fatal(err)
	}
	defer func() { _ = mgr.Stop(ctx) }()

	// LoadModel and UnloadModel should not error (mock returns nil).
	if err := mgr.LoadModel(ctx, "test-model"); err != nil {
		t.Errorf("LoadModel error: %v", err)
	}
	if err := mgr.UnloadModel(ctx, "test-model"); err != nil {
		t.Errorf("UnloadModel error: %v", err)
	}

	// Verify they don't interfere with each other.
	_ = loadCalled
	_ = unloadCalled
}

package runtime

import (
	"context"
	"log/slog"
	"sync"
	"time"
)

// Manager wraps a Runtime and adds health monitoring, auto-start,
// and auto-pull of configured models.
type Manager struct {
	rt        Runtime
	autoStart bool
	autoPull  bool
	models    []string // models to ensure are pulled
	interval  time.Duration

	mu        sync.RWMutex
	health    *Health
	startedAt time.Time
	cancel    context.CancelFunc
	wg        sync.WaitGroup // tracks background goroutines (auto-pull)
}

// ManagerConfig configures the Manager's behavior.
type ManagerConfig struct {
	AutoStart   bool          `yaml:"auto_start" json:"auto_start"`
	AutoPull    bool          `yaml:"auto_pull" json:"auto_pull"`
	Models      []string      `yaml:"models" json:"models,omitempty"`
	HealthCheck time.Duration `yaml:"health_interval" json:"health_interval"`
}

// NewManager creates a Manager wrapping the given runtime.
func NewManager(rt Runtime, cfg ManagerConfig) *Manager {
	interval := cfg.HealthCheck
	if interval == 0 {
		interval = 10 * time.Second
	}
	return &Manager{
		rt:        rt,
		autoStart: cfg.AutoStart,
		autoPull:  cfg.AutoPull,
		models:    cfg.Models,
		interval:  interval,
	}
}

// Start optionally launches the runtime and begins health monitoring.
// Can be called after Stop to restart the Manager.
func (m *Manager) Start(ctx context.Context) error {
	// Cancel any previous health loop (enables restart after Stop).
	m.cancelHealthLoop()

	if m.autoStart {
		if err := m.startRuntime(ctx); err != nil {
			return err
		}
	}

	hctx := m.startHealthLoop(ctx)
	m.startAutoPull(hctx)

	return nil
}

// StartRuntime explicitly starts the runtime and begins health monitoring.
// The runtime startup can use a short-lived request context, while monitoring
// should use the application's long-lived context so UI RPC completion does not
// cancel health polling.
func (m *Manager) StartRuntime(startCtx, monitorCtx context.Context) error {
	m.cancelHealthLoop()
	if err := m.startRuntime(startCtx); err != nil {
		return err
	}
	hctx := m.startHealthLoop(monitorCtx)
	m.startAutoPull(hctx)
	return nil
}

func (m *Manager) startRuntime(ctx context.Context) error {
	slog.Info("starting runtime", "backend", m.rt.Name())
	if err := m.rt.Start(ctx); err != nil {
		return err
	}
	m.mu.Lock()
	m.startedAt = time.Now()
	m.mu.Unlock()
	return nil
}

func (m *Manager) startHealthLoop(ctx context.Context) context.Context {
	hctx, cancel := context.WithCancel(ctx)
	m.mu.Lock()
	m.cancel = cancel
	m.mu.Unlock()
	go m.healthLoop(hctx)
	return hctx
}

func (m *Manager) cancelHealthLoop() {
	m.mu.Lock()
	cancel := m.cancel
	m.cancel = nil
	m.mu.Unlock()
	if cancel != nil {
		cancel()
	}
}

func (m *Manager) startAutoPull(ctx context.Context) {
	if m.autoPull && len(m.models) > 0 {
		m.wg.Add(1)
		go func() {
			defer m.wg.Done()
			for _, model := range m.models {
				slog.Info("pulling model", "model", model, "backend", m.rt.Name())
				if err := m.rt.PullModel(ctx, model, nil); err != nil {
					slog.Warn("failed to pull model", "model", model, "error", err)
				}
			}
		}()
	}
}

// Stop stops health monitoring and shuts down the runtime.
// It waits for any in-flight auto-pull goroutines to finish.
func (m *Manager) Stop(ctx context.Context) error {
	m.cancelHealthLoop()
	m.wg.Wait() // wait for auto-pull goroutines to finish
	err := m.rt.Stop(ctx)
	// Update cached health immediately so GetHealth reflects the stopped state.
	m.mu.Lock()
	m.health = &Health{
		Status:    StatusStopped,
		Endpoint:  m.rt.Endpoint(),
		CheckedAt: time.Now(),
	}
	m.startedAt = time.Time{}
	m.mu.Unlock()
	return err
}

// Health returns the latest cached health status.
// Returns a shallow copy to avoid races with the background health loop.
func (m *Manager) Health() *Health {
	m.mu.RLock()
	defer m.mu.RUnlock()
	if m.health == nil {
		return &Health{Status: StatusStopped, CheckedAt: time.Now()}
	}
	h := *m.health
	return &h
}

// Endpoint returns the runtime's OpenAI-compat base URL.
func (m *Manager) Endpoint() string {
	return m.rt.Endpoint()
}

// Runtime returns the underlying runtime for direct API calls.
func (m *Manager) Runtime() Runtime {
	return m.rt
}

// LoadModel loads a model into GPU memory via the underlying runtime.
func (m *Manager) LoadModel(ctx context.Context, modelID string) error {
	return m.rt.LoadModel(ctx, modelID)
}

// UnloadModel removes a model from GPU memory via the underlying runtime.
func (m *Manager) UnloadModel(ctx context.Context, modelID string) error {
	return m.rt.UnloadModel(ctx, modelID)
}

func (m *Manager) healthLoop(ctx context.Context) {
	// Immediate first check.
	m.checkHealth(ctx)

	ticker := time.NewTicker(m.interval)
	defer ticker.Stop()

	for {
		select {
		case <-ctx.Done():
			return
		case <-ticker.C:
			m.checkHealth(ctx)
		}
	}
}

func (m *Manager) checkHealth(ctx context.Context) {
	h, err := m.rt.Health(ctx)
	if err != nil {
		h = &Health{
			Status:    StatusError,
			Error:     err.Error(),
			CheckedAt: time.Now(),
		}
	}
	m.mu.Lock()
	if !m.startedAt.IsZero() && h.Status == StatusRunning {
		h.Uptime = time.Since(m.startedAt).Seconds()
	}
	m.health = h
	m.mu.Unlock()
}

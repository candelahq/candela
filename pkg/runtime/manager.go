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
func (m *Manager) Start(ctx context.Context) error {
	if m.autoStart {
		slog.Info("starting runtime", "backend", m.rt.Name())
		if err := m.rt.Start(ctx); err != nil {
			return err
		}
		m.mu.Lock()
		m.startedAt = time.Now()
		m.mu.Unlock()
	}

	// Start health check loop.
	hctx, cancel := context.WithCancel(ctx)
	m.cancel = cancel
	go m.healthLoop(hctx)

	// Auto-pull configured models in the background.
	if m.autoPull && len(m.models) > 0 {
		go func() {
			for _, model := range m.models {
				slog.Info("pulling model", "model", model, "backend", m.rt.Name())
				if err := m.rt.PullModel(hctx, model, nil); err != nil {
					slog.Warn("failed to pull model", "model", model, "error", err)
				}
			}
		}()
	}

	return nil
}

// Stop stops health monitoring and shuts down the runtime.
func (m *Manager) Stop(ctx context.Context) error {
	if m.cancel != nil {
		m.cancel()
	}
	return m.rt.Stop(ctx)
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

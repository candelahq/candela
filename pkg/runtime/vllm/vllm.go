// Package vllm implements the Runtime interface for the vLLM inference
// server. vLLM is a static server — one model per process, specified at
// launch time. Switching models requires stopping and restarting.
//
// API reference: https://docs.vllm.ai/en/latest/serving/openai_compatible_server.html
package vllm

import (
	"context"
	"encoding/json"
	"fmt"
	"log/slog"
	"net/http"
	"os/exec"
	"time"

	"github.com/candelahq/candela/pkg/runtime"
)

func init() {
	runtime.Register("vllm", func(cfg runtime.Config) (runtime.Runtime, error) {
		return New(cfg)
	})
}

// Runtime manages a vLLM inference server.
type Runtime struct {
	host   string
	port   int
	binary string
	model  string // model is baked in at launch time
	args   []string
	cmd    *exec.Cmd
}

// New creates a vLLM runtime with the given config.
func New(cfg runtime.Config) (*Runtime, error) {
	model := ""
	if m, ok := cfg.Args["model"].(string); ok {
		model = m
	}

	// Collect extra CLI args.
	var args []string
	if gpuMem, ok := runtime.ConfigString(cfg.Args, "gpu_memory_utilization"); ok {
		args = append(args, "--gpu-memory-utilization", gpuMem)
	}
	if maxLen, ok := runtime.ConfigString(cfg.Args, "max_model_len"); ok {
		args = append(args, "--max-model-len", maxLen)
	}

	return &Runtime{
		host:   runtime.DefaultHost(cfg.Host),
		port:   runtime.DefaultPort(cfg.Port, 8000),
		binary: runtime.ConfigBinary(cfg.Args, "vllm"),
		model:  model,
		args:   args,
	}, nil
}

func (r *Runtime) Name() string { return "vllm" }

func (r *Runtime) Endpoint() string {
	return fmt.Sprintf("http://%s:%d/v1", r.host, r.port)
}

func (r *Runtime) baseURL() string {
	return fmt.Sprintf("http://%s:%d", r.host, r.port)
}

// Start launches `vllm serve <model>` and waits until ready.
func (r *Runtime) Start(ctx context.Context) error {
	if r.model == "" {
		return fmt.Errorf("vllm: model is required (set args.model in config)")
	}

	cmdArgs := []string{"serve", r.model, "--port", fmt.Sprintf("%d", r.port), "--host", r.host}
	cmdArgs = append(cmdArgs, r.args...)

	r.cmd = exec.CommandContext(ctx, r.binary, cmdArgs...)
	if err := r.cmd.Start(); err != nil {
		return fmt.Errorf("vllm: starting %q: %w", r.binary, err)
	}
	// vLLM can take a while to load large models.
	return runtime.WaitHealthy(ctx, r.baseURL()+"/health/ready", 2*time.Second, 5*time.Minute)
}

// Stop terminates the vLLM process.
func (r *Runtime) Stop(ctx context.Context) error {
	if r.cmd == nil || r.cmd.Process == nil {
		return nil
	}
	if err := r.cmd.Process.Kill(); err != nil {
		return fmt.Errorf("vllm: stop: %w", err)
	}
	_ = r.cmd.Wait()
	r.cmd = nil
	return nil
}

// Health checks if vLLM is reachable and the model is loaded.
// Note: GET /health returns 200 even while loading; /health/ready is authoritative.
func (r *Runtime) Health(ctx context.Context) (*runtime.Health, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, r.baseURL()+"/health/ready", nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return &runtime.Health{
			Status:    runtime.StatusStopped,
			Endpoint:  r.Endpoint(),
			CheckedAt: time.Now(),
		}, nil
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return &runtime.Health{
			Status:    runtime.StatusStarting,
			Endpoint:  r.Endpoint(),
			CheckedAt: time.Now(),
		}, nil
	}

	models, _ := r.ListModels(ctx)
	return &runtime.Health{
		Status:    runtime.StatusRunning,
		Endpoint:  r.Endpoint(),
		Models:    models,
		CheckedAt: time.Now(),
	}, nil
}

// ListModels returns the single model served by this vLLM instance.
func (r *Runtime) ListModels(ctx context.Context) ([]runtime.Model, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, r.Endpoint()+"/models", nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("vllm: list models: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	var result struct {
		Data []struct {
			ID      string `json:"id"`
			OwnedBy string `json:"owned_by"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("vllm: decode models: %w", err)
	}

	models := make([]runtime.Model, len(result.Data))
	for i, m := range result.Data {
		models[i] = runtime.Model{
			ID:     m.ID,
			Loaded: true, // vLLM always has its model loaded
		}
	}
	return models, nil
}

// PullModel is a no-op for vLLM — models are downloaded automatically
// from HuggingFace on first `vllm serve`. To change models, stop and
// restart with a different model name.
func (r *Runtime) PullModel(_ context.Context, modelID string, progress chan<- runtime.PullProgress) error {
	slog.Info("vllm: pull is a no-op — models are fetched at launch time",
		"model", modelID)
	if progress != nil {
		progress <- runtime.PullProgress{
			Status:  "done",
			Percent: 100,
		}
	}
	return nil
}

// LoadModel switches the model by stopping the current process and launching
// a new one with the requested model. Returns immediately — the caller should
// poll Health() for readiness (vLLM reports "starting" via /health/ready until
// the model is fully loaded).
//
// If the requested model is already loaded, this is a no-op.
func (r *Runtime) LoadModel(ctx context.Context, modelID string) error {
	if r.model == modelID && r.cmd != nil && r.cmd.Process != nil {
		slog.Info("vllm: model already loaded", "model", modelID)
		return nil
	}

	// Stop current instance if running.
	if err := r.Stop(ctx); err != nil {
		slog.Warn("vllm: failed to stop before model switch", "error", err)
	}

	// Update the model and restart.
	r.model = modelID
	cmdArgs := []string{"serve", r.model, "--port", fmt.Sprintf("%d", r.port), "--host", r.host}
	cmdArgs = append(cmdArgs, r.args...)

	r.cmd = exec.Command(r.binary, cmdArgs...)
	if err := r.cmd.Start(); err != nil {
		return fmt.Errorf("vllm: starting with model %q: %w", modelID, err)
	}

	slog.Info("vllm: loading model (async)", "model", modelID)
	return nil
}

// UnloadModel stops the vLLM process. vLLM can only serve one model per
// process, so unloading means stopping the server entirely.
func (r *Runtime) UnloadModel(ctx context.Context, modelID string) error {
	slog.Info("vllm: unloading model (stopping process)", "model", modelID)
	return r.Stop(ctx)
}

// DeleteModel is not supported by vLLM (models live in HuggingFace cache).
func (r *Runtime) DeleteModel(_ context.Context, _ string) error {
	return fmt.Errorf("vllm: delete not supported (remove from HuggingFace cache manually)")
}

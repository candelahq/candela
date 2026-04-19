// Package lmstudio implements the Runtime interface for LM Studio.
// LM Studio is a multi-model manager with explicit load/unload/download
// APIs and supports both a desktop app and headless server (lms/llmster).
//
// API reference: https://lmstudio.ai/docs/api
package lmstudio

import (
	"bytes"
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
	runtime.Register("lmstudio", func(cfg runtime.Config) (runtime.Runtime, error) {
		return New(cfg)
	})
}

// Runtime manages an LM Studio inference server.
type Runtime struct {
	host   string
	port   int
	binary string // "lms" (CLI) or "llmster" (headless)
	cmd    *exec.Cmd
}

// New creates an LM Studio runtime with the given config.
func New(cfg runtime.Config) (*Runtime, error) {
	return &Runtime{
		host:   runtime.DefaultHost(cfg.Host),
		port:   runtime.DefaultPort(cfg.Port, 1234),
		binary: runtime.ConfigBinary(cfg.Args, "lms"),
	}, nil
}

func (r *Runtime) Name() string { return "lmstudio" }

func (r *Runtime) Endpoint() string {
	return fmt.Sprintf("http://%s:%d/v1", r.host, r.port)
}

func (r *Runtime) baseURL() string {
	return fmt.Sprintf("http://%s:%d", r.host, r.port)
}

// Start launches the LM Studio server.
func (r *Runtime) Start(ctx context.Context) error {
	r.cmd = exec.CommandContext(ctx, r.binary, "server", "start",
		"--port", fmt.Sprintf("%d", r.port))
	if err := r.cmd.Start(); err != nil {
		return fmt.Errorf("lmstudio: starting %q: %w", r.binary, err)
	}
	return runtime.WaitHealthy(ctx, r.baseURL(), 500*time.Millisecond, 30*time.Second)
}

// Stop shuts down the LM Studio server.
func (r *Runtime) Stop(ctx context.Context) error {
	// Try graceful CLI stop first.
	stopCmd := exec.CommandContext(ctx, r.binary, "server", "stop")
	if err := stopCmd.Run(); err != nil {
		slog.Warn("lmstudio: graceful stop failed, killing process", "error", err)
		if r.cmd != nil && r.cmd.Process != nil {
			_ = r.cmd.Process.Kill()
			_ = r.cmd.Wait()
		}
	}
	r.cmd = nil
	return nil
}

// Health checks if LM Studio is reachable.
func (r *Runtime) Health(ctx context.Context) (*runtime.Health, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, r.baseURL()+"/v1/models", nil)
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

	models, _ := r.ListModels(ctx)
	return &runtime.Health{
		Status:    runtime.StatusRunning,
		Endpoint:  r.Endpoint(),
		Models:    models,
		CheckedAt: time.Now(),
	}, nil
}

// ListModels returns models available in LM Studio.
func (r *Runtime) ListModels(ctx context.Context) ([]runtime.Model, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, r.baseURL()+"/v1/models", nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("lmstudio: list models: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	var result struct {
		Data []struct {
			ID      string `json:"id"`
			OwnedBy string `json:"owned_by"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("lmstudio: decode models: %w", err)
	}

	models := make([]runtime.Model, len(result.Data))
	for i, m := range result.Data {
		models[i] = runtime.Model{
			ID:     m.ID,
			Loaded: true,
		}
	}
	return models, nil
}

// PullModel downloads a model via LM Studio's API.
func (r *Runtime) PullModel(ctx context.Context, modelID string, progress chan<- runtime.PullProgress) error {
	payload, err := json.Marshal(map[string]string{"model": modelID})
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		r.baseURL()+"/api/v1/models/download", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("lmstudio: pull %q: %w", modelID, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("lmstudio: pull %q: status %d", modelID, resp.StatusCode)
	}

	if progress != nil {
		progress <- runtime.PullProgress{
			Status:  "done",
			Percent: 100,
		}
	}

	slog.Info("model download initiated", "model", modelID, "backend", "lmstudio")
	return nil
}

// LoadModel loads a model into GPU memory using the lms CLI.
func (r *Runtime) LoadModel(ctx context.Context, modelID string) error {
	cmd := exec.CommandContext(ctx, r.binary, "load", modelID)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("lmstudio: load model %q: %w\n%s", modelID, err, output)
	}
	slog.Info("model loaded", "model", modelID, "backend", "lmstudio")
	return nil
}

// UnloadModel removes a model from GPU memory using the lms CLI.
func (r *Runtime) UnloadModel(ctx context.Context, modelID string) error {
	cmd := exec.CommandContext(ctx, r.binary, "unload", modelID)
	if output, err := cmd.CombinedOutput(); err != nil {
		return fmt.Errorf("lmstudio: unload model %q: %w\n%s", modelID, err, output)
	}
	slog.Info("model unloaded", "model", modelID, "backend", "lmstudio")
	return nil
}

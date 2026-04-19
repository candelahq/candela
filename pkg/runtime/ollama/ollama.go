// Package ollama implements the Runtime interface for the Ollama inference
// server. Ollama is a lazy-loading runtime — it manages its own model
// loading into VRAM and supports multiple models simultaneously.
//
// API reference: https://github.com/ollama/ollama/blob/main/docs/api.md
package ollama

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"os/exec"
	"strings"
	"time"

	"github.com/candelahq/candela/pkg/runtime"
)

func init() {
	runtime.Register("ollama", func(cfg runtime.Config) (runtime.Runtime, error) {
		return New(cfg)
	})
}

// Runtime manages an Ollama inference server.
type Runtime struct {
	host       string
	port       int
	binary     string
	cmd        *exec.Cmd
	httpClient *http.Client
}

// New creates an Ollama runtime with the given config.
func New(cfg runtime.Config) (*Runtime, error) {
	return &Runtime{
		host:       runtime.DefaultHost(cfg.Host),
		port:       runtime.DefaultPort(cfg.Port, 11434),
		binary:     runtime.ConfigBinary(cfg.Args, "ollama"),
		httpClient: &http.Client{Timeout: 30 * time.Second},
	}, nil
}

func (r *Runtime) Name() string { return "ollama" }

func (r *Runtime) Endpoint() string {
	return fmt.Sprintf("http://%s:%d/v1", r.host, r.port)
}

func (r *Runtime) baseURL() string {
	return fmt.Sprintf("http://%s:%d", r.host, r.port)
}

// Start launches `ollama serve` and waits until the server is healthy.
func (r *Runtime) Start(ctx context.Context) error {
	r.cmd = exec.CommandContext(ctx, r.binary, "serve")
	r.cmd.Env = append(r.cmd.Environ(),
		fmt.Sprintf("OLLAMA_HOST=%s:%d", r.host, r.port),
	)
	if err := r.cmd.Start(); err != nil {
		return fmt.Errorf("ollama: starting %q: %w", r.binary, err)
	}
	return runtime.WaitHealthy(ctx, r.baseURL(), 500*time.Millisecond, 30*time.Second)
}

// Stop terminates the Ollama process.
func (r *Runtime) Stop(ctx context.Context) error {
	if r.cmd == nil || r.cmd.Process == nil {
		return nil
	}
	if err := r.cmd.Process.Kill(); err != nil {
		return fmt.Errorf("ollama: stop: %w", err)
	}
	// Wait to avoid zombie processes.
	_ = r.cmd.Wait()
	r.cmd = nil
	return nil
}

// Health checks if Ollama is reachable and returns its status.
func (r *Runtime) Health(ctx context.Context) (*runtime.Health, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, r.baseURL(), nil)
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

// ollamaTagsResponse is the JSON response from GET /api/tags.
type ollamaTagsResponse struct {
	Models []struct {
		Name       string    `json:"name"`
		Model      string    `json:"model"`
		ModifiedAt time.Time `json:"modified_at"`
		Size       int64     `json:"size"`
		Details    struct {
			Family        string `json:"family"`
			ParameterSize string `json:"parameter_size"`
			Quantization  string `json:"quantization_level"`
		} `json:"details"`
	} `json:"models"`
}

// ListModels returns all locally available models.
func (r *Runtime) ListModels(ctx context.Context) ([]runtime.Model, error) {
	req, err := http.NewRequestWithContext(ctx, http.MethodGet, r.baseURL()+"/api/tags", nil)
	if err != nil {
		return nil, err
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("ollama: list models: %w", err)
	}
	defer func() { _ = resp.Body.Close() }()

	var result ollamaTagsResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("ollama: decode tags: %w", err)
	}

	models := make([]runtime.Model, len(result.Models))
	for i, m := range result.Models {
		models[i] = runtime.Model{
			ID:           m.Name,
			SizeBytes:    m.Size,
			Family:       m.Details.Family,
			Parameters:   m.Details.ParameterSize,
			Quantization: m.Details.Quantization,
		}
	}
	return models, nil
}

// PullModel downloads a model from the Ollama registry.
// Progress updates are streamed via NDJSON from the Ollama API.
func (r *Runtime) PullModel(ctx context.Context, modelID string, progress chan<- runtime.PullProgress) error {
	body := fmt.Sprintf(`{"name": %q, "stream": true}`, modelID)
	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		r.baseURL()+"/api/pull", strings.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return fmt.Errorf("ollama: pull %q: %w", modelID, err)
	}
	defer func() { _ = resp.Body.Close() }()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(io.LimitReader(resp.Body, 1024))
		return fmt.Errorf("ollama: pull %q: status %d: %s", modelID, resp.StatusCode, respBody)
	}

	// Stream NDJSON progress.
	dec := json.NewDecoder(resp.Body)
	for dec.More() {
		var line struct {
			Status    string `json:"status"`
			Completed int64  `json:"completed"`
			Total     int64  `json:"total"`
		}
		if err := dec.Decode(&line); err != nil {
			break
		}
		if progress != nil {
			pct := 0.0
			if line.Total > 0 {
				pct = float64(line.Completed) / float64(line.Total) * 100
			}
			select {
			case progress <- runtime.PullProgress{
				Status:    line.Status,
				Completed: line.Completed,
				Total:     line.Total,
				Percent:   pct,
			}:
			case <-ctx.Done():
				return ctx.Err()
			}
		}
	}

	slog.Info("model pulled", "model", modelID, "backend", "ollama")
	return nil
}

// LoadModel loads a model into GPU memory by sending a generate request
// with keep_alive set to -1 (infinite). The model will stay loaded until
// explicitly unloaded or the server is stopped.
func (r *Runtime) LoadModel(ctx context.Context, modelID string) error {
	return r.sendKeepAlive(ctx, modelID, -1, "load")
}

// UnloadModel removes a model from GPU memory by sending a generate request
// with keep_alive set to 0 (immediate eviction).
func (r *Runtime) UnloadModel(ctx context.Context, modelID string) error {
	return r.sendKeepAlive(ctx, modelID, 0, "unload")
}

// sendKeepAlive sends a /api/generate request to control model VRAM residency.
func (r *Runtime) sendKeepAlive(ctx context.Context, modelID string, keepAlive int, action string) error {
	reqBody := struct {
		Model     string `json:"model"`
		KeepAlive int    `json:"keep_alive"`
	}{
		Model:     modelID,
		KeepAlive: keepAlive,
	}
	payload, err := json.Marshal(reqBody)
	if err != nil {
		return err
	}

	req, err := http.NewRequestWithContext(ctx, http.MethodPost,
		r.baseURL()+"/api/generate", bytes.NewReader(payload))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")

	resp, err := r.httpClient.Do(req)
	if err != nil {
		return fmt.Errorf("ollama: %s model %q: %w", action, modelID, err)
	}
	defer func() { _ = resp.Body.Close() }()
	// Drain the response body (Ollama streams NDJSON even for empty generates).
	_, _ = io.Copy(io.Discard, resp.Body)

	if resp.StatusCode != http.StatusOK {
		return fmt.Errorf("ollama: %s model %q: status %d", action, modelID, resp.StatusCode)
	}
	slog.Info("model "+action+"ed", "model", modelID, "backend", "ollama")
	return nil
}

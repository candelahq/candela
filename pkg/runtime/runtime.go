// Package runtime defines the Runtime interface for managing local LLM
// inference servers (Ollama, vLLM, LM Studio). It provides a common
// abstraction over lifecycle management, health monitoring, and model
// operations.
//
// Usage:
//
//	import (
//	    "github.com/candelahq/candela/pkg/runtime"
//	    _ "github.com/candelahq/candela/pkg/runtime/ollama" // register
//	)
//
//	rt, err := runtime.New("ollama", runtime.Config{
//	    Host: "127.0.0.1",
//	    Port: 11434,
//	})
package runtime

import (
	"context"
	"time"
)

// Status represents the current state of a runtime.
type Status string

const (
	StatusStopped  Status = "stopped"
	StatusStarting Status = "starting"
	StatusRunning  Status = "running"
	StatusError    Status = "error"
)

// Model describes a model available in the runtime.
type Model struct {
	ID           string    `json:"id"`
	SizeBytes    int64     `json:"size_bytes,omitempty"`
	Family       string    `json:"family,omitempty"`
	Parameters   string    `json:"parameters,omitempty"`
	Quantization string    `json:"quantization,omitempty"`
	Loaded       bool      `json:"loaded"`
	LastUsed     time.Time `json:"last_used,omitempty"`
}

// Health holds the current health status of a runtime.
type Health struct {
	Status    Status    `json:"status"`
	Endpoint  string    `json:"endpoint"`
	Uptime    float64   `json:"uptime_seconds"`
	Models    []Model   `json:"models,omitempty"`
	Error     string    `json:"error,omitempty"`
	CheckedAt time.Time `json:"checked_at"`
}

// PullProgress reports model download progress.
type PullProgress struct {
	Status    string  `json:"status"`              // "pulling", "downloading", "verifying", "done"
	Completed int64   `json:"completed,omitempty"` // bytes downloaded
	Total     int64   `json:"total,omitempty"`     // total bytes
	Percent   float64 `json:"percent,omitempty"`
}

// Runtime manages a local inference server's lifecycle.
type Runtime interface {
	// Name returns the runtime identifier ("ollama", "vllm", "lmstudio").
	Name() string

	// Start launches the runtime process. Blocks until healthy or ctx expires.
	Start(ctx context.Context) error

	// Stop gracefully shuts down the runtime.
	Stop(ctx context.Context) error

	// Health returns the current health status.
	Health(ctx context.Context) (*Health, error)

	// Endpoint returns the OpenAI-compat base URL
	// (e.g. "http://127.0.0.1:11434/v1").
	Endpoint() string

	// ListModels returns models available in the runtime.
	ListModels(ctx context.Context) ([]Model, error)

	// PullModel downloads a model. Returns immediately if already present.
	// The progress channel receives status updates; it may be nil if the
	// caller doesn't need progress updates.
	PullModel(ctx context.Context, modelID string, progress chan<- PullProgress) error
}

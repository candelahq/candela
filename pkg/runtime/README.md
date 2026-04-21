# pkg/runtime — Local LLM Runtime Abstraction

This package provides a unified Go interface for managing local LLM inference
servers. Candela uses it to treat **Ollama**, **vLLM**, and **LM Studio** as
interchangeable backends behind a single API surface.

## Architecture Overview

```
┌───────────────────────────────────────────────────────────┐
│                     runtime.Manager                       │
│  (health monitoring, auto-start, auto-pull, lifecycle)    │
└──────────────────────────┬────────────────────────────────┘
                           │ wraps
              ┌────────────▼────────────┐
              │     runtime.Runtime     │  ← interface
              │  (Start, Stop, Health,  │
              │   ListModels, Pull,     │
              │   Load, Unload, Delete) │
              └────────────┬────────────┘
                           │ implemented by
         ┌─────────────────┼─────────────────┐
         ▼                 ▼                 ▼
   ollama.Runtime    vllm.Runtime    lmstudio.Runtime
   (port 11434)      (port 8000)     (port 1234)
```

## Core Interface

Defined in [`runtime.go`](runtime.go):

```go
type Runtime interface {
    Name() string                              // "ollama", "vllm", "lmstudio"
    Start(ctx context.Context) error           // Launch the server process
    Stop(ctx context.Context) error            // Shut down the process
    Health(ctx context.Context) (*Health, error)
    Endpoint() string                          // OpenAI-compat base URL (e.g. http://127.0.0.1:11434/v1)
    ListModels(ctx context.Context) ([]Model, error)
    PullModel(ctx context.Context, modelID string, progress chan<- PullProgress) error
    LoadModel(ctx context.Context, modelID string) error   // Pin model in GPU VRAM
    UnloadModel(ctx context.Context, modelID string) error // Evict from VRAM
    DeleteModel(ctx context.Context, modelID string) error // Remove from disk
}
```

Every backend exposes an **OpenAI-compatible `/v1` endpoint** via `Endpoint()`.
This means `candela-local` can route `/v1/chat/completions` to any backend
without caring which server is running underneath.

## Registry Pattern

Backends self-register via `init()` using a factory function
([`registry.go`](registry.go)):

```go
// In pkg/runtime/ollama/ollama.go
func init() {
    runtime.Register("ollama", func(cfg runtime.Config) (runtime.Runtime, error) {
        return New(cfg)
    })
}
```

Callers create runtimes by name:

```go
import (
    "github.com/candelahq/candela/pkg/runtime"
    _ "github.com/candelahq/candela/pkg/runtime/ollama"   // register
    _ "github.com/candelahq/candela/pkg/runtime/vllm"     // register
    _ "github.com/candelahq/candela/pkg/runtime/lmstudio" // register
)

rt, err := runtime.New("ollama", runtime.Config{
    Host: "127.0.0.1",
    Port: 11434,
})
```

`runtime.Names()` returns all registered backend names (sorted).

## Backends

### Ollama (`pkg/runtime/ollama`)

| Property | Value |
|----------|-------|
| Default port | `11434` |
| Binary | `ollama` |
| Model management | Lazy-loading; manages its own model storage |
| API surface | Native API (`/api/tags`, `/api/ps`, `/api/pull`, `/api/generate`, `/api/delete`) for management; `/v1` for inference |

Ollama is a **lazy-loading** runtime — it manages model downloading and VRAM
loading automatically. `LoadModel` / `UnloadModel` use the `keep_alive`
parameter on `/api/generate` to pin or evict models from GPU memory.

### vLLM (`pkg/runtime/vllm`)

| Property | Value |
|----------|-------|
| Default port | `8000` |
| Binary | `vllm` |
| Model management | Single-model; requires restart to switch models |
| API surface | OpenAI-compatible `/v1` + `/health/ready` |

vLLM is a **single-model** runtime. Loading a different model requires
stopping and restarting the process with the new model. Callers should
poll `Health()` after `LoadModel()` to wait for readiness.

### LM Studio (`pkg/runtime/lmstudio`)

| Property | Value |
|----------|-------|
| Default port | `1234` |
| Binary | `lms` |
| Model management | Via `lms` CLI (`lms load`, `lms unload`, `lms ls`) |
| API surface | OpenAI-compatible `/v1` + `/api/v1` management |

LM Studio uses its `lms` CLI for lifecycle and model management.

## Manager

[`manager.go`](manager.go) wraps a `Runtime` with operational concerns:

- **Health monitoring** — Background goroutine polls `Health()` at a
  configurable interval (default: 10s) and caches the result.
- **Auto-start** — Optionally launches the runtime on `Manager.Start()`.
- **Auto-pull** — Optionally pulls a list of configured models after start.
- **Thread-safe** — All state is protected by `sync.RWMutex`.

```go
mgr := runtime.NewManager(rt, runtime.ManagerConfig{
    AutoStart:   true,
    AutoPull:    true,
    Models:      []string{"llama3.2:3b", "mistral:7b"},
    HealthCheck: 15 * time.Second,
})

mgr.Start(ctx)           // Start runtime + health loop + auto-pull
defer mgr.Stop(ctx)      // Stop runtime + cancel health loop

h := mgr.Health()        // Latest cached health
rt := mgr.Runtime()      // Access underlying Runtime directly
```

## Discovery

[`discovery.go`](discovery.go) scans `$PATH` for known runtime binaries:

```go
backends := runtime.Discover()
// → [{Name:"ollama", Installed:true, BinaryPath:"/opt/homebrew/bin/ollama", InstallHint:"brew install ollama"}, ...]
```

This powers the **backend discovery** card in the `candela-local` management
UI (`/_local/`), showing install status and platform-specific install hints.

## Shared Helpers

[`helpers.go`](helpers.go) provides utilities shared across all backends:

| Function | Purpose |
|----------|---------|
| `WaitHealthy(ctx, url, interval, timeout)` | Poll a URL until HTTP 200 or timeout — used by `Start()` |
| `DefaultHost(host)` | Returns `"127.0.0.1"` if empty |
| `DefaultPort(port, fallback)` | Returns fallback if port is zero |
| `ConfigBinary(args, fallback)` | Extracts `"binary"` from `Config.Args` map |
| `ConfigString(args, key)` | Extracts a string from `Args`, handling YAML number coercion |

## Adding a New Backend

1. Create a new package: `pkg/runtime/mybackend/`

2. Implement the `Runtime` interface:

```go
// pkg/runtime/mybackend/mybackend.go
package mybackend

import (
	"fmt"

	"github.com/candelahq/candela/pkg/runtime"
)

func init() {
    runtime.Register("mybackend", func(cfg runtime.Config) (runtime.Runtime, error) {
        return New(cfg)
    })
}

type Runtime struct { /* ... */ }

func New(cfg runtime.Config) (*Runtime, error) {
    return &Runtime{
        host:   runtime.DefaultHost(cfg.Host),
        port:   runtime.DefaultPort(cfg.Port, 9999),
        binary: runtime.ConfigBinary(cfg.Args, "mybackend"),
    }, nil
}

func (r *Runtime) Name() string     { return "mybackend" }
func (r *Runtime) Endpoint() string { return fmt.Sprintf("http://%s:%d/v1", r.host, r.port) }
// ... implement remaining interface methods
```

3. Add discovery metadata in [`discovery.go`](discovery.go):

```go
var knownBackends = map[string]backendMeta{
    // ... existing entries ...
    "mybackend": {
        binary: "mybackend",
        installHint: map[string]string{
            "darwin": "brew install mybackend",
            "linux":  "apt install mybackend",
        },
    },
}
```

4. Import the package in `cmd/candela-local` (blank import for `init()`
   registration):

```go
import _ "github.com/candelahq/candela/pkg/runtime/mybackend"
```

5. Add tests — see existing `*_test.go` files for patterns using
   `httptest.NewServer` to mock the backend API.

## Testing

```bash
# All runtime tests
nix develop -c go test ./pkg/runtime/... -v

# Specific backend
nix develop -c go test ./pkg/runtime/ollama/... -v
```

Tests use `httptest.NewServer` to mock backend HTTP APIs — no real Ollama /
vLLM / LM Studio process is needed.

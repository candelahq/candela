package runtime

import (
	"errors"
	"fmt"
	"sort"
	"sync"
)

// Config holds runtime configuration from YAML.
type Config struct {
	Host string         `yaml:"host" json:"host"`
	Port int            `yaml:"port" json:"port"`
	Args map[string]any `yaml:"args" json:"args,omitempty"` // backend-specific extra config
}

// Factory creates a Runtime from the given config.
type Factory func(cfg Config) (Runtime, error)

var (
	// ErrAlreadyRegistered is returned by Register when a backend name is already in use.
	ErrAlreadyRegistered = errors.New("runtime: backend already registered")

	mu       sync.RWMutex
	registry = map[string]Factory{}
)

// Register adds a runtime factory. It returns ErrAlreadyRegistered if the backend name
// has already been registered, or an error if the factory is nil. Unlike MustRegister,
// Register does not panic, making it safe for dynamic registration and test environments.
func Register(name string, f Factory) error {
	if f == nil {
		return fmt.Errorf("runtime: factory cannot be nil for %q", name)
	}
	mu.Lock()
	defer mu.Unlock()
	if _, dup := registry[name]; dup {
		return fmt.Errorf("%w: %q", ErrAlreadyRegistered, name)
	}
	registry[name] = f
	return nil
}

// MustRegister adds a runtime factory. It is typically called from init() in each
// implementation package (e.g. pkg/runtime/ollama, pkg/runtime/vllm).
//
// Panics if the factory is nil or if the backend name has already been registered.
// For dynamic registration where duplicates should be handled gracefully as errors,
// use Register instead.
func MustRegister(name string, f Factory) {
	if err := Register(name, f); err != nil {
		panic(fmt.Sprintf("runtime: %v", err))
	}
}

// UnregisterForTesting removes a runtime factory. It is intended only for tests
// to reset state and avoid cross-test pollution.
func UnregisterForTesting(name string) {
	mu.Lock()
	defer mu.Unlock()
	delete(registry, name)
}

// New creates a runtime by name using the registered factory.
func New(name string, cfg Config) (Runtime, error) {
	mu.RLock()
	f, ok := registry[name]
	mu.RUnlock()
	if !ok {
		return nil, fmt.Errorf("runtime: unknown backend %q (registered: %v)", name, Names())
	}
	return f(cfg)
}

// Names returns all registered runtime backend names, sorted.
func Names() []string {
	mu.RLock()
	defer mu.RUnlock()
	out := make([]string, 0, len(registry))
	for k := range registry {
		out = append(out, k)
	}
	sort.Strings(out)
	return out
}

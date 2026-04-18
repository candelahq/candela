package runtime

import (
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
	mu       sync.RWMutex
	registry = map[string]Factory{}
)

// Register adds a runtime factory. Typically called from init() in each
// implementation package (e.g. pkg/runtime/ollama).
func Register(name string, f Factory) {
	mu.Lock()
	defer mu.Unlock()
	if _, dup := registry[name]; dup {
		panic(fmt.Sprintf("runtime: Register called twice for %q", name))
	}
	registry[name] = f
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

package cloudauth

import (
	"fmt"
	"sync"
)

// registry holds all registered cloud auth providers.
var (
	registryMu sync.RWMutex
	registry   = make(map[string]Provider)
)

func init() {
	// Register built-in providers.
	Register(NewGCPProvider())
}

// Register adds a Provider to the global registry.
// It is safe to call from init() or at runtime.
func Register(p Provider) {
	registryMu.Lock()
	defer registryMu.Unlock()
	registry[p.Name()] = p
}

// Get returns the Provider for the given name, or an error if not found.
func Get(name string) (Provider, error) {
	registryMu.RLock()
	defer registryMu.RUnlock()
	p, ok := registry[name]
	if !ok {
		return nil, fmt.Errorf("unknown cloud auth provider: %q (available: %v)", name, Names())
	}
	return p, nil
}

// Names returns the names of all registered providers.
func Names() []string {
	registryMu.RLock()
	defer registryMu.RUnlock()
	names := make([]string, 0, len(registry))
	for name := range registry {
		names = append(names, name)
	}
	return names
}

// All returns all registered providers.
func All() []Provider {
	registryMu.RLock()
	defer registryMu.RUnlock()
	providers := make([]Provider, 0, len(registry))
	for _, p := range registry {
		providers = append(providers, p)
	}
	return providers
}

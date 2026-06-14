package catalog

import (
	"context"
	"strings"
)

// ConfigStore is a read-only ModelCatalogStore backed by a static slice
// of entries. It is used for compiled-in defaults and YAML/JSON config.
type ConfigStore struct {
	entries []Entry
}

// NewConfigStore creates a ConfigStore from the given entries.
// The slice is copied so the caller can safely mutate the original after construction.
func NewConfigStore(entries []Entry) *ConfigStore {
	cp := make([]Entry, len(entries))
	copy(cp, entries)
	return &ConfigStore{entries: cp}
}

// List returns catalog entries. When includeDisabled is false, only
// entries with Enabled==true are returned.
func (s *ConfigStore) List(_ context.Context, includeDisabled bool) ([]Entry, error) {
	if includeDisabled {
		out := make([]Entry, len(s.entries))
		copy(out, s.entries)
		return out, nil
	}

	out := make([]Entry, 0, len(s.entries))
	for _, e := range s.entries {
		if e.Enabled {
			out = append(out, e)
		}
	}
	return out, nil
}

// Get returns a single entry by provider and model ID (case-insensitive).
// Returns ErrNotFound if no matching entry exists.
func (s *ConfigStore) Get(_ context.Context, provider, modelID string) (*Entry, error) {
	for _, e := range s.entries {
		if strings.EqualFold(e.Provider, provider) && strings.EqualFold(e.ModelID, modelID) {
			cp := e
			return &cp, nil
		}
	}
	return nil, ErrNotFound
}

// Update always returns ErrReadOnly — ConfigStore is immutable.
func (s *ConfigStore) Update(_ context.Context, _ Entry) error {
	return ErrReadOnly
}

// Delete always returns ErrReadOnly — ConfigStore is immutable.
func (s *ConfigStore) Delete(_ context.Context, _, _ string) error {
	return ErrReadOnly
}

// Source returns "config".
func (s *ConfigStore) Source() string { return "config" }

// Writable returns false — ConfigStore does not support mutations.
func (s *ConfigStore) Writable() bool { return false }

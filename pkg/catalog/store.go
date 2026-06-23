// Package catalog defines the ModelCatalogStore interface — the pluggable
// abstraction for listing, looking up, and updating model catalog entries.
//
// Implementations include:
//   - ConfigStore: read-only, backed by static config / compiled-in defaults
//   - (future) FirestoreStore: read-write, backed by Cloud Firestore
package catalog

import (
	"context"
	"errors"

	domain "github.com/candelahq/candela/gen/go/candela/types/domain"
)

// Sentinel errors returned by ModelCatalogStore implementations.
var (
	// ErrNotFound indicates that the requested model entry does not exist.
	ErrNotFound = errors.New("catalog: not found")

	// ErrReadOnly indicates that the store does not support mutations.
	ErrReadOnly = errors.New("catalog: read-only store")
)

// Entry represents a single model in the catalog.
// This is a type alias for the proto2type-generated domain type,
// keeping backward compatibility while the struct definition lives
// in the proto schema.
type Entry = domain.ModelCatalogEntry

// ModelCatalogStore is the core abstraction for the model catalog.
// Read-only stores (config, compiled-in defaults) return ErrReadOnly from Update.
// Read-write stores (Firestore) support full CRUD.
type ModelCatalogStore interface {
	// List returns catalog entries. When includeDisabled is false, only
	// entries with Enabled==true are returned.
	List(ctx context.Context, includeDisabled bool) ([]Entry, error)

	// Get returns a single entry by provider and model ID.
	// Returns ErrNotFound if no matching entry exists.
	Get(ctx context.Context, provider, modelID string) (*Entry, error)

	// Update creates or replaces an entry. Read-only stores return ErrReadOnly.
	Update(ctx context.Context, entry Entry) error

	// Delete removes an entry by provider and model ID.
	// Returns ErrNotFound if no matching entry exists.
	// Read-only stores return ErrReadOnly.
	Delete(ctx context.Context, provider, modelID string) error

	// Source returns a human-readable label for this store (e.g. "config", "firestore").
	Source() string

	// Writable reports whether this store supports Update/Delete.
	Writable() bool
}

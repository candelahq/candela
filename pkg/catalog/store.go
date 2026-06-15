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
	"time"
)

// Sentinel errors returned by ModelCatalogStore implementations.
var (
	// ErrNotFound indicates that the requested model entry does not exist.
	ErrNotFound = errors.New("catalog: not found")

	// ErrReadOnly indicates that the store does not support mutations.
	ErrReadOnly = errors.New("catalog: read-only store")
)

// Entry represents a single model in the catalog.
// This is a plain Go struct with no protobuf dependency — proto conversion
// lives in the handler layer.
type Entry struct {
	// ModelID is the canonical model identifier (e.g. "gemini-2.5-pro").
	ModelID  string `json:"model_id"`
	Provider string `json:"provider"`

	// DisplayName is a human-friendly label (e.g. "Gemini 2.5 Pro").
	DisplayName string `json:"display_name,omitempty"`

	// Pricing — base tier (USD per 1M tokens).
	InputPerMillion  float64 `json:"input_per_million"`
	OutputPerMillion float64 `json:"output_per_million"`

	// Enabled controls whether the model is available for routing.
	Enabled bool `json:"enabled"`

	// Category groups models (e.g. "flagship", "lite", "reasoning").
	Category string `json:"category,omitempty"`

	// ContextWindow is the maximum context length in tokens.
	ContextWindow int64 `json:"context_window,omitempty"`

	// Tiered pricing — high tier (optional).
	InputPerMillionHigh  float64 `json:"input_per_million_high,omitempty"`
	OutputPerMillionHigh float64 `json:"output_per_million_high,omitempty"`
	TierThresholdTokens  int64   `json:"tier_threshold_tokens,omitempty"`

	// Aliases are alternative model names that resolve to this entry.
	Aliases []string `json:"aliases,omitempty"`

	// AllowedTenants restricts this model to specific tenant IDs.
	// An empty slice means "available to all tenants".
	AllowedTenants []string `json:"allowed_tenants,omitempty"`

	// DiscountPercent is a model-specific discount (0.0–1.0).
	DiscountPercent float64   `json:"discount_percent,omitempty"`
	UpdatedAt       time.Time `json:"updated_at,omitempty"` // last modification timestamp

	// ProviderModelID is the upstream provider's model identifier when it
	// differs from ModelID. For example, Anthropic uses "claude-opus-4.7"
	// but Vertex AI requires "claude-opus-4-7" (dashes instead of dots).
	// If empty, ModelID is used as-is for upstream requests.
	ProviderModelID string `json:"provider_model_id,omitempty"`

	// Region is the cloud region to use for this specific model.
	// For Vertex AI, this determines the endpoint (e.g. "global", "us-east5").
	// If empty, falls back to the deployment-wide CANDELA_VERTEX_REGION setting.
	Region string `json:"region,omitempty"`
}

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

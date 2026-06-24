package proxy

import (
	"context"
	"sync"
	"testing"

	"github.com/candelahq/candela/pkg/catalog"
)

// productionResolver returns a ModelResolver closure that exactly matches the
// production wiring in cmd/candela-server/main.go (~line 548). The production
// code does NOT filter by Enabled — disabled entries still resolve.
func productionResolver(store *catalog.ConfigStore) func(string) (string, string) {
	return func(model string) (string, string) {
		entry, err := store.Get(context.Background(), "anthropic", model)
		if err != nil || entry == nil {
			return "", ""
		}
		return entry.ProviderModelID, entry.Region
	}
}

// TestCatalogProxyModelResolver_Integration verifies the end-to-end wiring
// between a catalog.ConfigStore and VertexAIPathRewriter.ModelResolver,
// mirroring the production closure in cmd/candela-server/main.go.
//
// This covers:
//   - dot→dash conversion via ProviderModelID
//   - region override from catalog entries
//   - date-suffixed model resolution (Display name lookup)
//   - fallback when the model is not in the catalog
//   - disabled model resolution (production does NOT filter by Enabled)
//   - region-only override (empty ProviderModelID)
//   - streaming vs non-streaming paths
func TestCatalogProxyModelResolver_Integration(t *testing.T) {
	// ── Arrange: build a catalog store with test entries ─────────────
	store := catalog.NewConfigStore([]catalog.Entry{
		{
			ModelID:         "claude-opus-4.7",
			Provider:        "anthropic",
			ProviderModelID: "claude-opus-4-7", // dot→dash
			Region:          "us-east5",
			Enabled:         true,
		},
		{
			ModelID:         "claude-haiku-4.5",
			Provider:        "anthropic",
			ProviderModelID: "claude-haiku-4-5",
			Region:          "global",
			Enabled:         true,
		},
		{
			ModelID:         "claude-sonnet-4",
			Provider:        "anthropic",
			ProviderModelID: "", // no model ID override — region only
			Region:          "global",
			Enabled:         true,
		},
		{
			// Disabled entry — production does NOT filter by Enabled, so this
			// still resolves via the catalog. The region is intentionally
			// different from the default to verify the catalog is actually used.
			ModelID:         "claude-old-3",
			Provider:        "anthropic",
			ProviderModelID: "claude-old-3",
			Region:          "europe-west1",
			Enabled:         false,
		},
		{
			// Entry with empty ProviderModelID AND empty Region — should use
			// original model name and default region (both fields empty means
			// the resolver returns ("", ""), and RewritePath falls back).
			ModelID:         "claude-bare",
			Provider:        "anthropic",
			ProviderModelID: "",
			Region:          "",
			Enabled:         true,
		},
	})

	// ── Build the ModelResolver closure (same pattern as main.go) ───
	resolver := productionResolver(store)

	rewriter := &VertexAIPathRewriter{
		ProjectID:     "test-project",
		Region:        "us-central1", // default region
		ModelResolver: resolver,
	}

	tests := []struct {
		name      string
		model     string
		streaming bool
		want      string
	}{
		{
			name:  "dot to dash conversion with region override",
			model: "claude-opus-4.7",
			want:  "/v1/projects/test-project/locations/us-east5/publishers/anthropic/models/claude-opus-4-7:rawPredict",
		},
		{
			name:  "haiku dot to dash with global region",
			model: "claude-haiku-4.5",
			want:  "/v1/projects/test-project/locations/global/publishers/anthropic/models/claude-haiku-4-5:rawPredict",
		},
		{
			name:  "region override only — no model ID change",
			model: "claude-sonnet-4",
			want:  "/v1/projects/test-project/locations/global/publishers/anthropic/models/claude-sonnet-4:rawPredict",
		},
		{
			name:  "fallback — model not in catalog uses defaults",
			model: "claude-unknown-99",
			want:  "/v1/projects/test-project/locations/us-central1/publishers/anthropic/models/claude-unknown-99:rawPredict",
		},
		{
			// Production does NOT filter disabled entries. The catalog entry
			// still resolves, so the disabled model uses its catalog region
			// (europe-west1), not the default (us-central1).
			name:  "disabled model still resolves via catalog",
			model: "claude-old-3",
			want:  "/v1/projects/test-project/locations/europe-west1/publishers/anthropic/models/claude-old-3:rawPredict",
		},
		{
			name:      "streaming with catalog resolution",
			model:     "claude-opus-4.7",
			streaming: true,
			want:      "/v1/projects/test-project/locations/us-east5/publishers/anthropic/models/claude-opus-4-7:streamRawPredict",
		},
		{
			name:  "date-suffixed model resolves via Display name",
			model: "claude-opus-4.7-20250514",
			want:  "/v1/projects/test-project/locations/us-east5/publishers/anthropic/models/claude-opus-4-7@20250514:rawPredict",
		},
		{
			name:  "date-suffixed sonnet — region only, suffix preserved",
			model: "claude-sonnet-4-20250514",
			want:  "/v1/projects/test-project/locations/global/publishers/anthropic/models/claude-sonnet-4@20250514:rawPredict",
		},
		{
			// Disabled entry with date suffix — still resolves via catalog.
			name:  "date-suffixed disabled model still resolves",
			model: "claude-old-3-20240101",
			want:  "/v1/projects/test-project/locations/europe-west1/publishers/anthropic/models/claude-old-3@20240101:rawPredict",
		},
		{
			// Entry exists but both ProviderModelID and Region are empty.
			// The resolver returns ("", ""), so RewritePath falls back to
			// the raw model name and default region.
			name:  "empty ProviderModelID and empty Region — falls back to defaults",
			model: "claude-bare",
			want:  "/v1/projects/test-project/locations/us-central1/publishers/anthropic/models/claude-bare:rawPredict",
		},
		{
			name:      "streaming fallback — unknown model",
			model:     "claude-unknown-99",
			streaming: true,
			want:      "/v1/projects/test-project/locations/us-central1/publishers/anthropic/models/claude-unknown-99:streamRawPredict",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := rewriter.RewritePath(tt.model, tt.streaming)
			if got != tt.want {
				t.Errorf("RewritePath(%q, %v)\n  got:  %s\n  want: %s",
					tt.model, tt.streaming, got, tt.want)
			}
		})
	}
}

// TestCatalogProxyModelResolver_NilResolver verifies that when ModelResolver
// is nil, RewritePath falls back to parsing the raw model name directly.
// This matches the production behavior when no catalog is configured.
func TestCatalogProxyModelResolver_NilResolver(t *testing.T) {
	rewriter := &VertexAIPathRewriter{
		ProjectID:     "test-project",
		Region:        "us-central1",
		ModelResolver: nil,
	}

	tests := []struct {
		name      string
		model     string
		streaming bool
		want      string
	}{
		{
			name:  "nil resolver — plain model",
			model: "claude-sonnet-4",
			want:  "/v1/projects/test-project/locations/us-central1/publishers/anthropic/models/claude-sonnet-4:rawPredict",
		},
		{
			name:  "nil resolver — date-suffixed model",
			model: "claude-sonnet-4-20250514",
			want:  "/v1/projects/test-project/locations/us-central1/publishers/anthropic/models/claude-sonnet-4@20250514:rawPredict",
		},
		{
			name:      "nil resolver — streaming",
			model:     "claude-sonnet-4",
			streaming: true,
			want:      "/v1/projects/test-project/locations/us-central1/publishers/anthropic/models/claude-sonnet-4:streamRawPredict",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := rewriter.RewritePath(tt.model, tt.streaming)
			if got != tt.want {
				t.Errorf("RewritePath(%q, %v)\n  got:  %s\n  want: %s",
					tt.model, tt.streaming, got, tt.want)
			}
		})
	}
}

// TestCatalogProxyModelResolver_CaseInsensitiveLookup verifies that the
// catalog's case-insensitive Get is correctly exercised through the resolver.
func TestCatalogProxyModelResolver_CaseInsensitiveLookup(t *testing.T) {
	store := catalog.NewConfigStore([]catalog.Entry{
		{
			ModelID:         "claude-opus-4.7",
			Provider:        "Anthropic", // mixed case
			ProviderModelID: "claude-opus-4-7",
			Region:          "us-east5",
			Enabled:         true,
		},
	})

	resolver := productionResolver(store)

	rewriter := &VertexAIPathRewriter{
		ProjectID:     "test-project",
		Region:        "us-central1",
		ModelResolver: resolver,
	}

	got := rewriter.RewritePath("claude-opus-4.7", false)
	want := "/v1/projects/test-project/locations/us-east5/publishers/anthropic/models/claude-opus-4-7:rawPredict"
	if got != want {
		t.Errorf("case-insensitive lookup:\n  got:  %s\n  want: %s", got, want)
	}
}

// TestCatalogProxyModelResolver_SpecialCharacters verifies that model names
// containing dots, underscores, and other non-alphanumeric characters are
// correctly round-tripped through the resolver and path rewriter.
func TestCatalogProxyModelResolver_SpecialCharacters(t *testing.T) {
	store := catalog.NewConfigStore([]catalog.Entry{
		{
			ModelID:         "model_with_underscores",
			Provider:        "anthropic",
			ProviderModelID: "model-with-dashes",
			Region:          "asia-southeast1",
			Enabled:         true,
		},
		{
			ModelID:         "model.with.many.dots.1.2.3",
			Provider:        "anthropic",
			ProviderModelID: "model-with-many-dots-1-2-3",
			Region:          "us-west1",
			Enabled:         true,
		},
	})

	resolver := productionResolver(store)

	rewriter := &VertexAIPathRewriter{
		ProjectID:     "test-project",
		Region:        "us-central1",
		ModelResolver: resolver,
	}

	tests := []struct {
		name  string
		model string
		want  string
	}{
		{
			name:  "underscores in model ID",
			model: "model_with_underscores",
			want:  "/v1/projects/test-project/locations/asia-southeast1/publishers/anthropic/models/model-with-dashes:rawPredict",
		},
		{
			name:  "multiple dots in model ID",
			model: "model.with.many.dots.1.2.3",
			want:  "/v1/projects/test-project/locations/us-west1/publishers/anthropic/models/model-with-many-dots-1-2-3:rawPredict",
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			got := rewriter.RewritePath(tt.model, false)
			if got != tt.want {
				t.Errorf("RewritePath(%q)\n  got:  %s\n  want: %s",
					tt.model, got, tt.want)
			}
		})
	}
}

// TestCatalogProxyModelResolver_ConcurrentResolution verifies that concurrent
// calls to RewritePath with a shared resolver and rewriter are safe.
// This is important because the production proxy handles many concurrent
// requests sharing the same VertexAIPathRewriter and ModelResolver closure.
func TestCatalogProxyModelResolver_ConcurrentResolution(t *testing.T) {
	store := catalog.NewConfigStore([]catalog.Entry{
		{
			ModelID:         "claude-opus-4.7",
			Provider:        "anthropic",
			ProviderModelID: "claude-opus-4-7",
			Region:          "us-east5",
			Enabled:         true,
		},
		{
			ModelID:         "claude-sonnet-4",
			Provider:        "anthropic",
			ProviderModelID: "",
			Region:          "global",
			Enabled:         true,
		},
	})

	resolver := productionResolver(store)

	rewriter := &VertexAIPathRewriter{
		ProjectID:     "test-project",
		Region:        "us-central1",
		ModelResolver: resolver,
	}

	models := []struct {
		model string
		want  string
	}{
		{"claude-opus-4.7", "/v1/projects/test-project/locations/us-east5/publishers/anthropic/models/claude-opus-4-7:rawPredict"},
		{"claude-sonnet-4", "/v1/projects/test-project/locations/global/publishers/anthropic/models/claude-sonnet-4:rawPredict"},
		{"claude-unknown", "/v1/projects/test-project/locations/us-central1/publishers/anthropic/models/claude-unknown:rawPredict"},
	}

	const goroutines = 50
	var wg sync.WaitGroup
	errors := make(chan string, goroutines*len(models))

	for range goroutines {
		for _, m := range models {
			wg.Add(1)
			go func(model, want string) {
				defer wg.Done()
				got := rewriter.RewritePath(model, false)
				if got != want {
					errors <- "RewritePath(" + model + "): got " + got + ", want " + want
				}
			}(m.model, m.want)
		}
	}

	wg.Wait()
	close(errors)

	for err := range errors {
		t.Error(err)
	}
}

// TestCatalogProxyModelResolver_DifferentProvider verifies that the resolver
// correctly returns empty strings when the model exists under a different
// provider than the one being queried. This ensures cross-provider entries
// don't leak into the wrong provider's resolution path.
func TestCatalogProxyModelResolver_DifferentProvider(t *testing.T) {
	store := catalog.NewConfigStore([]catalog.Entry{
		{
			ModelID:         "gemini-pro",
			Provider:        "google",
			ProviderModelID: "gemini-1.5-pro",
			Region:          "us-west1",
			Enabled:         true,
		},
	})

	// Resolver queries "anthropic" but the entry is under "google".
	resolver := productionResolver(store)

	rewriter := &VertexAIPathRewriter{
		ProjectID:     "test-project",
		Region:        "us-central1",
		ModelResolver: resolver,
	}

	// Should fall back to defaults since "gemini-pro" is not in "anthropic".
	got := rewriter.RewritePath("gemini-pro", false)
	want := "/v1/projects/test-project/locations/us-central1/publishers/anthropic/models/gemini-pro:rawPredict"
	if got != want {
		t.Errorf("cross-provider lookup should fall back:\n  got:  %s\n  want: %s", got, want)
	}
}

package proxy

import (
	"context"
	"testing"

	"github.com/candelahq/candela/pkg/catalog"
)

// TestCatalogProxyModelResolver_Integration verifies the end-to-end wiring
// between a catalog.ConfigStore and VertexAIPathRewriter.ModelResolver,
// mirroring the production closure in cmd/candela-server/main.go.
//
// This covers:
//   - dot→dash conversion via ProviderModelID
//   - region override from catalog entries
//   - date-suffixed model resolution (Display name lookup)
//   - fallback when the model is not in the catalog
//   - disabled model rejection (entry exists but Enabled=false)
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
			// Disabled entry — resolver should treat this as absent.
			ModelID:         "claude-old-3",
			Provider:        "anthropic",
			ProviderModelID: "claude-old-3",
			Region:          "us-central1",
			Enabled:         false,
		},
	})

	// ── Build the ModelResolver closure (same pattern as main.go) ───
	// The production code does not filter by Enabled; this test validates
	// a resolver variant that rejects disabled entries, as a disabled
	// model should not resolve to a provider endpoint.
	resolver := func(model string) (string, string) {
		entry, err := store.Get(context.Background(), "anthropic", model)
		if err != nil || entry == nil {
			return "", ""
		}
		if !entry.Enabled {
			return "", ""
		}
		return entry.ProviderModelID, entry.Region
	}

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
			name:  "disabled model rejected — falls back to defaults",
			model: "claude-old-3",
			want:  "/v1/projects/test-project/locations/us-central1/publishers/anthropic/models/claude-old-3:rawPredict",
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
			name:  "date-suffixed disabled model — still rejected",
			model: "claude-old-3-20240101",
			want:  "/v1/projects/test-project/locations/us-central1/publishers/anthropic/models/claude-old-3@20240101:rawPredict",
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

	resolver := func(model string) (string, string) {
		// Production uses lowercase "anthropic" — ConfigStore.Get is case-insensitive.
		entry, err := store.Get(context.Background(), "anthropic", model)
		if err != nil || entry == nil {
			return "", ""
		}
		return entry.ProviderModelID, entry.Region
	}

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

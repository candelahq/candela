package catalog_test

import (
	"context"
	"errors"
	"testing"

	"github.com/candelahq/candela/pkg/catalog"
)

// testEntries is the canonical fixture set used by all conformance tests.
// Two enabled entries + one disabled entry.
var testEntries = []catalog.Entry{
	{
		ModelID:          "gemini-2.5-pro",
		Provider:         "google",
		DisplayName:      "Gemini 2.5 Pro",
		InputPerMillion:  1.25,
		OutputPerMillion: 10.00,
		Enabled:          true,
		Category:         "flagship",
		ContextWindow:    1_000_000,
	},
	{
		ModelID:          "claude-sonnet-4",
		Provider:         "anthropic",
		DisplayName:      "Claude Sonnet 4",
		InputPerMillion:  3.00,
		OutputPerMillion: 15.00,
		Enabled:          true,
		Category:         "flagship",
		ContextWindow:    200_000,
	},
	{
		ModelID:          "gpt-4.1",
		Provider:         "openai",
		DisplayName:      "GPT-4.1",
		InputPerMillion:  2.00,
		OutputPerMillion: 8.00,
		Enabled:          false,
		Category:         "flagship",
		ContextWindow:    1_000_000,
	},
}

// runConformanceSuite exercises the full ModelCatalogStore contract.
// writable indicates whether the store supports Update.
func runConformanceSuite(t *testing.T, store catalog.ModelCatalogStore, writable bool) {
	t.Helper()
	ctx := context.Background()

	// ── List (enabled only) ──────────────────────────────────────────────
	t.Run("List_EnabledOnly", func(t *testing.T) {
		entries, err := store.List(ctx, false)
		if err != nil {
			t.Fatalf("List(includeDisabled=false): %v", err)
		}
		for _, e := range entries {
			if !e.Enabled {
				t.Errorf("List(includeDisabled=false) returned disabled entry: %s/%s", e.Provider, e.ModelID)
			}
		}
		if len(entries) != 2 {
			t.Errorf("List(includeDisabled=false): got %d entries, want 2", len(entries))
		}
	})

	// ── List (include disabled) ──────────────────────────────────────────
	t.Run("List_IncludeDisabled", func(t *testing.T) {
		entries, err := store.List(ctx, true)
		if err != nil {
			t.Fatalf("List(includeDisabled=true): %v", err)
		}
		if len(entries) != 3 {
			t.Errorf("List(includeDisabled=true): got %d entries, want 3", len(entries))
		}
	})

	// ── Get (found) ──────────────────────────────────────────────────────
	t.Run("Get_Found", func(t *testing.T) {
		e, err := store.Get(ctx, "google", "gemini-2.5-pro")
		if err != nil {
			t.Fatalf("Get(google, gemini-2.5-pro): %v", err)
		}
		if e.ModelID != "gemini-2.5-pro" {
			t.Errorf("ModelID = %q, want %q", e.ModelID, "gemini-2.5-pro")
		}
		if e.Provider != "google" {
			t.Errorf("Provider = %q, want %q", e.Provider, "google")
		}
		if e.InputPerMillion != 1.25 {
			t.Errorf("InputPerMillion = %v, want 1.25", e.InputPerMillion)
		}
	})

	// ── Get (not found) ──────────────────────────────────────────────────
	t.Run("Get_NotFound", func(t *testing.T) {
		_, err := store.Get(ctx, "google", "nonexistent-model")
		if !errors.Is(err, catalog.ErrNotFound) {
			t.Errorf("Get(nonexistent): got %v, want %v", err, catalog.ErrNotFound)
		}
	})

	// ── Source ────────────────────────────────────────────────────────────
	t.Run("Source", func(t *testing.T) {
		if src := store.Source(); src == "" {
			t.Error("Source() returned empty string")
		}
	})

	// ── Writable ─────────────────────────────────────────────────────────
	t.Run("Writable", func(t *testing.T) {
		if got := store.Writable(); got != writable {
			t.Errorf("Writable() = %v, want %v", got, writable)
		}
	})

	// ── Update ───────────────────────────────────────────────────────────
	t.Run("Update", func(t *testing.T) {
		entry := catalog.Entry{
			ModelID:          "test-model",
			Provider:         "test-provider",
			InputPerMillion:  1.00,
			OutputPerMillion: 2.00,
			Enabled:          true,
		}
		err := store.Update(ctx, entry)

		if writable {
			if err != nil {
				t.Fatalf("Update on writable store: %v", err)
			}
			// Verify the entry was persisted.
			got, err := store.Get(ctx, "test-provider", "test-model")
			if err != nil {
				t.Fatalf("Get after Update: %v", err)
			}
			if got.InputPerMillion != 1.00 {
				t.Errorf("InputPerMillion = %v, want 1.00", got.InputPerMillion)
			}
		} else {
			if !errors.Is(err, catalog.ErrReadOnly) {
				t.Errorf("Update on read-only store: got %v, want %v", err, catalog.ErrReadOnly)
			}
		}
	})
}

package catalog

import (
	"context"
	"errors"
	"testing"
)

// ──────────────────────────────────────────────────────────────────────────────
// AUDIT: ConfigStore edge cases
// ──────────────────────────────────────────────────────────────────────────────

func TestConfigStore_EmptyEntries(t *testing.T) {
	store := NewConfigStore(nil)
	entries, err := store.List(context.Background(), false)
	if err != nil {
		t.Fatalf("List on empty store: %v", err)
	}
	if len(entries) != 0 {
		t.Errorf("List on empty store returned %d entries, want 0", len(entries))
	}
}

func TestConfigStore_GetNotFound(t *testing.T) {
	store := NewConfigStore(nil)
	_, err := store.Get(context.Background(), "openai", "nonexistent")
	if !errors.Is(err, ErrNotFound) {
		t.Errorf("Get on empty store = %v, want ErrNotFound", err)
	}
}

func TestConfigStore_GetCaseInsensitive(t *testing.T) {
	entries := []Entry{
		{ModelID: "gpt-4o", Provider: "OpenAI", Enabled: true},
	}
	store := NewConfigStore(entries)
	got, err := store.Get(context.Background(), "openai", "GPT-4O")
	if err != nil {
		t.Fatalf("Get case-insensitive: %v", err)
	}
	if got.ModelID != "gpt-4o" {
		t.Errorf("Get case-insensitive returned %q, want %q", got.ModelID, "gpt-4o")
	}
}

func TestConfigStore_ListIncludeDisabled(t *testing.T) {
	entries := []Entry{
		{ModelID: "enabled", Provider: "test", Enabled: true},
		{ModelID: "disabled", Provider: "test", Enabled: false},
	}
	store := NewConfigStore(entries)

	all, err := store.List(context.Background(), true)
	if err != nil {
		t.Fatalf("List(includeDisabled=true): %v", err)
	}
	if len(all) != 2 {
		t.Errorf("List(includeDisabled=true) = %d entries, want 2", len(all))
	}

	enabled, err := store.List(context.Background(), false)
	if err != nil {
		t.Fatalf("List(includeDisabled=false): %v", err)
	}
	if len(enabled) != 1 {
		t.Errorf("List(includeDisabled=false) = %d entries, want 1", len(enabled))
	}
}

func TestConfigStore_UpdateReadOnly(t *testing.T) {
	store := NewConfigStore(nil)
	err := store.Update(context.Background(), Entry{ModelID: "test", Provider: "test"})
	if !errors.Is(err, ErrReadOnly) {
		t.Errorf("Update on read-only store = %v, want ErrReadOnly", err)
	}
}

func TestConfigStore_DeleteReadOnly(t *testing.T) {
	store := NewConfigStore(nil)
	err := store.Delete(context.Background(), "test", "test")
	if !errors.Is(err, ErrReadOnly) {
		t.Errorf("Delete on read-only store = %v, want ErrReadOnly", err)
	}
}

func TestConfigStore_Source(t *testing.T) {
	store := NewConfigStore(nil)
	if got := store.Source(); got != "config" {
		t.Errorf("Source() = %q, want %q", got, "config")
	}
}

func TestConfigStore_Writable(t *testing.T) {
	store := NewConfigStore(nil)
	if store.Writable() {
		t.Error("Writable() = true, want false")
	}
}

// ──────────────────────────────────────────────────────────────────────────────
// AUDIT: Entry validation edge cases
// ──────────────────────────────────────────────────────────────────────────────

func TestConfigStore_DuplicateEntries(t *testing.T) {
	// Two entries with the same ModelID/Provider — Last-write-wins on Get.
	entries := []Entry{
		{ModelID: "model", Provider: "test", InputPerMillion: 1.0, Enabled: true},
		{ModelID: "model", Provider: "test", InputPerMillion: 2.0, Enabled: true},
	}
	store := NewConfigStore(entries)
	got, err := store.Get(context.Background(), "test", "model")
	if err != nil {
		t.Fatalf("Get duplicate: %v", err)
	}
	// First match wins in ConfigStore.Get (linear scan).
	if got.InputPerMillion != 1.0 {
		t.Errorf("Get duplicate returned InputPerMillion=%v, want 1.0 (first match)", got.InputPerMillion)
	}
}

func TestConfigStore_MutationIsolation(t *testing.T) {
	original := []Entry{
		{ModelID: "model", Provider: "test", Enabled: true},
	}
	store := NewConfigStore(original)

	// Mutate original slice — store should be unaffected.
	original[0].ModelID = "mutated"
	got, err := store.Get(context.Background(), "test", "model")
	if err != nil {
		t.Fatalf("Get after mutation: %v", err)
	}
	if got.ModelID != "model" {
		t.Error("ConfigStore was affected by mutation of original slice")
	}
}

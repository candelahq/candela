package main

import (
	"context"
	"errors"
	"strings"
	"testing"

	"github.com/candelahq/candela/pkg/catalog"
)

// memStore is a minimal in-memory ModelCatalogStore for unit tests.
type memStore struct {
	entries map[string]catalog.Entry // key: "provider/model"
}

func newMemStore(initial ...catalog.Entry) *memStore {
	s := &memStore{entries: make(map[string]catalog.Entry, len(initial))}
	for _, e := range initial {
		s.entries[e.Provider+"/"+e.ModelID] = e
	}
	return s
}

func (s *memStore) List(_ context.Context, includeDisabled bool) ([]catalog.Entry, error) {
	out := make([]catalog.Entry, 0, len(s.entries))
	for _, e := range s.entries {
		if includeDisabled || e.Enabled {
			out = append(out, e)
		}
	}
	return out, nil
}

func (s *memStore) Get(_ context.Context, provider, modelID string) (*catalog.Entry, error) {
	e, ok := s.entries[provider+"/"+modelID]
	if !ok {
		return nil, catalog.ErrNotFound
	}
	return &e, nil
}

func (s *memStore) Update(_ context.Context, entry catalog.Entry) error {
	s.entries[entry.Provider+"/"+entry.ModelID] = entry
	return nil
}

func (s *memStore) Delete(_ context.Context, provider, modelID string) error {
	key := provider + "/" + modelID
	if _, ok := s.entries[key]; !ok {
		return catalog.ErrNotFound
	}
	delete(s.entries, key)
	return nil
}

func (s *memStore) Source() string { return "mem" }
func (s *memStore) Writable() bool { return true }

// ── seedEntries tests ────────────────────────────────────────────────────

func TestSeedEntries_OverwriteMode(t *testing.T) {
	store := newMemStore()

	entries := []catalog.Entry{
		{Provider: "google", ModelID: "gemini-2.5-pro", InputPerMillion: 1.25, OutputPerMillion: 10, Enabled: true},
		{Provider: "openai", ModelID: "gpt-4.1", InputPerMillion: 2.00, OutputPerMillion: 8, Enabled: true},
	}

	result, err := seedEntries(context.Background(), store, entries, false)
	if err != nil {
		t.Fatalf("seedEntries: %v", err)
	}

	if result.seeded != 2 {
		t.Errorf("seeded = %d, want 2", result.seeded)
	}
	if result.skipped != 0 {
		t.Errorf("skipped = %d, want 0", result.skipped)
	}
	if result.errors != 0 {
		t.Errorf("errors = %d, want 0", result.errors)
	}
	if len(store.entries) != 2 {
		t.Errorf("store has %d entries, want 2", len(store.entries))
	}
}

func TestSeedEntries_MergeSkipsExisting(t *testing.T) {
	// Pre-populate the store with one entry that also appears in the seed list.
	existing := catalog.Entry{
		Provider:        "google",
		ModelID:         "gemini-2.5-pro",
		InputPerMillion: 1.25, OutputPerMillion: 10,
		Enabled: true,
	}
	store := newMemStore(existing)

	entries := []catalog.Entry{
		// This one already exists — should be skipped.
		{Provider: "google", ModelID: "gemini-2.5-pro", InputPerMillion: 99.0, OutputPerMillion: 99.0, Enabled: true},
		// This one is new — should be created.
		{Provider: "openai", ModelID: "gpt-4.1", InputPerMillion: 2.00, OutputPerMillion: 8.0, Enabled: true},
	}

	result, err := seedEntries(context.Background(), store, entries, true)
	if err != nil {
		t.Fatalf("seedEntries: %v", err)
	}

	if result.seeded != 1 {
		t.Errorf("seeded = %d, want 1", result.seeded)
	}
	if result.skipped != 1 {
		t.Errorf("skipped = %d, want 1", result.skipped)
	}
	if result.errors != 0 {
		t.Errorf("errors = %d, want 0", result.errors)
	}

	// Verify the existing entry was NOT overwritten (price should still be 1.25).
	got, err := store.Get(context.Background(), "google", "gemini-2.5-pro")
	if err != nil {
		t.Fatalf("Get existing entry: %v", err)
	}
	if got.InputPerMillion != 1.25 {
		t.Errorf("existing entry InputPerMillion = %f, want 1.25 (was overwritten)", got.InputPerMillion)
	}

	// Verify the new entry was created.
	got, err = store.Get(context.Background(), "openai", "gpt-4.1")
	if err != nil {
		t.Fatalf("Get new entry: %v", err)
	}
	if got.InputPerMillion != 2.00 {
		t.Errorf("new entry InputPerMillion = %f, want 2.00", got.InputPerMillion)
	}
}

func TestSeedEntries_MergeAllExisting(t *testing.T) {
	// When all entries already exist, everything should be skipped.
	store := newMemStore(
		catalog.Entry{Provider: "a", ModelID: "m1", InputPerMillion: 1, OutputPerMillion: 1, Enabled: true},
		catalog.Entry{Provider: "b", ModelID: "m2", InputPerMillion: 2, OutputPerMillion: 2, Enabled: true},
	)

	entries := []catalog.Entry{
		{Provider: "a", ModelID: "m1", InputPerMillion: 1, OutputPerMillion: 1, Enabled: true},
		{Provider: "b", ModelID: "m2", InputPerMillion: 2, OutputPerMillion: 2, Enabled: true},
	}

	result, err := seedEntries(context.Background(), store, entries, true)
	if err != nil {
		t.Fatalf("seedEntries: %v", err)
	}
	if result.seeded != 0 {
		t.Errorf("seeded = %d, want 0", result.seeded)
	}
	if result.skipped != 2 {
		t.Errorf("skipped = %d, want 2", result.skipped)
	}
}

func TestSeedEntries_MergeEmptyStore(t *testing.T) {
	// When the store is empty, merge mode should insert everything.
	store := newMemStore()

	entries := []catalog.Entry{
		{Provider: "a", ModelID: "m1", InputPerMillion: 1, OutputPerMillion: 1, Enabled: true},
	}

	result, err := seedEntries(context.Background(), store, entries, true)
	if err != nil {
		t.Fatalf("seedEntries: %v", err)
	}
	if result.seeded != 1 {
		t.Errorf("seeded = %d, want 1", result.seeded)
	}
	if result.skipped != 0 {
		t.Errorf("skipped = %d, want 0", result.skipped)
	}
}

func TestSeedEntries_ContextCancelled(t *testing.T) {
	store := newMemStore()

	ctx, cancel := context.WithCancel(context.Background())
	cancel() // Cancel immediately.

	entries := []catalog.Entry{
		{Provider: "a", ModelID: "m1", InputPerMillion: 1, OutputPerMillion: 1, Enabled: true},
	}

	result, err := seedEntries(ctx, store, entries, false)
	if err != nil {
		t.Fatalf("seedEntries should not return error on ctx cancel: %v", err)
	}
	// Nothing should have been seeded because context was already cancelled.
	if result.seeded != 0 {
		t.Errorf("seeded = %d, want 0", result.seeded)
	}
}

// errStore wraps memStore but injects an error into List or Update.
type errStore struct {
	*memStore
	listErr   error
	updateErr error
}

func (s *errStore) List(ctx context.Context, includeDisabled bool) ([]catalog.Entry, error) {
	if s.listErr != nil {
		return nil, s.listErr
	}
	return s.memStore.List(ctx, includeDisabled)
}

func (s *errStore) Update(ctx context.Context, entry catalog.Entry) error {
	if s.updateErr != nil {
		return s.updateErr
	}
	return s.memStore.Update(ctx, entry)
}

func TestSeedEntries_MergeListError(t *testing.T) {
	// If List() fails in merge mode, seedEntries should return the error.
	store := &errStore{
		memStore: newMemStore(),
		listErr:  errors.New("firestore: permission denied"),
	}

	entries := []catalog.Entry{
		{Provider: "a", ModelID: "m1", InputPerMillion: 1, OutputPerMillion: 1, Enabled: true},
	}

	_, err := seedEntries(context.Background(), store, entries, true)
	if err == nil {
		t.Fatal("expected error from List failure, got nil")
	}
	if !strings.Contains(err.Error(), "listing existing entries for merge") {
		t.Errorf("error should wrap context: got %v", err)
	}
}

func TestSeedEntries_UpdateError(t *testing.T) {
	// If Update() fails, the entry should be counted as an error, not seeded.
	store := &errStore{
		memStore:  newMemStore(),
		updateErr: errors.New("firestore: quota exceeded"),
	}

	entries := []catalog.Entry{
		{Provider: "a", ModelID: "m1", InputPerMillion: 1, OutputPerMillion: 1, Enabled: true},
	}

	result, err := seedEntries(context.Background(), store, entries, false)
	if err != nil {
		t.Fatalf("seedEntries should not return error for per-entry Update failures: %v", err)
	}
	if result.errors != 1 {
		t.Errorf("errors = %d, want 1", result.errors)
	}
	if result.seeded != 0 {
		t.Errorf("seeded = %d, want 0", result.seeded)
	}
}

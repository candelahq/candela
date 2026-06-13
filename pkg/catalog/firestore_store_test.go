package catalog_test

import (
	"context"
	"os"
	"reflect"
	"testing"

	"cloud.google.com/go/firestore"
	"github.com/candelahq/candela/pkg/catalog"
)

// TestFirestoreStore_InterfaceCompliance is a compile-time check that
// FirestoreStore satisfies ModelCatalogStore.
var _ catalog.ModelCatalogStore = (*catalog.FirestoreStore)(nil)

func TestFirestoreStore_SourceAndWritable(t *testing.T) {
	// Source and Writable don't need a live Firestore client.
	store := catalog.NewFirestoreStore(nil, "")
	if got := store.Source(); got != "firestore" {
		t.Errorf("Source() = %q, want %q", got, "firestore")
	}
	if got := store.Writable(); !got {
		t.Error("Writable() = false, want true")
	}
}

func TestFirestoreStore_DefaultCollection(t *testing.T) {
	// Passing "" should use default; non-empty should be preserved.
	// We can't inspect the private field directly, but we can verify
	// construction doesn't panic with a nil client + empty collection.
	store := catalog.NewFirestoreStore(nil, "")
	if store == nil {
		t.Fatal("NewFirestoreStore returned nil")
	}
	store2 := catalog.NewFirestoreStore(nil, "custom_collection")
	if store2 == nil {
		t.Fatal("NewFirestoreStore with custom collection returned nil")
	}
}

func TestFirestoreStore_FieldMapping(t *testing.T) {
	// Verify that entryToFirestore -> firestoreToEntry round-trips correctly.
	// We test this indirectly via the conformance suite (which calls Update+Get),
	// but here we do a structural check of the firestoreEntry struct tags.

	// firestoreEntry is unexported, so we verify via the public Entry struct
	// field count. If Entry gains a field and firestoreEntry doesn't, the
	// conformance test will catch data loss. This test guards the field count.
	entryType := reflect.TypeOf(catalog.Entry{})
	expectedFields := 15 // ModelID, Provider, DisplayName, InputPerMillion,
	// OutputPerMillion, Enabled, Category, ContextWindow,
	// InputPerMillionHigh, OutputPerMillionHigh, TierThresholdTokens,
	// Aliases, AllowedTenants, DiscountPercent, UpdatedAt

	if got := entryType.NumField(); got != expectedFields {
		t.Errorf("Entry has %d fields, expected %d — "+
			"did you add a field to Entry without updating firestoreEntry?",
			got, expectedFields)
	}
}

// ── Emulator-based integration tests ─────────────────────────────────────────

// requireEmulator skips the test if the Firestore emulator is not running.
// Start it with: gcloud emulators firestore start --host-port=localhost:8086
func requireCatalogEmulator(t *testing.T) {
	t.Helper()
	host := os.Getenv("FIRESTORE_EMULATOR_HOST")
	if host == "" {
		t.Skip("FIRESTORE_EMULATOR_HOST not set — skipping Firestore catalog tests")
	}
}

func newCatalogTestStore(t *testing.T) *catalog.FirestoreStore {
	t.Helper()
	requireCatalogEmulator(t)

	ctx := context.Background()
	client, err := firestore.NewClient(ctx, "test-project")
	if err != nil {
		t.Fatalf("failed to create Firestore client: %v", err)
	}
	t.Cleanup(func() { _ = client.Close() })

	// Use a unique collection per test to avoid cross-test pollution.
	col := "model_catalog_test_" + t.Name()
	store := catalog.NewFirestoreStore(client, col)

	// Seed with the standard test entries.
	for _, e := range testEntries {
		if err := store.Update(ctx, e); err != nil {
			t.Fatalf("seeding entry %s/%s: %v", e.Provider, e.ModelID, err)
		}
	}

	return store
}

func TestFirestoreStore_Conformance(t *testing.T) {
	store := newCatalogTestStore(t)
	runConformanceSuite(t, store, true)
}

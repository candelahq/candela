package catalog

import (
	"context"
	"fmt"

	"cloud.google.com/go/firestore"
	"google.golang.org/api/iterator"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
)

const defaultCatalogCollection = "model_catalog"

// firestoreEntry is the Firestore-internal representation of a catalog Entry.
// It owns the firestore:" struct tags so the shared Entry type remains
// database-agnostic (no Firestore-specific annotations).
type firestoreEntry struct {
	ModelID              string   `firestore:"model_id"`
	Provider             string   `firestore:"provider"`
	DisplayName          string   `firestore:"display_name,omitempty"`
	InputPerMillion      float64  `firestore:"input_per_million"`
	OutputPerMillion     float64  `firestore:"output_per_million"`
	Enabled              bool     `firestore:"enabled"`
	Category             string   `firestore:"category,omitempty"`
	ContextWindow        int64    `firestore:"context_window,omitempty"`
	InputPerMillionHigh  float64  `firestore:"input_per_million_high,omitempty"`
	OutputPerMillionHigh float64  `firestore:"output_per_million_high,omitempty"`
	TierThresholdTokens  int64    `firestore:"tier_threshold_tokens,omitempty"`
	Aliases              []string `firestore:"aliases,omitempty"`
	AllowedTenants       []string `firestore:"allowed_tenants,omitempty"`
	DiscountPercent      float64  `firestore:"discount_percent,omitempty"`
}

func entryToFirestore(e *Entry) *firestoreEntry {
	return &firestoreEntry{
		ModelID:              e.ModelID,
		Provider:             e.Provider,
		DisplayName:          e.DisplayName,
		InputPerMillion:      e.InputPerMillion,
		OutputPerMillion:     e.OutputPerMillion,
		Enabled:              e.Enabled,
		Category:             e.Category,
		ContextWindow:        e.ContextWindow,
		InputPerMillionHigh:  e.InputPerMillionHigh,
		OutputPerMillionHigh: e.OutputPerMillionHigh,
		TierThresholdTokens:  e.TierThresholdTokens,
		Aliases:              e.Aliases,
		AllowedTenants:       e.AllowedTenants,
		DiscountPercent:      e.DiscountPercent,
	}
}

func firestoreToEntry(fe *firestoreEntry) Entry {
	return Entry{
		ModelID:              fe.ModelID,
		Provider:             fe.Provider,
		DisplayName:          fe.DisplayName,
		InputPerMillion:      fe.InputPerMillion,
		OutputPerMillion:     fe.OutputPerMillion,
		Enabled:              fe.Enabled,
		Category:             fe.Category,
		ContextWindow:        fe.ContextWindow,
		InputPerMillionHigh:  fe.InputPerMillionHigh,
		OutputPerMillionHigh: fe.OutputPerMillionHigh,
		TierThresholdTokens:  fe.TierThresholdTokens,
		Aliases:              fe.Aliases,
		AllowedTenants:       fe.AllowedTenants,
		DiscountPercent:      fe.DiscountPercent,
	}
}

// docID returns the deterministic Firestore document ID for a model entry.
// Format: {provider}_{model_id} (e.g. "google_gemini-2.5-pro").
func docID(provider, modelID string) string {
	return provider + "_" + modelID
}

// FirestoreStore is a read-write ModelCatalogStore backed by Cloud Firestore.
type FirestoreStore struct {
	client     *firestore.Client
	collection string
}

// NewFirestoreStore creates a Firestore-backed ModelCatalogStore.
// If collection is empty, it defaults to "model_catalog".
func NewFirestoreStore(client *firestore.Client, collection string) *FirestoreStore {
	if collection == "" {
		collection = defaultCatalogCollection
	}
	return &FirestoreStore{
		client:     client,
		collection: collection,
	}
}

// List returns catalog entries from Firestore. When includeDisabled is false,
// only entries with enabled==true are returned.
func (s *FirestoreStore) List(ctx context.Context, includeDisabled bool) ([]Entry, error) {
	col := s.client.Collection(s.collection)

	var iter *firestore.DocumentIterator
	if includeDisabled {
		iter = col.Documents(ctx)
	} else {
		iter = col.Where("enabled", "==", true).Documents(ctx)
	}
	defer iter.Stop()

	var entries []Entry
	for {
		snap, err := iter.Next()
		if err == iterator.Done {
			break
		}
		if err != nil {
			return nil, fmt.Errorf("catalog: listing firestore docs: %w", err)
		}
		var fe firestoreEntry
		if err := snap.DataTo(&fe); err != nil {
			return nil, fmt.Errorf("catalog: decoding doc %s: %w", snap.Ref.ID, err)
		}
		entries = append(entries, firestoreToEntry(&fe))
	}
	return entries, nil
}

// Get returns a single entry by provider and model ID.
// Returns ErrNotFound if no matching document exists.
func (s *FirestoreStore) Get(ctx context.Context, provider, modelID string) (*Entry, error) {
	id := docID(provider, modelID)
	snap, err := s.client.Collection(s.collection).Doc(id).Get(ctx)
	if err != nil {
		if status.Code(err) == codes.NotFound {
			return nil, ErrNotFound
		}
		return nil, fmt.Errorf("catalog: getting doc %s: %w", id, err)
	}
	var fe firestoreEntry
	if err := snap.DataTo(&fe); err != nil {
		return nil, fmt.Errorf("catalog: decoding doc %s: %w", id, err)
	}
	e := firestoreToEntry(&fe)
	return &e, nil
}

// Update creates or replaces an entry using Set with MergeAll (upsert).
func (s *FirestoreStore) Update(ctx context.Context, entry Entry) error {
	id := docID(entry.Provider, entry.ModelID)
	ref := s.client.Collection(s.collection).Doc(id)
	_, err := ref.Set(ctx, entryToFirestore(&entry), firestore.MergeAll)
	if err != nil {
		return fmt.Errorf("catalog: updating doc %s: %w", id, err)
	}
	return nil
}

// Source returns "firestore".
func (s *FirestoreStore) Source() string { return "firestore" }

// Writable returns true — FirestoreStore supports mutations.
func (s *FirestoreStore) Writable() bool { return true }

package connecthandlers

import (
	"context"
	"testing"
	"time"

	"github.com/candelahq/candela/pkg/storage"
)

// stubUserStore implements storage.UserStore for tests.
// Only GetUserByEmail is functional; all other methods panic to catch misuse.
type stubUserStore struct {
	storage.UserStore           // satisfies interface; panics on unimplemented methods
	calls             int
	result            *storage.UserRecord
	err               error
}

func (s *stubUserStore) GetUserByEmail(_ context.Context, _ string) (*storage.UserRecord, error) {
	s.calls++
	return s.result, s.err
}

// U4: second call returns cached result without calling Firestore.
func TestResolveUserID_CacheHit_NoFirestoreCall(t *testing.T) {
	resetUserIDCache()
	stub := &stubUserStore{result: &storage.UserRecord{ID: "uid-123"}}

	id1, err := resolveUserID(context.Background(), stub, "alice@example.com")
	if err != nil || id1 != "uid-123" {
		t.Fatalf("first call: got (%q, %v)", id1, err)
	}
	id2, err := resolveUserID(context.Background(), stub, "alice@example.com")
	if err != nil || id2 != "uid-123" {
		t.Fatalf("second call: got (%q, %v)", id2, err)
	}
	if stub.calls != 1 {
		t.Errorf("Firestore called %d times, want 1", stub.calls)
	}
}

// U5: expired entry triggers a fresh Firestore read.
func TestResolveUserID_ExpiredEntry_Refetches(t *testing.T) {
	resetUserIDCache()
	stub := &stubUserStore{result: &storage.UserRecord{ID: "uid-456"}}

	// Manually insert an already-expired entry.
	globalUserIDCache.mu.Lock()
	globalUserIDCache.entries["expired@example.com"] = userIDEntry{
		userID:    "old-id",
		found:     true,
		expiresAt: time.Now().Add(-1 * time.Second),
	}
	globalUserIDCache.mu.Unlock()

	id, err := resolveUserID(context.Background(), stub, "expired@example.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != "uid-456" {
		t.Errorf("got %q, want %q", id, "uid-456")
	}
	if stub.calls != 1 {
		t.Errorf("expected 1 Firestore call after expiry, got %d", stub.calls)
	}
}

// U6: email addresses with different casings share the same cache slot.
func TestResolveUserID_LowercasesKey(t *testing.T) {
	resetUserIDCache()
	stub := &stubUserStore{result: &storage.UserRecord{ID: "uid-789"}}

	_, _ = resolveUserID(context.Background(), stub, "User@Example.COM")
	id, err := resolveUserID(context.Background(), stub, "user@example.com")
	if err != nil || id != "uid-789" {
		t.Fatalf("got (%q, %v)", id, err)
	}
	// Both casings must share one cache slot → only 1 Firestore call.
	if stub.calls != 1 {
		t.Errorf("Firestore called %d times, want 1 (cache should share slot)", stub.calls)
	}
}

// U7: expired entries are pruned after each write, keeping the map bounded.
func TestResolveUserID_EvictsExpiredOnWrite(t *testing.T) {
	resetUserIDCache()

	// Pre-populate with several expired entries.
	globalUserIDCache.mu.Lock()
	for _, email := range []string{"a@x.com", "b@x.com", "c@x.com"} {
		globalUserIDCache.entries[email] = userIDEntry{
			userID:    "stale",
			found:     true,
			expiresAt: time.Now().Add(-time.Minute),
		}
	}
	globalUserIDCache.mu.Unlock()

	stub := &stubUserStore{result: &storage.UserRecord{ID: "new-uid"}}
	_, _ = resolveUserID(context.Background(), stub, "trigger@x.com")

	globalUserIDCache.mu.RLock()
	size := len(globalUserIDCache.entries)
	globalUserIDCache.mu.RUnlock()

	// Only the freshly written entry should remain (3 expired ones pruned).
	if size != 1 {
		t.Errorf("cache has %d entries after eviction, want 1", size)
	}
}

// U8: when GetUserByEmail returns nil (user not yet provisioned), the result is
// negatively cached and Firestore is NOT hammered on subsequent calls.
func TestResolveUserID_NilUser_NegativeCaches(t *testing.T) {
	resetUserIDCache()
	stub := &stubUserStore{result: nil, err: nil} // user not found

	id1, err := resolveUserID(context.Background(), stub, "new@example.com")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id1 != "" {
		t.Errorf("expected empty ID for not-found user, got %q", id1)
	}

	// Second call must hit the negative cache — no additional Firestore read.
	id2, _ := resolveUserID(context.Background(), stub, "new@example.com")
	if id2 != "" {
		t.Errorf("negative cache returned non-empty ID: %q", id2)
	}
	if stub.calls != 1 {
		t.Errorf("Firestore called %d times on not-found user, want 1 (negative cache)", stub.calls)
	}
}

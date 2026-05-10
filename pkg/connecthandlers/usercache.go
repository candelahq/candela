package connecthandlers

import (
	"context"
	"sync"
	"time"

	"github.com/candelahq/candela/pkg/storage"
)

// userIDCache is an in-process cache from email → Firestore user ID.
//
// Every handler call (GetMyUsage, GetMyBudget, GetTeamLeaderboard, …) resolves
// the caller's email to a Firestore doc ID via GetUserByEmail. For a user that
// calls the dashboard every 5 seconds, that's 12 Firestore reads per minute
// just for identity resolution. This cache eliminates the redundant reads.
//
// TTL is intentionally short (60s) so:
//   - Provisioning delays resolve quickly after a new user is created.
//   - Email changes (rare) propagate within a minute.
//
// The cache is safe for concurrent use and has no dependency on external state.
type userIDCache struct {
	mu      sync.RWMutex
	entries map[string]userIDEntry
}

type userIDEntry struct {
	userID    string
	expiresAt time.Time
}

const userIDCacheTTL = 60 * time.Second

var globalUserIDCache = &userIDCache{
	entries: make(map[string]userIDEntry),
}

// resolveUserID returns the Firestore user ID for the given email, using a
// 60-second in-process cache to avoid hitting Firestore on every API call.
func resolveUserID(ctx context.Context, users storage.UserStore, email string) (string, error) {
	if users == nil {
		return "", nil
	}

	// Fast path: cache hit.
	globalUserIDCache.mu.RLock()
	if e, ok := globalUserIDCache.entries[email]; ok && time.Now().Before(e.expiresAt) {
		globalUserIDCache.mu.RUnlock()
		return e.userID, nil
	}
	globalUserIDCache.mu.RUnlock()

	// Slow path: Firestore lookup.
	user, err := users.GetUserByEmail(ctx, email)
	if err != nil {
		return "", err
	}

	// Cache the result.
	globalUserIDCache.mu.Lock()
	globalUserIDCache.entries[email] = userIDEntry{
		userID:    user.ID,
		expiresAt: time.Now().Add(userIDCacheTTL),
	}
	globalUserIDCache.mu.Unlock()

	return user.ID, nil
}

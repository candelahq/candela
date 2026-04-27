// Package session provides pluggable session resolution for grouping
// LLM requests into conversations. The default chain resolver tries
// an explicit X-Session-ID header first, then falls back to a
// message-prefix heuristic that fingerprints the first user message.
package session

import (
	"crypto/sha256"
	"encoding/hex"
	"encoding/json"
	"net/http"
	"sync"
	"time"

	"github.com/google/uuid"
)

// SessionInfo carries the request context needed by resolvers.
type SessionInfo struct {
	UserID   string
	Model    string
	Messages json.RawMessage // raw JSON messages array
	Headers  http.Header
}

// SessionResolver assigns a session ID to a proxied request.
type SessionResolver interface {
	Resolve(info SessionInfo) string
}

// ── Chain Resolver ──

// ChainResolver tries resolvers in order; first non-empty result wins.
type ChainResolver struct {
	resolvers []SessionResolver
}

// NewChainResolver creates a resolver that tries each resolver in order.
func NewChainResolver(resolvers ...SessionResolver) *ChainResolver {
	return &ChainResolver{resolvers: resolvers}
}

func (c *ChainResolver) Resolve(info SessionInfo) string {
	for _, r := range c.resolvers {
		if id := r.Resolve(info); id != "" {
			return id
		}
	}
	return uuid.NewString()
}

// ── Header Resolver ──

const defaultHeaderName = "X-Session-Id"

// HeaderResolver reads the session ID from an HTTP header.
type HeaderResolver struct {
	headerName string
}

// NewHeaderResolver creates a resolver that reads the given header.
// If headerName is empty, defaults to "X-Session-Id".
func NewHeaderResolver(headerName string) *HeaderResolver {
	if headerName == "" {
		headerName = defaultHeaderName
	}
	return &HeaderResolver{headerName: headerName}
}

func (h *HeaderResolver) Resolve(info SessionInfo) string {
	if info.Headers == nil {
		return ""
	}
	return info.Headers.Get(h.headerName)
}

// ── User Message Resolver ──

// sessionEntry tracks an active session in the in-memory cache.
type sessionEntry struct {
	sessionID    string
	lastMsgCount int
	lastAccess   time.Time
}

// UserMsgResolver fingerprints conversations by hashing the first user
// message combined with the user ID. It uses the "superset" heuristic:
// if the message count grew since the last request with the same
// fingerprint, it's the same session.
type UserMsgResolver struct {
	mu         sync.Mutex
	cache      map[string]*sessionEntry // fingerprint → entry
	ttl        time.Duration
	maxEntries int              // safety cap; 0 means 1000
	nowFunc    func() time.Time // for testing
}

// NewUserMsgResolver creates a resolver with the given in-memory TTL.
// Entries not accessed within the TTL are evicted lazily on next lookup.
// The cache is capped at 1000 entries to prevent unbounded growth.
func NewUserMsgResolver(ttl time.Duration) *UserMsgResolver {
	if ttl <= 0 {
		ttl = 30 * time.Minute
	}
	return &UserMsgResolver{
		cache:      make(map[string]*sessionEntry),
		ttl:        ttl,
		maxEntries: 1000,
		nowFunc:    time.Now,
	}
}

func (u *UserMsgResolver) Resolve(info SessionInfo) string {
	fp, msgCount := u.fingerprint(info)
	if fp == "" {
		return ""
	}

	u.mu.Lock()
	defer u.mu.Unlock()

	now := u.nowFunc()

	if entry, ok := u.cache[fp]; ok {
		// Lazy eviction: if stale, remove and treat as new.
		if now.Sub(entry.lastAccess) > u.ttl {
			delete(u.cache, fp)
		} else if msgCount >= entry.lastMsgCount {
			// Superset check: messages grew or stayed same → same session.
			entry.lastMsgCount = msgCount
			entry.lastAccess = now
			return entry.sessionID
		}
		// Message count shrunk → likely compaction or new conversation.
		// Fall through to create new session.
	}

	// Evict stale entries + enforce max size before inserting.
	u.evictLocked(now)

	// New session.
	id := uuid.NewString()
	u.cache[fp] = &sessionEntry{
		sessionID:    id,
		lastMsgCount: msgCount,
		lastAccess:   now,
	}
	return id
}

// evictLocked removes expired entries and, if still over cap, evicts the
// oldest entries. Must be called with u.mu held.
func (u *UserMsgResolver) evictLocked(now time.Time) {
	// Phase 1: remove expired entries.
	for fp, entry := range u.cache {
		if now.Sub(entry.lastAccess) > u.ttl {
			delete(u.cache, fp)
		}
	}

	// Phase 2: if still over cap, evict oldest.
	max := u.maxEntries
	if max <= 0 {
		max = 1000
	}
	for len(u.cache) >= max {
		var oldestFP string
		var oldestTime time.Time
		for fp, entry := range u.cache {
			if oldestFP == "" || entry.lastAccess.Before(oldestTime) {
				oldestFP = fp
				oldestTime = entry.lastAccess
			}
		}
		delete(u.cache, oldestFP)
	}
}

// fingerprint extracts the first user message from the messages array
// and hashes it with the user ID to produce a stable conversation key.
// Handles both string content and multi-modal content (array of objects)
// per the OpenAI API spec.
func (u *UserMsgResolver) fingerprint(info SessionInfo) (string, int) {
	if len(info.Messages) == 0 {
		return "", 0
	}

	var msgs []struct {
		Role    string          `json:"role"`
		Content json.RawMessage `json:"content"`
	}
	if err := json.Unmarshal(info.Messages, &msgs); err != nil {
		return "", 0
	}

	// Find the first user message.
	var contentBytes []byte
	for _, m := range msgs {
		if m.Role == "user" && len(m.Content) > 0 {
			// Try to unmarshal as a plain string first (common case).
			var s string
			if err := json.Unmarshal(m.Content, &s); err == nil {
				contentBytes = []byte(s)
			} else {
				// Multi-modal content (array of objects): use raw JSON as fingerprint.
				contentBytes = m.Content
			}
			break
		}
	}
	if len(contentBytes) == 0 {
		return "", len(msgs)
	}

	h := sha256.New()
	h.Write([]byte(info.UserID))
	h.Write([]byte{0}) // separator
	h.Write(contentBytes)

	return hex.EncodeToString(h.Sum(nil)), len(msgs)
}

// CacheSize returns the number of entries in the in-memory cache.
// Intended for testing and metrics.
func (u *UserMsgResolver) CacheSize() int {
	u.mu.Lock()
	defer u.mu.Unlock()
	return len(u.cache)
}

package proxy

import (
	"sync"
	"time"
)

// saRateLimiter provides per-service-account in-process rate limiting.
//
// Unlike user rate limits (Firestore-backed, per-minute window docs),
// SA rate limits are in-process only — service accounts are server-to-server
// callers and a single Cloud Run instance sees all of a given SA's traffic.
//
// Design: simple fixed-window counter per SA per minute. Lightweight,
// no external dependencies, no Firestore cost on the hot path.
type saRateLimiter struct {
	mu      sync.Mutex
	windows map[string]*saWindow
	limit   int // requests per minute
	gcEvery int // run GC every N calls to Allow
	gcCount int
}

type saWindow struct {
	minute string // "2006-01-02T15:04" — the window key
	count  int
}

const defaultSARateLimit = 120 // requests per minute per SA

// newSARateLimiter creates a rate limiter with the given per-minute limit.
// Use limit <= 0 for the default (120 RPM).
func newSARateLimiter(limit int) *saRateLimiter {
	if limit <= 0 {
		limit = defaultSARateLimit
	}
	return &saRateLimiter{
		windows: make(map[string]*saWindow),
		limit:   limit,
		gcEvery: 100, // GC stale windows every 100 calls
	}
}

// Allow checks whether the given service account is within its rate limit.
// Returns (allowed, currentCount, limit).
func (r *saRateLimiter) Allow(sa string) (bool, int, int) {
	now := time.Now().UTC().Format("2006-01-02T15:04")

	r.mu.Lock()
	defer r.mu.Unlock()

	// Lazy GC: remove stale windows periodically.
	r.gcCount++
	if r.gcCount >= r.gcEvery {
		r.gcCount = 0
		r.gc(now)
	}

	w, ok := r.windows[sa]
	if !ok || w.minute != now {
		// New window — first request this minute.
		r.windows[sa] = &saWindow{minute: now, count: 1}
		return true, 1, r.limit
	}

	w.count++
	return w.count <= r.limit, w.count, r.limit
}

// gc removes windows from previous minutes.
func (r *saRateLimiter) gc(currentMinute string) {
	for sa, w := range r.windows {
		if w.minute != currentMinute {
			delete(r.windows, sa)
		}
	}
}

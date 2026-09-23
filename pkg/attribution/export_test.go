package attribution

import "time"

// resetRateLimiterForTesting resets the rate limiter state for tests.
func resetRateLimiterForTesting() {
	defaultLimiter.mu.Lock()
	defer defaultLimiter.mu.Unlock()
	defaultLimiter.lastWarn = make(map[string]time.Time)
	defaultLimiter.lastPrune = time.Time{}
}

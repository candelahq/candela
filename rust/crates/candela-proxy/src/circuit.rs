//! Circuit breaker for per-provider failure isolation.
//!
//! Ported from: `pkg/proxy/circuit_breaker.go`

use std::time::{Duration, Instant};

/// Per-provider circuit breaker to prevent cascading failures.
pub struct CircuitBreaker {
    failure_count: u32,
    threshold: u32,
    reset_timeout: Duration,
    last_failure: Option<Instant>,
    state: State,
}

#[derive(Debug, Clone, Copy, PartialEq, Eq)]
pub enum State {
    Closed,
    Open,
    HalfOpen,
}

impl CircuitBreaker {
    pub fn new(threshold: u32, reset_timeout: Duration) -> Self {
        Self {
            failure_count: 0,
            threshold,
            reset_timeout,
            last_failure: None,
            state: State::Closed,
        }
    }

    /// Record a successful call — resets failure count.
    pub fn record_success(&mut self) {
        self.failure_count = 0;
        self.state = State::Closed;
    }

    /// Record a failed call — may trip the breaker.
    pub fn record_failure(&mut self) {
        self.failure_count = self.failure_count.saturating_add(1);
        self.last_failure = Some(Instant::now());
        if self.failure_count >= self.threshold {
            self.state = State::Open;
        }
    }

    /// Check if requests are currently allowed.
    pub fn is_allowed(&mut self) -> bool {
        match self.state {
            State::Closed => true,
            State::Open => {
                if let Some(last) = self.last_failure {
                    if last.elapsed() >= self.reset_timeout {
                        self.state = State::HalfOpen;
                        true
                    } else {
                        false
                    }
                } else {
                    true
                }
            }
            State::HalfOpen => true,
        }
    }

    pub fn state(&self) -> State {
        self.state
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn trips_after_threshold() {
        let mut cb = CircuitBreaker::new(3, Duration::from_secs(30));
        assert!(cb.is_allowed());

        cb.record_failure();
        cb.record_failure();
        assert!(cb.is_allowed());

        cb.record_failure();
        assert!(!cb.is_allowed());
        assert_eq!(cb.state(), State::Open);
    }

    #[test]
    fn resets_on_success() {
        let mut cb = CircuitBreaker::new(2, Duration::from_secs(30));
        cb.record_failure();
        cb.record_failure();
        assert!(!cb.is_allowed());

        // Simulate timeout elapsed by directly setting state
        cb.state = State::HalfOpen;
        assert!(cb.is_allowed());

        cb.record_success();
        assert_eq!(cb.state(), State::Closed);
    }

    #[test]
    fn failure_count_saturates() {
        // CRITICAL-2: Must not overflow/wrap.
        let mut cb = CircuitBreaker::new(u32::MAX, Duration::from_secs(30));
        cb.failure_count = u32::MAX - 1;
        cb.record_failure(); // Should saturate to u32::MAX, not panic.
        assert_eq!(cb.failure_count, u32::MAX);
        cb.record_failure(); // Should stay at u32::MAX.
        assert_eq!(cb.failure_count, u32::MAX);
    }

    #[test]
    fn half_open_trips_on_failure() {
        let mut cb = CircuitBreaker::new(2, Duration::from_secs(30));
        cb.record_failure();
        cb.record_failure();
        assert_eq!(cb.state(), State::Open);

        // Simulate entering half-open.
        cb.state = State::HalfOpen;
        assert!(cb.is_allowed());

        // Another failure should re-trip to open.
        cb.record_failure();
        assert_eq!(cb.state(), State::Open);
    }

    // U-14: Default config creates a working circuit breaker.
    #[test]
    fn default_config_creates_working_breaker() {
        let mut cb = CircuitBreaker::new(5, Duration::from_secs(30));
        assert_eq!(cb.state(), State::Closed);
        assert!(cb.is_allowed());

        // 4 failures — should still be closed.
        for _ in 0..4 {
            cb.record_failure();
        }
        assert!(cb.is_allowed());

        // 5th failure trips it.
        cb.record_failure();
        assert_eq!(cb.state(), State::Open);
        assert!(!cb.is_allowed());

        // Success resets.
        cb.state = State::HalfOpen;
        cb.record_success();
        assert_eq!(cb.state(), State::Closed);
        assert!(cb.is_allowed());
    }

    // ── New comprehensive tests ──

    /// Threshold of 1 should trip immediately on first failure.
    #[test]
    fn threshold_one_trips_immediately() {
        let mut cb = CircuitBreaker::new(1, Duration::from_secs(30));
        assert!(cb.is_allowed());
        cb.record_failure();
        assert_eq!(cb.state(), State::Open);
        assert!(!cb.is_allowed());
    }

    /// Success in half-open state should reset failure count to 0.
    #[test]
    fn success_in_half_open_resets_failure_count() {
        let mut cb = CircuitBreaker::new(3, Duration::from_secs(30));
        // Trip the breaker.
        for _ in 0..3 {
            cb.record_failure();
        }
        assert_eq!(cb.failure_count, 3);

        // Simulate entering half-open and succeeding.
        cb.state = State::HalfOpen;
        cb.record_success();
        assert_eq!(cb.failure_count, 0);
        assert_eq!(cb.state(), State::Closed);

        // Should now tolerate 2 more failures before tripping again.
        cb.record_failure();
        cb.record_failure();
        assert!(cb.is_allowed());
    }

    /// Rapid alternation between success and failure should not trip if
    /// failures never accumulate to threshold.
    #[test]
    fn interleaved_success_failure_stays_closed() {
        let mut cb = CircuitBreaker::new(3, Duration::from_secs(30));
        for _ in 0..100 {
            cb.record_failure();
            cb.record_success(); // resets count
        }
        assert_eq!(cb.state(), State::Closed);
        assert!(cb.is_allowed());
    }

    /// New breaker with very short timeout should transition quickly.
    #[test]
    fn very_short_timeout_transitions() {
        let mut cb = CircuitBreaker::new(1, Duration::from_millis(1));
        cb.record_failure();
        assert_eq!(cb.state(), State::Open);

        // Sleep past the timeout.
        std::thread::sleep(Duration::from_millis(5));

        // Should transition to HalfOpen on next check.
        assert!(cb.is_allowed());
        assert_eq!(cb.state(), State::HalfOpen);
    }

    /// Multiple successes after reset should keep state closed.
    #[test]
    fn multiple_successes_keep_closed() {
        let mut cb = CircuitBreaker::new(3, Duration::from_secs(30));
        for _ in 0..100 {
            cb.record_success();
        }
        assert_eq!(cb.state(), State::Closed);
        assert_eq!(cb.failure_count, 0);
    }

    /// Breaker should stay open until timeout elapses, even with more failures.
    #[test]
    fn stays_open_with_continued_failures() {
        let mut cb = CircuitBreaker::new(2, Duration::from_secs(300));
        cb.record_failure();
        cb.record_failure();
        assert_eq!(cb.state(), State::Open);

        // More failures shouldn't change state (already open).
        for _ in 0..10 {
            cb.record_failure();
        }
        assert_eq!(cb.state(), State::Open);
        assert!(!cb.is_allowed());
    }
}

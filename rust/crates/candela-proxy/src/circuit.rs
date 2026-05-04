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
        self.failure_count += 1;
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
}

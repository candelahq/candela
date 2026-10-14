//! Per-model token cost calculator.
//!
//! Ported from: `pkg/costcalc/calculator.go`

use std::collections::HashMap;
use std::time::{SystemTime, UNIX_EPOCH};

/// Promotional expiry for Gemini 4 Argon: 2026-11-30T23:59:59Z (Unix timestamp in seconds).
const GEMINI_4_ARGON_PROMO_VALID_UNTIL_UNIX: u64 = 1_796_083_199;

/// Per-model pricing configuration (rates in USD per 1 million tokens).
#[derive(Clone, Copy, Debug)]
struct Pricing {
    input_per_million: f64,
    output_per_million: f64,
    /// Optional context threshold above which high-tier rates apply.
    tier_threshold_tokens: Option<i64>,
    input_per_million_high: Option<f64>,
    output_per_million_high: Option<f64>,
    /// Optional promotional expiry (Unix timestamp in seconds, inclusive) and fallback rates.
    valid_until_unix: Option<u64>,
    fallback_input_per_million: Option<f64>,
    fallback_output_per_million: Option<f64>,
}

impl Pricing {
    const fn flat(input_per_million: f64, output_per_million: f64) -> Self {
        Self {
            input_per_million,
            output_per_million,
            tier_threshold_tokens: None,
            input_per_million_high: None,
            output_per_million_high: None,
            valid_until_unix: None,
            fallback_input_per_million: None,
            fallback_output_per_million: None,
        }
    }

    const fn with_promo_fallback(
        mut self,
        valid_until_unix: u64,
        fallback_input_per_million: f64,
        fallback_output_per_million: f64,
    ) -> Self {
        self.valid_until_unix = Some(valid_until_unix);
        self.fallback_input_per_million = Some(fallback_input_per_million);
        self.fallback_output_per_million = Some(fallback_output_per_million);
        self
    }

    const fn with_tiers(
        mut self,
        tier_threshold_tokens: i64,
        input_per_million_high: f64,
        output_per_million_high: f64,
    ) -> Self {
        self.tier_threshold_tokens = Some(tier_threshold_tokens);
        self.input_per_million_high = Some(input_per_million_high);
        self.output_per_million_high = Some(output_per_million_high);
        self
    }

    fn effective_base_rates(&self, now_unix: u64) -> (f64, f64) {
        if let Some(valid_until) = self.valid_until_unix
            && now_unix > valid_until
        {
            return (
                self.fallback_input_per_million
                    .unwrap_or(self.input_per_million),
                self.fallback_output_per_million
                    .unwrap_or(self.output_per_million),
            );
        }
        (self.input_per_million, self.output_per_million)
    }
}

fn current_unix_timestamp() -> u64 {
    SystemTime::now()
        .duration_since(UNIX_EPOCH)
        .map(|d| d.as_secs())
        .unwrap_or(0)
}

/// Token cost calculator for LLM API calls.
pub struct CostCalculator {
    prices: HashMap<String, Pricing>,
}

impl CostCalculator {
    /// Create a new calculator with default model pricing.
    ///
    /// Prices are list prices in USD per 1 million tokens (as of April 2026).
    pub fn new() -> Self {
        let mut prices = HashMap::new();

        // ── Google Gemini ─────────────────────────────────────────
        // Gemini 4 (promotional $2/$10 through 2026-11-30T23:59:59Z, then $4/$20)
        prices.insert(
            "gemini-4-argon".into(),
            Pricing::flat(2.00, 10.00).with_promo_fallback(
                GEMINI_4_ARGON_PROMO_VALID_UNTIL_UNIX,
                4.00,
                20.00,
            ),
        );
        // Gemini 3.1 (latest)
        prices.insert("gemini-3.1-pro".into(), Pricing::flat(2.00, 12.00));
        // Gemini 2.5
        prices.insert("gemini-2.5-pro".into(), Pricing::flat(1.25, 10.00));
        prices.insert("gemini-2.5-flash".into(), Pricing::flat(0.30, 2.50));
        prices.insert("gemini-2.5-flash-lite".into(), Pricing::flat(0.10, 0.40));
        // Gemini 2.0
        prices.insert("gemini-2.0-flash".into(), Pricing::flat(0.10, 0.40));
        prices.insert("gemini-2.0-pro".into(), Pricing::flat(1.25, 10.00));
        // Gemini 1.5 (legacy)
        prices.insert("gemini-1.5-flash".into(), Pricing::flat(0.075, 0.30));
        prices.insert("gemini-1.5-pro".into(), Pricing::flat(1.25, 5.00));

        // ── OpenAI ───────────────────────────────────────────────
        // GPT-5.4 (latest, March 2026)
        prices.insert("gpt-5.4-pro".into(), Pricing::flat(30.00, 180.00));
        prices.insert("gpt-5.4".into(), Pricing::flat(2.50, 15.00));
        prices.insert("gpt-5.4-mini".into(), Pricing::flat(0.75, 4.50));
        prices.insert("gpt-5.4-nano".into(), Pricing::flat(0.20, 1.25));
        // GPT-4.1 family (1M context)
        prices.insert("gpt-4.1".into(), Pricing::flat(2.00, 8.00));
        prices.insert("gpt-4.1-mini".into(), Pricing::flat(0.40, 1.60));
        prices.insert("gpt-4.1-nano".into(), Pricing::flat(0.10, 0.40));
        // GPT-4o (legacy)
        prices.insert("gpt-4o".into(), Pricing::flat(2.50, 10.00));
        prices.insert("gpt-4o-mini".into(), Pricing::flat(0.15, 0.60));
        // GPT-4 (legacy)
        prices.insert("gpt-4-turbo".into(), Pricing::flat(10.00, 30.00));
        prices.insert("gpt-3.5-turbo".into(), Pricing::flat(0.50, 1.50));
        // Reasoning models (o3 price cut: was $10/$40, now $2/$8 as of early 2026)
        prices.insert("o3".into(), Pricing::flat(2.00, 8.00));
        prices.insert("o4-mini".into(), Pricing::flat(1.10, 4.40));
        prices.insert("o3-mini".into(), Pricing::flat(1.10, 4.40));
        prices.insert("o1".into(), Pricing::flat(15.00, 60.00));
        prices.insert("o1-mini".into(), Pricing::flat(3.00, 12.00));

        // ── Anthropic (via Vertex AI or direct) ──────────────────
        // Claude 5.5 / 5
        prices.insert("claude-opus-5.5".into(), Pricing::flat(4.00, 20.00));
        prices.insert("claude-sonnet-5.5".into(), Pricing::flat(2.00, 10.00));
        prices.insert(
            "claude-haiku-5.5".into(),
            Pricing::flat(0.10, 0.50).with_tiers(100_000, 0.50, 2.50),
        );
        prices.insert("claude-opus-5".into(), Pricing::flat(5.00, 25.00));
        prices.insert("claude-sonnet-5".into(), Pricing::flat(2.00, 10.00));
        // Claude 4.6/4.7/4.8
        prices.insert("claude-opus-4.8".into(), Pricing::flat(5.00, 25.00));
        prices.insert("claude-opus-4.7".into(), Pricing::flat(5.00, 25.00));
        prices.insert("claude-opus-4.6".into(), Pricing::flat(5.00, 25.00));
        prices.insert("claude-sonnet-4.6".into(), Pricing::flat(3.00, 15.00));
        prices.insert("claude-haiku-4.5".into(), Pricing::flat(1.00, 5.00));
        // Claude 4 (Vertex AI model IDs)
        prices.insert(
            "claude-sonnet-4-20250514".into(),
            Pricing::flat(3.00, 15.00),
        );
        // Claude 3.5 (legacy)
        prices.insert(
            "claude-3-5-sonnet-20241022".into(),
            Pricing::flat(3.00, 15.00),
        );
        prices.insert(
            "claude-haiku-3-5-20241022".into(),
            Pricing::flat(0.80, 4.00),
        );
        prices.insert("claude-3-opus-20240229".into(), Pricing::flat(15.00, 75.00));

        Self { prices }
    }

    /// Calculate cost in USD for a given model and token counts.
    ///
    /// Returns 0.0 for unknown models.
    pub fn calculate(
        &self,
        provider: &str,
        model: &str,
        input_tokens: i64,
        output_tokens: i64,
    ) -> f64 {
        self.calculate_at(
            provider,
            model,
            input_tokens,
            output_tokens,
            current_unix_timestamp(),
        )
    }

    fn calculate_at(
        &self,
        _provider: &str,
        model: &str,
        input_tokens: i64,
        output_tokens: i64,
        now_unix: u64,
    ) -> f64 {
        let Some(pricing) = self.prices.get(model) else {
            return 0.0;
        };

        // CRITICAL: Clamp negative tokens to prevent negative costs.
        let input_tokens = input_tokens.max(0);
        let output_tokens = output_tokens.max(0);

        let (input_rate, output_rate) = pricing.effective_base_rates(now_unix);

        let input_cost = match (
            pricing.tier_threshold_tokens,
            pricing.input_per_million_high,
        ) {
            (Some(threshold), Some(high_input_rate))
                if threshold > 0 && input_tokens > threshold =>
            {
                let base_tokens = threshold;
                let overflow_tokens = input_tokens - threshold;
                (base_tokens as f64 / 1_000_000.0) * input_rate
                    + (overflow_tokens as f64 / 1_000_000.0) * high_input_rate
            }
            _ => (input_tokens as f64 / 1_000_000.0) * input_rate,
        };

        let effective_output_rate = match (
            pricing.tier_threshold_tokens,
            pricing.output_per_million_high,
        ) {
            (Some(threshold), Some(high_output_rate))
                if threshold > 0 && input_tokens > threshold =>
            {
                high_output_rate
            }
            _ => output_rate,
        };
        let output_cost = (output_tokens as f64 / 1_000_000.0) * effective_output_rate;
        input_cost + output_cost
    }

    /// Returns true if the model has pricing configured.
    /// Used by the pricing gate to block calls to unknown models (which would
    /// be billed at $0, letting users make free API calls).
    pub fn has_pricing(&self, model: &str) -> bool {
        self.prices.contains_key(model)
    }
}

impl Default for CostCalculator {
    fn default() -> Self {
        Self::new()
    }
}

#[cfg(test)]
mod tests {
    use super::*;

    #[test]
    fn calculates_openai_cost() {
        let calc = CostCalculator::new();
        let cost = calc.calculate("openai", "gpt-4o", 1_000_000, 500_000);
        // 1M input @ $2.50/M + 500K output @ $10.00/M = $2.50 + $5.00 = $7.50
        assert!((cost - 7.50).abs() < 0.001);
    }

    #[test]
    fn calculates_anthropic_cost() {
        let calc = CostCalculator::new();
        let cost = calc.calculate("anthropic", "claude-sonnet-4-20250514", 100_000, 50_000);
        // 100K @ $3.00/M + 50K @ $15.00/M = $0.30 + $0.75 = $1.05
        assert!((cost - 1.05).abs() < 0.001);
    }

    #[test]
    fn claude_opus4_uses_correct_pricing() {
        let calc = CostCalculator::new();
        // claude-opus-4.8 should be $5/$25, NOT the legacy $15/$75
        let cost = calc.calculate("anthropic", "claude-opus-4.8", 1_000_000, 1_000_000);
        // 1M input @ $5.00/M + 1M output @ $25.00/M = $5.00 + $25.00 = $30.00
        assert!(
            (cost - 30.00).abs() < 0.001,
            "got {cost}, want 30.00 — was this set to legacy Claude 3 Opus pricing?"
        );
    }

    #[test]
    fn gemini_4_argon_promo_and_fallback_pricing() {
        let expected_expiry = chrono::DateTime::parse_from_rfc3339("2026-11-30T23:59:59Z")
            .unwrap()
            .timestamp() as u64;
        assert_eq!(GEMINI_4_ARGON_PROMO_VALID_UNTIL_UNIX, expected_expiry);

        let calc = CostCalculator::new();
        // During promo window (on or before 2026-11-30T23:59:59Z): $2/$10 = $12.00 for 1M/1M
        let promo_cost = calc.calculate_at(
            "google",
            "gemini-4-argon",
            1_000_000,
            1_000_000,
            GEMINI_4_ARGON_PROMO_VALID_UNTIL_UNIX,
        );
        assert!((promo_cost - 12.00).abs() < 0.001);

        // After promo expiry (2026-12-01T00:00:00Z): $4/$20 = $24.00 for 1M/1M
        let post_promo_cost = calc.calculate_at(
            "google",
            "gemini-4-argon",
            1_000_000,
            1_000_000,
            GEMINI_4_ARGON_PROMO_VALID_UNTIL_UNIX + 1,
        );
        assert!((post_promo_cost - 24.00).abs() < 0.001);
    }

    #[test]
    fn claude_haiku_55_tiered_pricing() {
        let calc = CostCalculator::new();
        // At 100K threshold: 100K input @ $0.10/M ($0.01) + 10K output @ $0.50/M ($0.005) = $0.015
        let low_tier = calc.calculate("anthropic", "claude-haiku-5.5", 100_000, 10_000);
        assert!(
            (low_tier - 0.015).abs() < 1e-6,
            "got {low_tier}, want 0.015"
        );

        // Above 100K threshold (200K input, 10K output):
        // Base 100K input @ $0.10/M ($0.01) + overflow 100K input @ $0.50/M ($0.05)
        // + 10K output @ high rate $2.50/M ($0.025) = $0.085
        let high_tier = calc.calculate("anthropic", "claude-haiku-5.5", 200_000, 10_000);
        assert!(
            (high_tier - 0.085).abs() < 1e-6,
            "got {high_tier}, want 0.085"
        );
    }

    #[test]
    fn claude3_opus_legacy_pricing() {
        let calc = CostCalculator::new();
        // claude-3-opus-20240229 (legacy) should be $15/$75
        let cost = calc.calculate("anthropic", "claude-3-opus-20240229", 1_000_000, 1_000_000);
        // 1M input @ $15.00/M + 1M output @ $75.00/M = $15.00 + $75.00 = $90.00
        assert!((cost - 90.00).abs() < 0.001);
    }

    #[test]
    fn unknown_model_returns_zero() {
        let calc = CostCalculator::new();
        assert_eq!(calc.calculate("openai", "unknown-model", 1000, 1000), 0.0);
    }

    #[test]
    fn gemini_cost_calculation() {
        let calc = CostCalculator::new();
        let cost = calc.calculate("google", "gemini-2.0-flash", 1_000_000, 1_000_000);
        // 1M input @ $0.10/M + 1M output @ $0.40/M = $0.10 + $0.40 = $0.50
        assert!((cost - 0.50).abs() < 0.001);
    }

    #[test]
    fn gemini_25_flash_pricing() {
        let calc = CostCalculator::new();
        let cost = calc.calculate("google", "gemini-2.5-flash", 1_000_000, 1_000_000);
        // 1M input @ $0.30/M + 1M output @ $2.50/M = $0.30 + $2.50 = $2.80
        assert!((cost - 2.80).abs() < 0.001);
    }

    #[test]
    fn gpt54_pricing() {
        let calc = CostCalculator::new();
        let cost = calc.calculate("openai", "gpt-5.4", 1_000_000, 1_000_000);
        // 1M input @ $2.50/M + 1M output @ $15.00/M = $2.50 + $15.00 = $17.50
        assert!((cost - 17.50).abs() < 0.001);
    }

    #[test]
    fn negative_tokens_clamped() {
        // CRITICAL-3: Negative tokens must not produce negative costs.
        let calc = CostCalculator::new();
        let cost = calc.calculate("openai", "gpt-4o", -1000, -500);
        assert!(cost >= 0.0, "cost must never be negative, got {cost}");
        assert_eq!(cost, 0.0); // clamped to 0 tokens = $0
    }

    #[test]
    fn zero_tokens_returns_zero() {
        let calc = CostCalculator::new();
        let cost = calc.calculate("openai", "gpt-4o", 0, 0);
        assert_eq!(cost, 0.0);
    }

    #[test]
    fn has_pricing_known_model() {
        let calc = CostCalculator::new();
        assert!(calc.has_pricing("gpt-4o"));
        assert!(calc.has_pricing("claude-opus-5.5"));
        assert!(calc.has_pricing("gemini-4-argon"));
        assert!(calc.has_pricing("gemini-2.5-flash"));
    }

    #[test]
    fn has_pricing_unknown_model() {
        let calc = CostCalculator::new();
        assert!(!calc.has_pricing("unknown-model-xyz"));
        assert!(!calc.has_pricing(""));
    }

    // ── New comprehensive tests ──

    /// All Gemini model variants should have pricing.
    #[test]
    fn all_gemini_models_have_pricing() {
        let calc = CostCalculator::new();
        for model in &[
            "gemini-3.1-pro",
            "gemini-2.5-pro",
            "gemini-2.5-flash",
            "gemini-2.5-flash-lite",
            "gemini-2.0-flash",
            "gemini-2.0-pro",
            "gemini-1.5-flash",
            "gemini-1.5-pro",
        ] {
            assert!(calc.has_pricing(model), "missing pricing for {model}");
        }
    }

    /// All OpenAI reasoning models should have pricing.
    #[test]
    fn all_reasoning_models_have_pricing() {
        let calc = CostCalculator::new();
        for model in &["o3", "o3-mini", "o1", "o1-mini"] {
            assert!(calc.has_pricing(model), "missing pricing for {model}");
        }
    }

    /// Claude pricing hierarchy: Opus > Sonnet > Haiku.
    #[test]
    fn claude_pricing_hierarchy() {
        let calc = CostCalculator::new();
        let opus = calc.calculate("anthropic", "claude-opus-4.6", 1_000_000, 1_000_000);
        let sonnet = calc.calculate("anthropic", "claude-sonnet-4.6", 1_000_000, 1_000_000);
        let haiku = calc.calculate("anthropic", "claude-haiku-4.5", 1_000_000, 1_000_000);
        assert!(
            opus > sonnet,
            "Opus ({opus}) should be more expensive than Sonnet ({sonnet})"
        );
        assert!(
            sonnet > haiku,
            "Sonnet ({sonnet}) should be more expensive than Haiku ({haiku})"
        );
    }

    /// GPT pricing hierarchy: Pro > Standard > Mini > Nano.
    #[test]
    fn gpt54_pricing_hierarchy() {
        let calc = CostCalculator::new();
        let pro = calc.calculate("openai", "gpt-5.4-pro", 1_000_000, 1_000_000);
        let standard = calc.calculate("openai", "gpt-5.4", 1_000_000, 1_000_000);
        let mini = calc.calculate("openai", "gpt-5.4-mini", 1_000_000, 1_000_000);
        let nano = calc.calculate("openai", "gpt-5.4-nano", 1_000_000, 1_000_000);
        assert!(pro > standard, "Pro ({pro}) > Standard ({standard})");
        assert!(standard > mini, "Standard ({standard}) > Mini ({mini})");
        assert!(mini > nano, "Mini ({mini}) > Nano ({nano})");
    }

    /// Very large token count should not panic or produce NaN.
    #[test]
    fn large_token_count_no_panic() {
        let calc = CostCalculator::new();
        let cost = calc.calculate("openai", "gpt-4o", i64::MAX, i64::MAX);
        assert!(cost.is_finite(), "cost must be finite, got {cost}");
        assert!(cost > 0.0);
    }

    /// Single-token precision: cost should be calculable for 1 token.
    #[test]
    fn single_token_cost_precision() {
        let calc = CostCalculator::new();
        let cost = calc.calculate("openai", "gpt-4o", 1, 0);
        // 1 token @ $2.50/M = $0.0000025
        assert!(cost > 0.0, "single token should produce non-zero cost");
        assert!((cost - 2.5e-6).abs() < 1e-9);
    }

    /// Provider field is ignored — only model matters for pricing.
    #[test]
    fn provider_field_ignored() {
        let calc = CostCalculator::new();
        let cost_a = calc.calculate("openai", "gpt-4o", 1000, 1000);
        let cost_b = calc.calculate("anything", "gpt-4o", 1000, 1000);
        assert_eq!(cost_a, cost_b, "provider field should not affect cost");
    }

    /// Output tokens are typically more expensive than input tokens.
    #[test]
    fn output_more_expensive_than_input() {
        let calc = CostCalculator::new();
        let input_only = calc.calculate("openai", "gpt-4o", 1_000_000, 0);
        let output_only = calc.calculate("openai", "gpt-4o", 0, 1_000_000);
        assert!(
            output_only > input_only,
            "output ({output_only}) should cost more than input ({input_only}) for gpt-4o"
        );
    }
}

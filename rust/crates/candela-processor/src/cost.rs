//! Per-model token cost calculator.
//!
//! Ported from: `pkg/costcalc/calculator.go`

use std::collections::HashMap;

/// Per-model pricing: (input_cost_per_million, output_cost_per_million).
type Pricing = (f64, f64);

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
        // Gemini 3.1 (latest)
        prices.insert("gemini-3.1-pro".into(), (2.00, 12.00));
        // Gemini 2.5
        prices.insert("gemini-2.5-pro".into(), (1.25, 10.00));
        prices.insert("gemini-2.5-flash".into(), (0.30, 2.50));
        prices.insert("gemini-2.5-flash-lite".into(), (0.10, 0.40));
        // Gemini 2.0
        prices.insert("gemini-2.0-flash".into(), (0.10, 0.40));
        prices.insert("gemini-2.0-pro".into(), (1.25, 10.00));
        // Gemini 1.5 (legacy)
        prices.insert("gemini-1.5-flash".into(), (0.075, 0.30));
        prices.insert("gemini-1.5-pro".into(), (1.25, 5.00));

        // ── OpenAI ───────────────────────────────────────────────
        // GPT-5.4 (latest, March 2026)
        prices.insert("gpt-5.4-pro".into(), (30.00, 180.00));
        prices.insert("gpt-5.4".into(), (2.50, 15.00));
        prices.insert("gpt-5.4-mini".into(), (0.75, 4.50));
        prices.insert("gpt-5.4-nano".into(), (0.20, 1.25));
        // GPT-4o
        prices.insert("gpt-4o".into(), (2.50, 10.00));
        prices.insert("gpt-4o-mini".into(), (0.15, 0.60));
        // GPT-4 (legacy)
        prices.insert("gpt-4-turbo".into(), (10.00, 30.00));
        prices.insert("gpt-3.5-turbo".into(), (0.50, 1.50));
        // Reasoning models
        prices.insert("o3".into(), (10.00, 40.00));
        prices.insert("o3-mini".into(), (1.10, 4.40));
        prices.insert("o1".into(), (15.00, 60.00));
        prices.insert("o1-mini".into(), (3.00, 12.00));

        // ── Anthropic (via Vertex AI or direct) ──────────────────
        // Claude 4.6/4.7 (latest)
        prices.insert("claude-opus-4.7".into(), (5.00, 25.00));
        prices.insert("claude-opus-4.6".into(), (5.00, 25.00));
        prices.insert("claude-sonnet-4.6".into(), (3.00, 15.00));
        prices.insert("claude-haiku-4.5".into(), (1.00, 5.00));
        // Claude 4 (Vertex AI model IDs)
        prices.insert("claude-sonnet-4-20250514".into(), (3.00, 15.00));
        prices.insert("claude-opus-4-20250514".into(), (5.00, 25.00));
        // Claude 3.5 (legacy)
        prices.insert("claude-3-5-sonnet-20241022".into(), (3.00, 15.00));
        prices.insert("claude-haiku-3-5-20241022".into(), (0.80, 4.00));
        prices.insert("claude-3-opus-20240229".into(), (15.00, 75.00));

        Self { prices }
    }

    /// Calculate cost in USD for a given model and token counts.
    ///
    /// Returns 0.0 for unknown models.
    pub fn calculate(
        &self,
        _provider: &str,
        model: &str,
        input_tokens: i64,
        output_tokens: i64,
    ) -> f64 {
        let Some(&(input_rate, output_rate)) = self.prices.get(model) else {
            return 0.0;
        };

        // CRITICAL: Clamp negative tokens to prevent negative costs.
        let input_tokens = input_tokens.max(0);
        let output_tokens = output_tokens.max(0);

        let input_cost = (input_tokens as f64 / 1_000_000.0) * input_rate;
        let output_cost = (output_tokens as f64 / 1_000_000.0) * output_rate;
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
        // claude-opus-4-20250514 should be $5/$25, NOT the legacy $15/$75
        let cost = calc.calculate("anthropic", "claude-opus-4-20250514", 1_000_000, 1_000_000);
        // 1M input @ $5.00/M + 1M output @ $25.00/M = $5.00 + $25.00 = $30.00
        assert!(
            (cost - 30.00).abs() < 0.001,
            "got {cost}, want 30.00 — was this set to legacy Claude 3 Opus pricing?"
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
        assert!(calc.has_pricing("claude-opus-4-20250514"));
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

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
    pub fn new() -> Self {
        let mut prices = HashMap::new();

        // OpenAI
        prices.insert("gpt-4o".into(), (2.50, 10.00));
        prices.insert("gpt-4o-mini".into(), (0.15, 0.60));
        prices.insert("gpt-4-turbo".into(), (10.00, 30.00));
        prices.insert("gpt-4".into(), (30.00, 60.00));
        prices.insert("gpt-3.5-turbo".into(), (0.50, 1.50));
        prices.insert("o1".into(), (15.00, 60.00));
        prices.insert("o1-mini".into(), (3.00, 12.00));
        prices.insert("o3-mini".into(), (1.10, 4.40));

        // Anthropic
        prices.insert("claude-sonnet-4-20250514".into(), (3.00, 15.00));
        prices.insert("claude-3-5-sonnet-20241022".into(), (3.00, 15.00));
        prices.insert("claude-3-5-haiku-20241022".into(), (0.80, 4.00));
        prices.insert("claude-3-opus-20240229".into(), (15.00, 75.00));
        prices.insert("claude-3-haiku-20240307".into(), (0.25, 1.25));

        // Google Gemini
        prices.insert("gemini-2.0-flash".into(), (0.10, 0.40));
        prices.insert("gemini-2.0-flash-lite".into(), (0.075, 0.30));
        prices.insert("gemini-1.5-pro".into(), (1.25, 5.00));
        prices.insert("gemini-1.5-flash".into(), (0.075, 0.30));

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
}

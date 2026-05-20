/**
 * Static per-model pricing registry.
 *
 * SOURCE OF TRUTH: pkg/costcalc/calculator.go → loadDefaults()
 * Keep this file in sync whenever model pricing is added or changed.
 *
 * Prices are list prices in USD per 1 million tokens.
 */

export interface ModelPricing {
  inputPerMillion: number;
  outputPerMillion: number;
}

export interface CacheEfficiency {
  /** Hit rate 0.0–1.0 */
  rate: number;
  label: string;
  /** CSS-friendly color */
  color: string;
  /** CSS class suffix: "excellent" | "good" | "low" */
  tier: "excellent" | "good" | "low";
}

// ── Registry ──────────────────────────────────────────────────────
// Keys are lowercase model names (provider-agnostic, matching the
// fallback behavior in calculator.go).

const PRICING_MAP: Record<string, ModelPricing> = {
  // Google Gemini
  "gemini-3.5-flash":        { inputPerMillion: 1.50,  outputPerMillion: 9.00 },
  "gemini-3.1-pro":          { inputPerMillion: 2.00,  outputPerMillion: 12.00 },
  "gemini-3.1-flash":        { inputPerMillion: 0.50,  outputPerMillion: 3.00 },
  "gemini-3.1-flash-lite":   { inputPerMillion: 0.25,  outputPerMillion: 1.50 },
  "gemini-3.0-pro":          { inputPerMillion: 2.00,  outputPerMillion: 12.00 },
  "gemini-3.0-flash":        { inputPerMillion: 0.50,  outputPerMillion: 3.00 },
  "gemini-2.5-pro":          { inputPerMillion: 1.25,  outputPerMillion: 10.00 },
  "gemini-2.5-flash":        { inputPerMillion: 0.30,  outputPerMillion: 2.50 },
  "gemini-2.5-flash-lite":   { inputPerMillion: 0.10,  outputPerMillion: 0.40 },
  "gemini-2.0-flash":        { inputPerMillion: 0.10,  outputPerMillion: 0.40 },
  "gemini-2.0-pro":          { inputPerMillion: 1.25,  outputPerMillion: 10.00 },
  "gemini-1.5-flash":        { inputPerMillion: 0.075, outputPerMillion: 0.30 },
  "gemini-1.5-pro":          { inputPerMillion: 1.25,  outputPerMillion: 5.00 },

  // OpenAI
  "gpt-5.4-pro":     { inputPerMillion: 30.00, outputPerMillion: 180.00 },
  "gpt-5.4":         { inputPerMillion: 2.50,  outputPerMillion: 15.00 },
  "gpt-5.4-mini":    { inputPerMillion: 0.75,  outputPerMillion: 4.50 },
  "gpt-5.4-nano":    { inputPerMillion: 0.20,  outputPerMillion: 1.25 },
  "gpt-4o":          { inputPerMillion: 2.50,  outputPerMillion: 10.00 },
  "gpt-4o-mini":     { inputPerMillion: 0.15,  outputPerMillion: 0.60 },
  "gpt-4-turbo":     { inputPerMillion: 10.00, outputPerMillion: 30.00 },
  "gpt-3.5-turbo":   { inputPerMillion: 0.50,  outputPerMillion: 1.50 },
  "o3":              { inputPerMillion: 10.00, outputPerMillion: 40.00 },
  "o3-mini":         { inputPerMillion: 1.10,  outputPerMillion: 4.40 },
  "o1":              { inputPerMillion: 15.00, outputPerMillion: 60.00 },
  "o1-mini":         { inputPerMillion: 3.00,  outputPerMillion: 12.00 },

  // Anthropic
  "claude-opus-4.7":              { inputPerMillion: 5.00,  outputPerMillion: 25.00 },
  "claude-opus-4.6":              { inputPerMillion: 5.00,  outputPerMillion: 25.00 },
  "claude-sonnet-4.6":            { inputPerMillion: 3.00,  outputPerMillion: 15.00 },
  "claude-haiku-4.5":             { inputPerMillion: 1.00,  outputPerMillion: 5.00 },
  "claude-sonnet-4":              { inputPerMillion: 3.00,  outputPerMillion: 15.00 },
  "claude-opus-4":                { inputPerMillion: 5.00,  outputPerMillion: 25.00 },
  "claude-sonnet-4-20250514":     { inputPerMillion: 3.00,  outputPerMillion: 15.00 },
  "claude-opus-4-20250514":       { inputPerMillion: 5.00,  outputPerMillion: 25.00 },
  "claude-3-5-sonnet-20241022":   { inputPerMillion: 3.00,  outputPerMillion: 15.00 },
  "claude-haiku-3-5-20241022":    { inputPerMillion: 0.80,  outputPerMillion: 4.00 },
  "claude-3-opus-20240229":       { inputPerMillion: 15.00, outputPerMillion: 75.00 },
};

/**
 * Look up static list pricing for a model.
 * Returns null for models without a known price (e.g. local/custom).
 */
export function getModelPricing(model: string): ModelPricing | null {
  return PRICING_MAP[model.toLowerCase()] ?? null;
}

/**
 * Compute cache efficiency from per-model token counts.
 * Returns null when there are no cache read tokens.
 */
export function getCacheEfficiency(
  cacheReadTokens: number,
  inputTokens: number
): CacheEfficiency | null {
  if (inputTokens <= 0 || cacheReadTokens <= 0) return null;

  const rate = Math.min(1, cacheReadTokens / inputTokens);

  if (rate >= 0.5) {
    return { rate, label: "Excellent", color: "var(--success, #4ade80)", tier: "excellent" };
  }
  if (rate >= 0.2) {
    return { rate, label: "Good", color: "var(--accent, #60a5fa)", tier: "good" };
  }
  return { rate, label: "Low", color: "var(--warning, #fbbf24)", tier: "low" };
}

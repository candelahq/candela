/**
 * Cache-efficiency analysis utilities.
 *
 * Extracted from modelPricing.ts so they survive the deprecation of the
 * hardcoded pricing registry.
 */

export interface CacheEfficiency {
  /** Hit rate 0.0–1.0 */
  rate: number;
  label: string;
  /** CSS-friendly color */
  color: string;
  /** CSS class suffix: "excellent" | "good" | "low" */
  tier: "excellent" | "good" | "low";
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

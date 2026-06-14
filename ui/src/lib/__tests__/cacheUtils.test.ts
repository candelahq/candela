import { describe, it, expect } from 'vitest';
import { getCacheEfficiency } from '../cacheUtils';

describe('getCacheEfficiency', () => {
  it('returns null when inputTokens is zero', () => {
    expect(getCacheEfficiency(100, 0)).toBeNull();
  });

  it('returns null when inputTokens is negative', () => {
    expect(getCacheEfficiency(100, -1)).toBeNull();
  });

  it('returns null when cacheReadTokens is zero', () => {
    expect(getCacheEfficiency(0, 1000)).toBeNull();
  });

  it('returns null when cacheReadTokens is negative', () => {
    expect(getCacheEfficiency(-5, 1000)).toBeNull();
  });

  it('returns null when both are zero', () => {
    expect(getCacheEfficiency(0, 0)).toBeNull();
  });

  // Excellent: rate >= 0.5
  it('returns "Excellent" tier for rate >= 0.5', () => {
    const result = getCacheEfficiency(600, 1000);
    expect(result).not.toBeNull();
    expect(result!.label).toBe('Excellent');
    expect(result!.tier).toBe('excellent');
    expect(result!.rate).toBe(0.6);
  });

  it('returns "Excellent" at exactly 50% boundary', () => {
    const result = getCacheEfficiency(500, 1000);
    expect(result).not.toBeNull();
    expect(result!.label).toBe('Excellent');
    expect(result!.tier).toBe('excellent');
    expect(result!.rate).toBe(0.5);
  });

  // Good: 0.2 <= rate < 0.5
  it('returns "Good" tier for rate in [0.2, 0.5)', () => {
    const result = getCacheEfficiency(300, 1000);
    expect(result).not.toBeNull();
    expect(result!.label).toBe('Good');
    expect(result!.tier).toBe('good');
    expect(result!.rate).toBe(0.3);
  });

  it('returns "Good" at exactly 20% boundary', () => {
    const result = getCacheEfficiency(200, 1000);
    expect(result).not.toBeNull();
    expect(result!.label).toBe('Good');
    expect(result!.tier).toBe('good');
    expect(result!.rate).toBe(0.2);
  });

  // Low: rate < 0.2
  it('returns "Low" tier for rate < 0.2', () => {
    const result = getCacheEfficiency(50, 1000);
    expect(result).not.toBeNull();
    expect(result!.label).toBe('Low');
    expect(result!.tier).toBe('low');
    expect(result!.rate).toBe(0.05);
  });

  it('returns "Low" for very small rate', () => {
    const result = getCacheEfficiency(1, 10000);
    expect(result).not.toBeNull();
    expect(result!.label).toBe('Low');
    expect(result!.tier).toBe('low');
    expect(result!.rate).toBeCloseTo(0.0001);
  });

  // Edge: cacheReadTokens > inputTokens is clamped to 1.0
  it('clamps rate to 1.0 when cacheReadTokens exceeds inputTokens', () => {
    const result = getCacheEfficiency(2000, 1000);
    expect(result).not.toBeNull();
    expect(result!.rate).toBe(1);
    expect(result!.label).toBe('Excellent');
    expect(result!.tier).toBe('excellent');
  });

  // Color is always a string
  it('includes a CSS color string', () => {
    const result = getCacheEfficiency(500, 1000);
    expect(result).not.toBeNull();
    expect(typeof result!.color).toBe('string');
    expect(result!.color.length).toBeGreaterThan(0);
  });
});

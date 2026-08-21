import { describe, it, expect } from 'vitest';
import { fmtTokens } from '@/components/today/utils';

describe('fmtTokens', () => {
  it('formats millions correctly', () => {
    expect(fmtTokens(1_500_000)).toBe('1.5M');
    expect(fmtTokens(1_000_000)).toBe('1.0M');
    expect(fmtTokens(12_345_678)).toBe('12.3M');
  });

  it('formats thousands correctly', () => {
    expect(fmtTokens(1_500)).toBe('1.5k');
    expect(fmtTokens(999_999)).toBe('1000.0k'); // Based on logic: 999999 / 1000.toFixed(1)
    expect(fmtTokens(1_000)).toBe('1.0k');
  });

  it('formats regular numbers correctly', () => {
    expect(fmtTokens(999)).toBe('999');
    expect(fmtTokens(500)).toBe('500');
    expect(fmtTokens(0)).toBe('0');
  });
});

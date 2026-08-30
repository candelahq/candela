import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { renderHook, waitFor } from '@testing-library/react';
import { useUsage } from '../useUsage';

// ---------------------------------------------------------------------------
// Mock RPC client — useUsage imports dashboardClient from "@/lib/api".
// ---------------------------------------------------------------------------

const mockGetMyUsage = vi.fn();

vi.mock('@/lib/api', () => ({
  dashboardClient: {
    getMyUsage: (...args: unknown[]) => mockGetMyUsage(...args),
  },
}));

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

/** Minimal successful usage RPC response. */
function usageResponse(overrides?: {
  budget?: Record<string, unknown> | null;
  totalCalls?: bigint;
}) {
  return {
    totalCalls: overrides?.totalCalls ?? BigInt(10),
    totalInputTokens: BigInt(5000),
    totalOutputTokens: BigInt(3000),
    totalCostUsd: 1.5,
    avgLatencyMs: 200,
    totalRemainingUsd: 98.5,
    models: [],
    budget: overrides?.budget === null ? null : {
      limitUsd: 100,
      spentUsd: 1.5,
      periodType: 1,
      ...(overrides?.budget ?? {}),
    },
  };
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe('useUsage', () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it('returns loading state initially', () => {
    mockGetMyUsage.mockReturnValue(new Promise(() => {})); // never resolves
    const { result } = renderHook(() => useUsage());
    expect(result.current.loading).toBe(true);
    expect(result.current.data).toBeNull();
    expect(result.current.error).toBeNull();
  });

  it('returns data on successful fetch', async () => {
    mockGetMyUsage.mockResolvedValue(usageResponse());
    const { result } = renderHook(() => useUsage());

    await waitFor(() => expect(result.current.loading).toBe(false));

    expect(result.current.data).not.toBeNull();
    expect(result.current.data!.totalCalls).toBe(10);
    expect(result.current.data!.budget).not.toBeNull();
    expect(result.current.data!.budget!.limitUsd).toBe(100);
    expect(result.current.data!.budget!.spentUsd).toBe(1.5);
  });

  // -------------------------------------------------------------------------
  // #584 regression: $0 budget should NOT be treated as missing
  // -------------------------------------------------------------------------

  it('preserves limitUsd: 0 (does not fall back to undefined)', async () => {
    mockGetMyUsage.mockResolvedValue(usageResponse({
      budget: { limitUsd: 0, spentUsd: 0, periodType: 1 },
    }));
    const { result } = renderHook(() => useUsage());

    await waitFor(() => expect(result.current.loading).toBe(false));

    expect(result.current.data!.budget).not.toBeNull();
    expect(result.current.data!.budget!.limitUsd).toBe(0);
    expect(result.current.data!.budget!.spentUsd).toBe(0);
    expect(result.current.data!.budget!.percentUsed).toBe(0);
  });

  it('preserves spentUsd: 0 with positive limit', async () => {
    mockGetMyUsage.mockResolvedValue(usageResponse({
      budget: { limitUsd: 50, spentUsd: 0, periodType: 1 },
    }));
    const { result } = renderHook(() => useUsage());

    await waitFor(() => expect(result.current.loading).toBe(false));

    expect(result.current.data!.budget!.spentUsd).toBe(0);
    expect(result.current.data!.budget!.limitUsd).toBe(50);
    expect(result.current.data!.budget!.percentUsed).toBe(0);
  });

  it('handles null budget response', async () => {
    mockGetMyUsage.mockResolvedValue(usageResponse({ budget: null }));
    const { result } = renderHook(() => useUsage());

    await waitFor(() => expect(result.current.loading).toBe(false));

    expect(result.current.data!.budget).toBeNull();
  });

  it('handles undefined limitUsd/spentUsd via ?? fallback', async () => {
    // Server returns budget object but with undefined fields
    mockGetMyUsage.mockResolvedValue(usageResponse({
      budget: { limitUsd: undefined, spentUsd: undefined, periodType: 1 },
    }));
    const { result } = renderHook(() => useUsage());

    await waitFor(() => expect(result.current.loading).toBe(false));

    // ?? 0 should kick in for undefined values
    expect(result.current.data!.budget!.limitUsd).toBe(0);
    expect(result.current.data!.budget!.spentUsd).toBe(0);
  });

  // -------------------------------------------------------------------------
  // Budget percentage calculation
  // -------------------------------------------------------------------------

  it('calculates percentUsed correctly', async () => {
    mockGetMyUsage.mockResolvedValue(usageResponse({
      budget: { limitUsd: 200, spentUsd: 50, periodType: 1 },
    }));
    const { result } = renderHook(() => useUsage());

    await waitFor(() => expect(result.current.loading).toBe(false));

    expect(result.current.data!.budget!.percentUsed).toBe(25);
  });

  it('returns 0% when limit is 0 (avoids division by zero)', async () => {
    mockGetMyUsage.mockResolvedValue(usageResponse({
      budget: { limitUsd: 0, spentUsd: 10, periodType: 1 },
    }));
    const { result } = renderHook(() => useUsage());

    await waitFor(() => expect(result.current.loading).toBe(false));

    expect(result.current.data!.budget!.percentUsed).toBe(0);
  });

  // -------------------------------------------------------------------------
  // Error handling
  // -------------------------------------------------------------------------

  it('returns error on fetch failure', async () => {
    mockGetMyUsage.mockRejectedValue(new Error('network error'));
    const { result } = renderHook(() => useUsage());

    await waitFor(() => expect(result.current.loading).toBe(false));

    expect(result.current.error).toBe('network error');
    expect(result.current.data).toBeNull();
  });

  // -------------------------------------------------------------------------
  // Reducer: timeRangeToMs / setTimeRange
  // -------------------------------------------------------------------------

  it('defaults to 7d time range', () => {
    mockGetMyUsage.mockReturnValue(new Promise(() => {}));
    const { result } = renderHook(() => useUsage());
    expect(result.current.timeRange).toBe('7d');
  });
});

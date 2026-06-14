import { describe, it, expect, vi, beforeEach, afterEach } from 'vitest';
import { renderHook, waitFor, act } from '@testing-library/react';
import { useDashboard } from '../useDashboard';

// ---------------------------------------------------------------------------
// Mock RPC clients — useDashboard imports dashboardClient + traceClient
// from "@/lib/api".
// ---------------------------------------------------------------------------

const mockGetDashboardData = vi.fn();
const mockListTraces = vi.fn();
const mockGetJobLeaderboard = vi.fn();

vi.mock('@/lib/api', () => ({
  dashboardClient: {
    getDashboardData: (...args: unknown[]) => mockGetDashboardData(...args),
    getJobLeaderboard: (...args: unknown[]) => mockGetJobLeaderboard(...args),
  },
  traceClient: {
    listTraces: (...args: unknown[]) => mockListTraces(...args),
  },
}));

// ---------------------------------------------------------------------------
// Helpers
// ---------------------------------------------------------------------------

/** Minimal successful dashboard RPC response. */
function dashboardResponse(overrides?: {
  summary?: Record<string, unknown>;
  models?: Array<Record<string, unknown>>;
  budgetContext?: Record<string, unknown> | null;
}) {
  return {
    summary: {
      totalTraces: BigInt(42),
      totalSpans: BigInt(100),
      totalLlmCalls: BigInt(10),
      totalInputTokens: BigInt(5000),
      totalOutputTokens: BigInt(3000),
      totalCostUsd: 1.5,
      avgLatencyMs: 200,
      errorRate: 0.02,
      totalCacheReadTokens: BigInt(1000),
      totalCacheCreationTokens: BigInt(500),
      tracesOverTime: [],
      costOverTime: [],
      tokensOverTime: [],
      ...(overrides?.summary ?? {}),
    },
    models: overrides?.models ?? [
      {
        model: 'gemini-2.5-pro',
        provider: 'google',
        callCount: BigInt(8),
        inputTokens: BigInt(4000),
        outputTokens: BigInt(2000),
        costUsd: 1.2,
        avgLatencyMs: 180,
        cacheReadTokens: BigInt(800),
        cacheCreationTokens: BigInt(400),
      },
    ],
    budgetContext: overrides?.budgetContext !== undefined
      ? overrides.budgetContext
      : null,
  };
}

function tracesResponse(traces?: Array<Record<string, unknown>>) {
  return {
    traces: traces ?? [
      {
        traceId: 'trace-1',
        rootSpanName: 'my-span',
        primaryModel: 'gemini-2.5-pro',
        spanCount: 3,
        totalTokens: BigInt(1000),
        totalCostUsd: 0.5,
        duration: { seconds: BigInt(1), nanos: 500_000_000 },
        status: 0,
        startTime: { seconds: BigInt(1718000000), nanos: 0 },
      },
    ],
  };
}

function jobLeaderboardResponse(
  jobs?: Array<Record<string, unknown>>,
) {
  return {
    jobs: jobs ?? [],
  };
}

/** Wire up all three mocks with valid defaults. */
function mockAllSuccess(
  dashOverrides?: Parameters<typeof dashboardResponse>[0],
) {
  mockGetDashboardData.mockResolvedValue(dashboardResponse(dashOverrides));
  mockListTraces.mockResolvedValue(tracesResponse());
  mockGetJobLeaderboard.mockResolvedValue(jobLeaderboardResponse());
}

// ---------------------------------------------------------------------------
// Tests
// ---------------------------------------------------------------------------

describe('useDashboard', () => {
  beforeEach(() => {
    mockGetDashboardData.mockReset();
    mockListTraces.mockReset();
    mockGetJobLeaderboard.mockReset();
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  // ── 1. Loading state on mount ────────────────────────────────────────

  it('starts in loading state', () => {
    // Never resolve — we just want to check initial state.
    mockGetDashboardData.mockReturnValue(new Promise(() => {}));
    mockListTraces.mockReturnValue(new Promise(() => {}));
    mockGetJobLeaderboard.mockReturnValue(new Promise(() => {}));

    const { result } = renderHook(() => useDashboard());

    expect(result.current.loading).toBe(true);
    expect(result.current.error).toBeNull();
    expect(result.current.models).toEqual([]);
    expect(result.current.summary).toBeNull();
  });

  // ── 2. Success transition ────────────────────────────────────────────

  it('fetches data and transitions to success', async () => {
    mockAllSuccess();

    const { result } = renderHook(() => useDashboard());

    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });

    expect(result.current.error).toBeNull();
    expect(result.current.models).toHaveLength(1);
    expect(result.current.models[0].model).toBe('gemini-2.5-pro');
    expect(result.current.models[0].callCount).toBe(8);
    expect(result.current.summary).not.toBeNull();
    expect(result.current.summary!.totalTraces).toBe(42);
    expect(result.current.summary!.totalCostUsd).toBe(1.5);
    expect(result.current.recentTraces).toHaveLength(1);
    expect(result.current.recentTraces[0].traceId).toBe('trace-1');
  });

  // ── 3. Error state ──────────────────────────────────────────────────

  it('sets error state on RPC failure', async () => {
    mockGetDashboardData.mockRejectedValue(new Error('server exploded'));
    mockListTraces.mockResolvedValue(tracesResponse());
    mockGetJobLeaderboard.mockResolvedValue(jobLeaderboardResponse());

    const { result } = renderHook(() => useDashboard());

    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });

    expect(result.current.error).toBe('server exploded');
  });

  // ── 4. AbortController on unmount ───────────────────────────────────

  it('aborts in-flight requests on unmount', async () => {
    let capturedSignal: AbortSignal | null = null;
    mockGetDashboardData.mockImplementation(
      (_req: unknown, opts: { signal: AbortSignal }) => {
        capturedSignal = opts?.signal ?? null;
        return new Promise(() => {}); // Never resolves
      },
    );
    mockListTraces.mockReturnValue(new Promise(() => {}));
    mockGetJobLeaderboard.mockReturnValue(new Promise(() => {}));

    const { unmount } = renderHook(() => useDashboard());
    await waitFor(() => expect(mockGetDashboardData).toHaveBeenCalled());

    unmount();

    expect(capturedSignal).not.toBeNull();
    expect(capturedSignal!.aborted).toBe(true);
  });

  // ── 5. Time range change triggers refetch ───────────────────────────

  it('refetches when time range changes', async () => {
    mockAllSuccess();

    const { result } = renderHook(() => useDashboard());

    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });

    expect(mockGetDashboardData).toHaveBeenCalledTimes(1);

    // Change time range — should trigger another fetch
    act(() => {
      result.current.setTimeRange('7d');
    });

    await waitFor(() => {
      expect(mockGetDashboardData).toHaveBeenCalledTimes(2);
    });

    expect(result.current.timeRange).toBe('7d');
  });

  // ── 6. Nullish coalescing — partial response with null fields ───────

  it('applies nullish coalescing defaults for partial response', async () => {
    mockGetDashboardData.mockResolvedValue({
      summary: {
        // All fields null / undefined — should fall back to 0
        totalTraces: null,
        totalSpans: null,
        totalLlmCalls: null,
        totalInputTokens: null,
        totalOutputTokens: null,
        totalCostUsd: null,
        avgLatencyMs: null,
        errorRate: null,
        totalCacheReadTokens: null,
        totalCacheCreationTokens: null,
        tracesOverTime: undefined,
        costOverTime: undefined,
        tokensOverTime: undefined,
      },
      models: [],
      budgetContext: null,
    });
    mockListTraces.mockResolvedValue({ traces: [] });
    mockGetJobLeaderboard.mockResolvedValue({ jobs: [] });

    const { result } = renderHook(() => useDashboard());

    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });

    const s = result.current.summary!;
    expect(s.totalTraces).toBe(0);
    expect(s.totalSpans).toBe(0);
    expect(s.totalLlmCalls).toBe(0);
    expect(s.totalInputTokens).toBe(0);
    expect(s.totalOutputTokens).toBe(0);
    expect(s.totalCostUsd).toBe(0);
    expect(s.avgLatencyMs).toBe(0);
    expect(s.errorRate).toBe(0);
    expect(s.totalCacheReadTokens).toBe(0);
    expect(s.totalCacheCreationTokens).toBe(0);
    expect(s.tracesOverTime).toEqual([]);
    expect(s.costOverTime).toEqual([]);
    expect(s.tokensOverTime).toEqual([]);
    expect(result.current.models).toEqual([]);
  });
});

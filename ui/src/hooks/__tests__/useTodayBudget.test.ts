import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { renderHook, waitFor, act } from "@testing-library/react";
import { useTodayBudget } from "../useTodayBudget";

const mockGetMyUsage = vi.fn();
const mockGetMyBudget = vi.fn();

vi.mock("@/lib/api", () => ({
  dashboardClient: {
    getMyUsage: (...args: unknown[]) => mockGetMyUsage(...args),
  },
  userClient: {
    getMyBudget: (...args: unknown[]) => mockGetMyBudget(...args),
  },
}));

function mockUsageResponse(overrides?: Record<string, unknown>) {
  return {
    totalCalls: BigInt(25),
    totalInputTokens: BigInt(12000),
    totalOutputTokens: BigInt(4000),
    totalCostUsd: 1.25,
    avgLatencyMs: 180,
    models: [
      {
        model: "gemini-2.5-pro",
        provider: "google",
        callCount: BigInt(15),
        inputTokens: BigInt(8000),
        outputTokens: BigInt(2500),
        costUsd: 0.95,
        avgLatencyMs: 210,
      },
    ],
    ...overrides,
  };
}

function mockBudgetResponse(overrides?: Record<string, unknown>) {
  return {
    budget: {
      limitUsd: 20,
      spentUsd: 5.5,
      periodType: 1, // daily
    },
    budgetRemainingUsd: 14.5,
    activeGrants: [
      {
        id: "grant-1",
        amountUsd: 50,
        spentUsd: 10,
        reason: "benchmark test",
        grantedBy: "admin@azra-ai.com",
        expiresAt: { seconds: BigInt(1787530400), nanos: 0 },
      },
    ],
    periodResetsAt: "2026-09-08T00:00:00Z",
    ...overrides,
  };
}

describe("useTodayBudget", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    vi.useFakeTimers({ shouldAdvanceTime: true });
  });

  afterEach(() => {
    vi.useRealTimers();
    vi.restoreAllMocks();
  });

  it("returns initial loading state", () => {
    mockGetMyUsage.mockReturnValue(new Promise(() => {}));
    mockGetMyBudget.mockReturnValue(new Promise(() => {}));

    const { result } = renderHook(() => useTodayBudget());
    expect(result.current.loading).toBe(true);
    expect(result.current.data).toBeNull();
    expect(result.current.error).toBeNull();
  });

  it("successfully fetches and aggregates usage and budget data", async () => {
    mockGetMyUsage.mockResolvedValue(mockUsageResponse());
    mockGetMyBudget.mockResolvedValue(mockBudgetResponse());

    const { result } = renderHook(() => useTodayBudget());

    await waitFor(() => expect(result.current.loading).toBe(false));

    const data = result.current.data!;
    expect(data).not.toBeNull();
    expect(data.totalCalls).toBe(25);
    expect(data.totalInputTokens).toBe(12000);
    expect(data.totalOutputTokens).toBe(4000);
    expect(data.totalCostUsd).toBe(1.25);
    expect(data.avgLatencyMs).toBe(180);

    // Models mapped correctly
    expect(data.models).toHaveLength(1);
    expect(data.models[0].model).toBe("gemini-2.5-pro");
    expect(data.models[0].costUsd).toBe(0.95);

    // Budget mapped correctly
    expect(data.budget).not.toBeNull();
    expect(data.budget!.limitUsd).toBe(20);
    expect(data.budget!.spentUsd).toBe(5.5);
    expect(data.budget!.remainingUsd).toBe(14.5);
    expect(data.budget!.percentUsed).toBeCloseTo(27.5);
    expect(data.budget!.periodType).toBe("daily");

    // Grants mapped correctly
    expect(data.grants).toHaveLength(1);
    expect(data.grants[0].id).toBe("grant-1");
    expect(data.grants[0].amountUsd).toBe(50);
    expect(data.grants[0].spentUsd).toBe(10);
    expect(data.grants[0].remainingUsd).toBe(40);
    expect(data.grants[0].percentUsed).toBe(20);
    expect(data.grants[0].reason).toBe("benchmark test");

    expect(data.periodResetsAt).toBe("2026-09-08T00:00:00Z");
  });

  it("handles empty budget gracefully (unconfigured budget)", async () => {
    mockGetMyUsage.mockResolvedValue(mockUsageResponse());
    mockGetMyBudget.mockResolvedValue({
      budget: null,
      budgetRemainingUsd: 0,
      activeGrants: [],
    });

    const { result } = renderHook(() => useTodayBudget());

    await waitFor(() => expect(result.current.loading).toBe(false));

    expect(result.current.data!.budget).toBeNull();
    expect(result.current.data!.grants).toEqual([]);
  });

  it("handles budget RPC error gracefully without failing usage data", async () => {
    mockGetMyUsage.mockResolvedValue(mockUsageResponse());
    mockGetMyBudget.mockRejectedValue(new Error("budget service unavailable"));

    const { result } = renderHook(() => useTodayBudget());

    await waitFor(() => expect(result.current.loading).toBe(false));

    expect(result.current.error).toBeNull();
    expect(result.current.data!.totalCalls).toBe(25);
    expect(result.current.data!.budget).toBeNull();
  });

  it("returns error state when usage RPC fails", async () => {
    mockGetMyUsage.mockRejectedValue(new Error("BigQuery timeout"));
    mockGetMyBudget.mockResolvedValue(mockBudgetResponse());

    const { result } = renderHook(() => useTodayBudget());

    await waitFor(() => expect(result.current.loading).toBe(false));

    expect(result.current.error).toBe("BigQuery timeout");
    expect(result.current.data).toBeNull();
  });

  it("triggers manual refresh when refresh() is called", async () => {
    mockGetMyUsage.mockResolvedValueOnce(mockUsageResponse({ totalCostUsd: 1.25 }));
    mockGetMyBudget.mockResolvedValueOnce(mockBudgetResponse({ budget: { limitUsd: 20, spentUsd: 5.5, periodType: 1 } }));

    const { result } = renderHook(() => useTodayBudget());

    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(mockGetMyUsage).toHaveBeenCalledTimes(1);
    expect(mockGetMyBudget).toHaveBeenCalledTimes(1);
    expect(result.current.data?.budget?.limitUsd).toBe(20);

    mockGetMyUsage.mockResolvedValueOnce(mockUsageResponse({ totalCostUsd: 3.50 }));
    mockGetMyBudget.mockResolvedValueOnce(mockBudgetResponse({ budget: { limitUsd: 50, spentUsd: 12.0, periodType: 1 } }));

    act(() => {
      result.current.refresh();
    });

    await waitFor(() => expect(mockGetMyUsage).toHaveBeenCalledTimes(2));
    await waitFor(() => expect(mockGetMyBudget).toHaveBeenCalledTimes(2));
    expect(result.current.data?.totalCostUsd).toBe(3.50);
    expect(result.current.data?.budget?.limitUsd).toBe(50);
  });

  it("stops auto-refreshing after reaching maxConsecutiveErrors and applies exponential backoff", async () => {
    mockGetMyUsage.mockRejectedValue(new Error("network error"));
    mockGetMyBudget.mockResolvedValue(null);

    const { result } = renderHook(() =>
      useTodayBudget({ baseIntervalMs: 1000, maxConsecutiveErrors: 3 })
    );

    // Initial fetch fails -> consecutiveErrors = 1
    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(result.current.consecutiveErrors).toBe(1);
    expect(mockGetMyUsage).toHaveBeenCalledTimes(1);

    // 1st backoff delay: 1000 * 2^1 = 2000ms
    // Advance 1500ms - should not have refreshed yet
    act(() => {
      vi.advanceTimersByTime(1500);
    });
    expect(mockGetMyUsage).toHaveBeenCalledTimes(1);

    // Advance remaining 500ms - triggers 2nd fetch -> consecutiveErrors = 2
    act(() => {
      vi.advanceTimersByTime(500);
    });
    await waitFor(() => expect(result.current.consecutiveErrors).toBe(2));
    expect(mockGetMyUsage).toHaveBeenCalledTimes(2);

    // 2nd backoff delay: 1000 * 2^2 = 4000ms
    act(() => {
      vi.advanceTimersByTime(4000);
    });
    await waitFor(() => expect(result.current.consecutiveErrors).toBe(3));
    expect(mockGetMyUsage).toHaveBeenCalledTimes(3);

    // Max errors (3) reached: auto-refresh is stopped
    act(() => {
      vi.advanceTimersByTime(60_000);
    });
    // No new calls should be made
    expect(mockGetMyUsage).toHaveBeenCalledTimes(3);

    // Now manually refresh with a successful response
    mockGetMyUsage.mockResolvedValueOnce(mockUsageResponse());
    mockGetMyBudget.mockResolvedValueOnce(mockBudgetResponse());

    act(() => {
      result.current.refresh();
    });

    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(mockGetMyUsage).toHaveBeenCalledTimes(4);
    expect(result.current.consecutiveErrors).toBe(0);
    expect(result.current.data).not.toBeNull();
  });

  it("resets consecutive errors counter to 0 on successful refresh", async () => {
    mockGetMyUsage.mockRejectedValueOnce(new Error("temporary error"));
    mockGetMyBudget.mockResolvedValueOnce(null);

    const { result } = renderHook(() =>
      useTodayBudget({ baseIntervalMs: 1000, maxConsecutiveErrors: 3 })
    );

    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(result.current.consecutiveErrors).toBe(1);

    mockGetMyUsage.mockResolvedValueOnce(mockUsageResponse());
    mockGetMyBudget.mockResolvedValueOnce(mockBudgetResponse());

    // Advance 2000ms (1000 * 2^1)
    act(() => {
      vi.advanceTimersByTime(2000);
    });

    await waitFor(() => expect(result.current.consecutiveErrors).toBe(0));
    expect(result.current.data?.totalCalls).toBe(25);
  });

  it("increments consecutive errors and reaches error limit on repeated budget RPC failures (partial success)", async () => {
    mockGetMyUsage.mockResolvedValue(mockUsageResponse());
    mockGetMyBudget.mockRejectedValue(new Error("firestore budget error"));

    const { result } = renderHook(() =>
      useTodayBudget({ baseIntervalMs: 1000, maxConsecutiveErrors: 2 })
    );

    // Initial fetch: usage succeeds, but budget rejects -> consecutiveErrors = 1
    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(result.current.data?.totalCalls).toBe(25);
    expect(result.current.data?.budget).toBeNull();
    expect(result.current.consecutiveErrors).toBe(1);

    // 1st backoff delay: 1000 * 2^1 = 2000ms
    act(() => {
      vi.advanceTimersByTime(2000);
    });
    // 2nd fetch -> consecutiveErrors = 2 (max reached)
    await waitFor(() => expect(result.current.consecutiveErrors).toBe(2));
    expect(mockGetMyUsage).toHaveBeenCalledTimes(2);

    // Advance 60s - should NOT make further calls because maxConsecutiveErrors was reached
    act(() => {
      vi.advanceTimersByTime(60_000);
    });
    expect(mockGetMyUsage).toHaveBeenCalledTimes(2);
  });
});

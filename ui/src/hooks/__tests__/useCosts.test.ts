import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { renderHook, waitFor, act } from "@testing-library/react";
import { useCosts } from "../useCosts";

const mockGetUsageSummary = vi.fn();
const mockGetModelBreakdown = vi.fn();

vi.mock("@/lib/api", () => ({
  dashboardClient: {
    getUsageSummary: (...args: unknown[]) => mockGetUsageSummary(...args),
    getModelBreakdown: (...args: unknown[]) => mockGetModelBreakdown(...args),
  },
}));

function mockUsageSummary(overrides?: Record<string, unknown>) {
  return {
    totalCostUsd: 12.34,
    totalInputTokens: BigInt(10000),
    totalOutputTokens: BigInt(5000),
    totalTraces: BigInt(42),
    costOverTime: [
      { timestamp: "2026-04-15T10:00:00Z", value: 1.2 },
      { timestamp: "2026-04-15T11:00:00Z", value: 2.3 },
    ],
    tokensOverTime: [
      { timestamp: "2026-04-15T10:00:00Z", value: 1000 },
      { timestamp: "2026-04-15T11:00:00Z", value: 2000 },
    ],
    ...overrides,
  };
}

function mockModelBreakdown(overrides?: Record<string, unknown>) {
  return {
    models: [
      {
        model: "claude-3-5-sonnet",
        provider: "anthropic",
        callCount: BigInt(20),
        inputTokens: BigInt(6000),
        outputTokens: BigInt(3000),
        costUsd: 8.5,
        avgLatencyMs: 320,
        cacheReadTokens: BigInt(1000),
        cacheCreationTokens: BigInt(500),
      },
    ],
    ...overrides,
  };
}

describe("useCosts", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("starts in loading state", () => {
    mockGetUsageSummary.mockReturnValue(new Promise(() => {}));
    mockGetModelBreakdown.mockReturnValue(new Promise(() => {}));

    const { result } = renderHook(() => useCosts());
    expect(result.current.loading).toBe(true);
    expect(result.current.summary).toBeNull();
    expect(result.current.models).toEqual([]);
    expect(result.current.error).toBeNull();
  });

  it("fetches data and populates summary and models", async () => {
    mockGetUsageSummary.mockResolvedValue(mockUsageSummary());
    mockGetModelBreakdown.mockResolvedValue(mockModelBreakdown());

    const { result } = renderHook(() => useCosts());

    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });

    expect(result.current.summary?.totalCostUsd).toBe(12.34);
    expect(result.current.summary?.totalInputTokens).toBe(10000);
    expect(result.current.summary?.totalOutputTokens).toBe(5000);
    expect(result.current.summary?.totalTraces).toBe(42);
    expect(result.current.summary?.costOverTime).toHaveLength(2);
    expect(result.current.summary?.tokensOverTime).toHaveLength(2);

    expect(result.current.models).toHaveLength(1);
    expect(result.current.models[0].model).toBe("claude-3-5-sonnet");
    expect(result.current.models[0].callCount).toBe(20);
  });

  it("handles errors from RPC failure", async () => {
    mockGetUsageSummary.mockRejectedValue(new Error("summary error"));
    mockGetModelBreakdown.mockResolvedValue(mockModelBreakdown());

    const { result } = renderHook(() => useCosts());

    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });

    expect(result.current.error).toBe("summary error");
  });

  it("re-fetches when timeRange changes", async () => {
    mockGetUsageSummary.mockResolvedValue(mockUsageSummary());
    mockGetModelBreakdown.mockResolvedValue(mockModelBreakdown());

    const { result } = renderHook(() => useCosts());

    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });

    expect(mockGetUsageSummary).toHaveBeenCalledTimes(1);

    act(() => {
      result.current.setTimeRange("24h");
    });

    await waitFor(() => {
      expect(mockGetUsageSummary).toHaveBeenCalledTimes(2);
    });

    expect(result.current.timeRange).toBe("24h");

    // Setting the same time range should NOT trigger another fetch
    act(() => {
      result.current.setTimeRange("24h");
    });

    expect(mockGetUsageSummary).toHaveBeenCalledTimes(2);
  });

  it("refreshes manually when refresh() is called", async () => {
    mockGetUsageSummary.mockResolvedValue(mockUsageSummary());
    mockGetModelBreakdown.mockResolvedValue(mockModelBreakdown());

    const { result } = renderHook(() => useCosts());

    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });

    expect(mockGetUsageSummary).toHaveBeenCalledTimes(1);

    act(() => {
      result.current.refresh();
    });

    await waitFor(() => {
      expect(mockGetUsageSummary).toHaveBeenCalledTimes(2);
    });
  });
});

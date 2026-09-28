import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { renderHook, waitFor, act } from "@testing-library/react";
import { useTraces } from "../useTraces";
import { DEFAULT_FILTERS } from "@/types/traces";

const mockListTraces = vi.fn();

vi.mock("@/lib/api", () => ({
  traceClient: {
    listTraces: (...args: unknown[]) => mockListTraces(...args),
  },
}));

vi.mock("@/components/UserScopeProvider", () => ({
  useScope: () => ({ isPersonalScope: false, mode: "team" }),
}));

function mockListResponse() {
  return Promise.resolve({
    traces: [
      {
        traceId: "trace-abc",
        rootSpanName: "chat",
        primaryModel: "gpt-4o",
        primaryProvider: "openai",
        environment: "production",
        totalTokens: BigInt(250),
        totalCostUsd: 0.012,
        status: 1,
        spanCount: 3,
        llmCallCount: 1,
        startTime: { seconds: BigInt(1710000000), nanos: 0 },
        duration: { seconds: BigInt(1), nanos: 200_000_000 },
      },
    ],
    pagination: {
      nextPageToken: "token-page-2",
    },
  });
}

describe("useTraces", () => {
  beforeEach(() => {
    vi.clearAllMocks();
    mockListTraces.mockImplementation(() => mockListResponse());
    window.history.replaceState(null, "", "/traces");
  });

  afterEach(() => {
    window.history.replaceState(null, "", "/traces");
    vi.restoreAllMocks();
  });

  it("initializes with default filters when no query params exist", async () => {
    const { result } = renderHook(() => useTraces({ syncUrl: false }));
    expect(result.current.filters).toEqual(DEFAULT_FILTERS);
    expect(result.current.hasActiveFilters).toBe(false);
  });

  it("restores filters from URL query parameters on initialization", async () => {
    window.history.replaceState(null, "", "/traces?model=gpt-4o&timeRange=7d&status=error&search=query");

    const { result } = renderHook(() => useTraces({ syncUrl: true }));

    expect(result.current.filters.model).toBe("gpt-4o");
    expect(result.current.filters.timeRange).toBe("7d");
    expect(result.current.filters.status).toBe("error");
    expect(result.current.filters.search).toBe("query");
    expect(result.current.hasActiveFilters).toBe(true);

    act(() => {
      result.current.fetchInitial();
    });

    await waitFor(() => {
      expect(mockListTraces).toHaveBeenCalledWith(
        expect.objectContaining({
          model: "gpt-4o",
          search: "query",
        }),
        expect.anything()
      );
    });
  });

  it("updates URL when non-search filters are modified", async () => {
    const replaceStateSpy = vi.spyOn(window.history, "replaceState");
    const { result } = renderHook(() => useTraces({ syncUrl: true }));

    await act(async () => {
      result.current.updateFilters({ model: "claude-3-5-sonnet", status: "ok" });
    });

    expect(result.current.filters.model).toBe("claude-3-5-sonnet");
    expect(result.current.filters.status).toBe("ok");

    expect(replaceStateSpy).toHaveBeenCalledWith(
      null,
      "",
      "/traces?model=claude-3-5-sonnet&status=ok"
    );
  });

  it("clears filters and restores URL to base path", async () => {
    window.history.replaceState(null, "", "/traces?model=gpt-4o");
    const replaceStateSpy = vi.spyOn(window.history, "replaceState");

    const { result } = renderHook(() => useTraces({ syncUrl: true }));
    expect(result.current.filters.model).toBe("gpt-4o");

    await act(async () => {
      result.current.clearFilters();
    });

    expect(result.current.filters).toEqual(DEFAULT_FILTERS);
    expect(replaceStateSpy).toHaveBeenCalledWith(null, "", "/traces");
  });

  it("restores filters when popstate event fires", async () => {
    const { result } = renderHook(() => useTraces({ syncUrl: true }));
    expect(result.current.filters.model).toBe("");

    await act(async () => {
      window.history.replaceState(null, "", "/traces?model=deepseek-r1&timeRange=30d");
      window.dispatchEvent(new PopStateEvent("popstate"));
    });

    expect(result.current.filters.model).toBe("deepseek-r1");
    expect(result.current.filters.timeRange).toBe("30d");
  });

  it("preserves rapid successive filter updates without dropping earlier updates", async () => {
    const { result } = renderHook(() => useTraces({ syncUrl: false }));

    await act(async () => {
      result.current.updateFilters({ provider: "anthropic" });
      result.current.updateFilters({ model: "claude-3-5-sonnet" });
      result.current.updateFilters({ environment: "production" });
    });

    expect(result.current.filters.provider).toBe("anthropic");
    expect(result.current.filters.model).toBe("claude-3-5-sonnet");
    expect(result.current.filters.environment).toBe("production");
  });
});

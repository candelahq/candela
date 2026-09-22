import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { renderHook, waitFor, act } from "@testing-library/react";
import { useLeaderboard } from "../useLeaderboard";

const mockGetTeamLeaderboard = vi.fn();

vi.mock("@/lib/api", () => ({
  dashboardClient: {
    getTeamLeaderboard: (...args: unknown[]) => mockGetTeamLeaderboard(...args),
  },
}));

function mockLeaderboardResponse() {
  return {
    users: [
      {
        userId: "user-1",
        email: "alice@example.com",
        displayName: "Alice",
        callCount: BigInt(50),
        totalTokens: BigInt(15000),
        costUsd: 4.25,
        avgLatencyMs: 180,
        topModel: "claude-3-5-sonnet",
      },
    ],
  };
}

describe("useLeaderboard", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("starts in loading state", () => {
    mockGetTeamLeaderboard.mockReturnValue(new Promise(() => {}));

    const { result } = renderHook(() => useLeaderboard());
    expect(result.current.loading).toBe(true);
    expect(result.current.rankings).toEqual([]);
    expect(result.current.error).toBeNull();
  });

  it("fetches rankings and populates state", async () => {
    mockGetTeamLeaderboard.mockResolvedValue(mockLeaderboardResponse());

    const { result } = renderHook(() => useLeaderboard());

    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });

    expect(result.current.rankings).toHaveLength(1);
    expect(result.current.rankings[0].userId).toBe("user-1");
    expect(result.current.rankings[0].callCount).toBe(50);
  });

  it("re-fetches when timeRange changes and avoids duplicate fetch on same range", async () => {
    mockGetTeamLeaderboard.mockResolvedValue(mockLeaderboardResponse());

    const { result } = renderHook(() => useLeaderboard());

    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });

    expect(mockGetTeamLeaderboard).toHaveBeenCalledTimes(1);

    act(() => {
      result.current.setTimeRange("24h");
    });

    await waitFor(() => {
      expect(mockGetTeamLeaderboard).toHaveBeenCalledTimes(2);
    });

    expect(result.current.timeRange).toBe("24h");

    // Setting same range should not trigger an extra fetch
    act(() => {
      result.current.setTimeRange("24h");
    });

    expect(mockGetTeamLeaderboard).toHaveBeenCalledTimes(2);
  });

  it("handles RPC error gracefully", async () => {
    mockGetTeamLeaderboard.mockRejectedValue(new Error("failed to fetch leaderboard"));

    const { result } = renderHook(() => useLeaderboard());

    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });

    expect(result.current.error).toBe("failed to fetch leaderboard");
  });
});

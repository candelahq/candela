import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { renderHook, waitFor, act } from "@testing-library/react";
import { useForecast } from "../useForecast";

const mockGetIdToken = vi.fn();
let mockCurrentUser: { uid: string; getIdToken: typeof mockGetIdToken } | null = null;

vi.mock("@/lib/firebase", () => ({
  get firebaseAuth() {
    return {
      currentUser: mockCurrentUser,
    };
  },
}));

vi.mock("@/lib/constants", () => ({
  API_BASE_URL: "http://localhost:8181",
}));

describe("useForecast", () => {
  const originalFetch = global.fetch;

  beforeEach(() => {
    vi.clearAllMocks();
    mockGetIdToken.mockResolvedValue("mock-firebase-token");
    mockCurrentUser = {
      uid: "user-123",
      getIdToken: mockGetIdToken,
    };
  });

  afterEach(() => {
    global.fetch = originalFetch;
    vi.restoreAllMocks();
  });

  it("returns unauthenticated error when no firebase user", async () => {
    mockCurrentUser = null;
    const { result } = renderHook(() => useForecast());

    await waitFor(() => expect(result.current.loading).toBe(false));

    expect(result.current.error).toBe("Not authenticated");
    expect(result.current.forecast).toBeNull();
  });

  it("fetches and parses forecast data successfully", async () => {
    const mockApiResponse = {
      burn_rate_usd_per_hour: 0.45,
      projected_eod_spend_usd: 12.5,
      will_exceed_budget: true,
      avg_daily_spend_usd: 8.75,
      estimated_exhaustion_date: "2026-09-08",
      days_until_exhaustion: 1,
      spend_history: [
        { date: "2026-09-06", spend_usd: 7.5, token_count: 50000 },
        { date: "2026-09-07", spend_usd: 9.2, token_count: 65000 },
      ],
    };

    global.fetch = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: async () => mockApiResponse,
    } as unknown as Response);

    const { result } = renderHook(() => useForecast());

    await waitFor(() => expect(result.current.loading).toBe(false));

    expect(result.current.error).toBeNull();
    const forecast = result.current.forecast!;
    expect(forecast).not.toBeNull();
    expect(forecast.burnRatePerHour).toBe(0.45);
    expect(forecast.projectedEodSpend).toBe(12.5);
    expect(forecast.willExceedBudget).toBe(true);
    expect(forecast.avgDailySpend).toBe(8.75);
    expect(forecast.estimatedExhaustionDate).toBe("2026-09-08");
    expect(forecast.daysUntilExhaustion).toBe(1);
    expect(forecast.spendHistory).toHaveLength(2);
    expect(forecast.spendHistory[0].spend_usd).toBe(7.5);

    expect(global.fetch).toHaveBeenCalledWith(
      "http://localhost:8181/api/v1/users/user-123/budget-forecast",
      expect.objectContaining({
        headers: { Authorization: "Bearer mock-firebase-token" },
      })
    );
  });

  it("handles HTTP error response", async () => {
    global.fetch = vi.fn().mockResolvedValue({
      ok: false,
      status: 500,
    } as unknown as Response);

    const { result } = renderHook(() => useForecast());

    await waitFor(() => expect(result.current.loading).toBe(false));

    expect(result.current.error).toBe("HTTP 500");
    expect(result.current.forecast).toBeNull();
  });

  it("handles network failure gracefully", async () => {
    global.fetch = vi.fn().mockRejectedValue(new Error("Connection refused"));

    const { result } = renderHook(() => useForecast());

    await waitFor(() => expect(result.current.loading).toBe(false));

    expect(result.current.error).toBe("Connection refused");
    expect(result.current.forecast).toBeNull();
  });

  it("refreshes on manual refresh() call", async () => {
    global.fetch = vi.fn().mockResolvedValue({
      ok: true,
      status: 200,
      json: async () => ({ burn_rate_usd_per_hour: 0.2 }),
    } as unknown as Response);

    const { result } = renderHook(() => useForecast());

    await waitFor(() => expect(result.current.loading).toBe(false));
    expect(global.fetch).toHaveBeenCalledTimes(1);

    act(() => {
      result.current.refresh();
    });

    await waitFor(() => expect(global.fetch).toHaveBeenCalledTimes(2));
  });
});

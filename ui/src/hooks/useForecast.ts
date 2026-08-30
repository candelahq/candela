"use client";

import { useCallback, useEffect, useReducer } from "react";
import { API_BASE_URL } from "@/lib/constants";
import { firebaseAuth } from "@/lib/firebase";

export interface ForecastData {
  burnRatePerHour: number;
  projectedEodSpend: number;
  willExceedBudget: boolean;
  avgDailySpend: number;
  estimatedExhaustionDate: string; // "2026-08-28" or ""
  daysUntilExhaustion: number; // -1 if N/A
  spendHistory: { date: string; spend_usd: number; token_count: number }[];
}

interface State {
  data: ForecastData | null;
  loading: boolean;
  error: string | null;
  fetchCount: number;
}

type Action =
  | { type: "fetch" }
  | { type: "success"; data: ForecastData }
  | { type: "error"; message: string }
  | { type: "refresh" };

function reducer(state: State, action: Action): State {
  switch (action.type) {
    case "fetch":
      return { ...state, loading: true, error: null };
    case "success":
      return { ...state, loading: false, data: action.data };
    case "error":
      return { ...state, loading: false, error: action.message };
    case "refresh":
      return { ...state, fetchCount: state.fetchCount + 1 };
    default:
      return state;
  }
}

/**
 * Fetches budget forecast data from the REST API.
 *
 * The forecast endpoint requires a userID. We extract it from the
 * Firebase auth user's UID (which matches the server's auth.FromContext).
 */
export function useForecast() {
  const [state, dispatch] = useReducer(reducer, {
    data: null,
    loading: true,
    error: null,
    fetchCount: 0,
  });

  useEffect(() => {
    const controller = new AbortController();
    dispatch({ type: "fetch" });

    (async () => {
      try {
        const user = firebaseAuth?.currentUser;
        if (!user) {
          dispatch({ type: "error", message: "Not authenticated" });
          return;
        }

        const token = await user.getIdToken();
        const url = `${API_BASE_URL}/api/v1/users/${user.uid}/budget-forecast`;
        const res = await fetch(url, {
          signal: controller.signal,
          headers: { Authorization: `Bearer ${token}` },
        });

        if (!res.ok) {
          dispatch({ type: "error", message: `HTTP ${res.status}` });
          return;
        }

        const json = await res.json();
        const data: ForecastData = {
          burnRatePerHour: json.burn_rate_usd_per_hour ?? 0,
          projectedEodSpend: json.projected_eod_spend_usd ?? 0,
          willExceedBudget: json.will_exceed_budget ?? false,
          avgDailySpend: json.avg_daily_spend_usd ?? 0,
          estimatedExhaustionDate: json.estimated_exhaustion_date ?? "",
          daysUntilExhaustion: json.days_until_exhaustion ?? -1,
          spendHistory: json.spend_history ?? [],
        };

        dispatch({ type: "success", data });
      } catch (err: unknown) {
        if (err instanceof DOMException && err.name === "AbortError") return;
        dispatch({
          type: "error",
          message: err instanceof Error ? err.message : "Unknown error",
        });
      }
    })();

    return () => controller.abort();
  }, [state.fetchCount]);

  // Auto-refresh every 5 minutes (forecast data is cached server-side for 5 min).
  useEffect(() => {
    const interval = setInterval(() => {
      dispatch({ type: "refresh" });
    }, 5 * 60_000);
    return () => clearInterval(interval);
  }, []);

  const refresh = useCallback(() => dispatch({ type: "refresh" }), []);

  return {
    forecast: state.data,
    loading: state.loading,
    error: state.error,
    refresh,
  };
}

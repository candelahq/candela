"use client";

import { useCallback, useEffect, useReducer } from "react";
import { dashboardClient } from "@/lib/api";

export interface DashboardSummary {
  totalTraces: number;
  totalSpans: number;
  totalLlmCalls: number;
  totalInputTokens: number;
  totalOutputTokens: number;
  totalCostUsd: number;
  avgLatencyMs: number;
  errorRate: number;
}

type State = {
  summary: DashboardSummary | null;
  loading: boolean;
  error: string | null;
  fetchCount: number;
};

type Action =
  | { type: "fetch" }
  | { type: "success"; summary: DashboardSummary }
  | { type: "error"; message: string }
  | { type: "refresh" };

function reducer(state: State, action: Action): State {
  switch (action.type) {
    case "fetch":
      return { ...state, loading: true, error: null };
    case "success":
      return { ...state, loading: false, summary: action.summary };
    case "error":
      return { ...state, loading: false, error: action.message };
    case "refresh":
      return { ...state, fetchCount: state.fetchCount + 1 };
  }
}

/**
 * Hook for fetching the dashboard usage summary.
 * Encapsulates the GetUsageSummary RPC.
 */
export function useDashboard() {
  const [state, dispatch] = useReducer(reducer, {
    summary: null,
    loading: true,
    error: null,
    fetchCount: 0,
  });

  useEffect(() => {
    let cancelled = false;

    dashboardClient
      .getUsageSummary({})
      .then((res) => {
        if (cancelled) return;
        dispatch({
          type: "success",
          summary: {
            totalTraces: Number(res.totalTraces),
            totalSpans: Number(res.totalSpans),
            totalLlmCalls: Number(res.totalLlmCalls),
            totalInputTokens: Number(res.totalInputTokens),
            totalOutputTokens: Number(res.totalOutputTokens),
            totalCostUsd: res.totalCostUsd,
            avgLatencyMs: res.avgLatencyMs,
            errorRate: res.errorRate,
          },
        });
      })
      .catch((err) => {
        if (!cancelled) dispatch({ type: "error", message: err.message });
      });

    return () => { cancelled = true; };
  }, [state.fetchCount]);

  const refresh = useCallback(() => dispatch({ type: "refresh" }), []);

  return { summary: state.summary, loading: state.loading, error: state.error, refresh };
}

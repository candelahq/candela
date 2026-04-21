"use client";

import { useCallback, useEffect, useReducer } from "react";
import { dashboardClient } from "@/lib/api";
import { DEFAULT_PROJECT_ID } from "@/lib/constants";
import { timestampFromDate } from "@bufbuild/protobuf/wkt";
import { TimeRange } from "./useDashboard";

export interface UserUsageData {
  totalCalls: number;
  totalInputTokens: number;
  totalOutputTokens: number;
  totalCostUsd: number;
  avgLatencyMs: number;
  models: Array<{
    model: string;
    provider: string;
    callCount: number;
    costUsd: number;
  }>;
  budget: {
    limitUsd: number;
    spentUsd: number;
    remainingUsd: number;
    percentUsed: number;
    periodType: string;
  } | null;
}

type State = {
  data: UserUsageData | null;
  loading: boolean;
  error: string | null;
  timeRange: TimeRange;
  fetchCount: number;
};

type Action =
  | { type: "fetch" }
  | { type: "success"; data: UserUsageData }
  | { type: "error"; message: string }
  | { type: "refresh" }
  | { type: "setTimeRange"; range: TimeRange };

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
    case "setTimeRange":
      return { ...state, timeRange: action.range, fetchCount: state.fetchCount + 1 };
  }
}

function timeRangeToMs(range: TimeRange): number {
  switch (range) {
    case "24h": return 24 * 60 * 60 * 1000;
    case "7d": return 7 * 24 * 60 * 60 * 1000;
    case "30d": return 30 * 24 * 60 * 60 * 1000;
  }
}

export function useUsage() {
  const [state, dispatch] = useReducer(reducer, {
    data: null,
    loading: true,
    error: null,
    timeRange: "7d",
    fetchCount: 0,
  });

  useEffect(() => {
    let cancelled = false;
    dispatch({ type: "fetch" });

    const now = new Date();
    const start = new Date(now.getTime() - timeRangeToMs(state.timeRange));

    dashboardClient.getMyUsage({
      projectId: DEFAULT_PROJECT_ID,
      timeRange: {
        start: timestampFromDate(start),
        end: timestampFromDate(now),
      },
    })
      .then((res) => {
        if (cancelled) return;

        const limit = res.budget?.limitUsd || 0;
        const spent = res.budget?.spentUsd || 0;

        dispatch({
          type: "success",
          data: {
            totalCalls: Number(res.totalCalls),
            totalInputTokens: Number(res.totalInputTokens),
            totalOutputTokens: Number(res.totalOutputTokens),
            totalCostUsd: res.totalCostUsd,
            avgLatencyMs: res.avgLatencyMs,
            models: res.models.map((m) => ({
              model: m.model,
              provider: m.provider,
              callCount: Number(m.callCount),
              costUsd: m.costUsd,
            })),
            budget: res.budget ? {
              limitUsd: limit,
              spentUsd: spent,
              remainingUsd: res.totalRemainingUsd,
              percentUsed: limit > 0 ? (spent / limit) * 100 : 0,
              periodType: {
                0: "unspecified",
                1: "daily",
              }[res.budget.periodType] || "daily",
            } : null,
          }
        });
      })
      .catch((err) => {
        if (!cancelled) dispatch({ type: "error", message: err.message });
      });

    return () => { cancelled = true; };
  }, [state.fetchCount, state.timeRange]);

  const refresh = useCallback(() => dispatch({ type: "refresh" }), []);
  const setTimeRange = useCallback(
    (range: TimeRange) => dispatch({ type: "setTimeRange", range }),
    []
  );

  return {
    data: state.data,
    loading: state.loading,
    error: state.error,
    timeRange: state.timeRange,
    setTimeRange,
    refresh,
  };
}

"use client";

import { useCallback, useEffect, useReducer } from "react";
import { dashboardClient } from "@/lib/api";
import { DEFAULT_PROJECT_ID } from "@/lib/constants";
import { timestampFromDate } from "@bufbuild/protobuf/wkt";
import type { DataPoint } from "@/components/chart";
import {
  type TimeRange,
  timeRangeToMs,
  toDataPoints,
} from "@/lib/timeUtils";
import { mapModelUsage, type ModelUsageRow } from "@/hooks/useDashboard";

export interface CostSummary {
  totalCostUsd: number;
  totalInputTokens: number;
  totalOutputTokens: number;
  totalTraces: number;
  costOverTime: DataPoint[];
  tokensOverTime: DataPoint[];
}

export type ModelBreakdown = ModelUsageRow;

type State = {
  summary: CostSummary | null;
  models: ModelBreakdown[];
  loading: boolean;
  error: string | null;
  timeRange: TimeRange;
  fetchCount: number;
};

type Action =
  | { type: "fetch" }
  | { type: "success"; summary: CostSummary; models: ModelBreakdown[] }
  | { type: "error"; message: string }
  | { type: "refresh" }
  | { type: "setTimeRange"; range: TimeRange };

function reducer(state: State, action: Action): State {
  switch (action.type) {
    case "fetch":
      return { ...state, loading: true, error: null };
    case "success":
      return { ...state, loading: false, summary: action.summary, models: action.models };
    case "error":
      return { ...state, loading: false, error: action.message };
    case "refresh":
      return { ...state, fetchCount: state.fetchCount + 1 };
    case "setTimeRange":
      if (state.timeRange === action.range) return state;
      return { ...state, timeRange: action.range };
  }
}

/**
 * Hook for fetching cost data — summary, time series, and model breakdown.
 *
 * NOTE on RPC design (#614):
 * While GetDashboardData returns overlapping summary and model data, useDashboard
 * also queries ListTraces (5 recent traces) and GetJobLeaderboard.
 * To keep the /costs page lean and avoid issuing unnecessary trace and job queries,
 * useCosts issues targeted GetUsageSummary and GetModelBreakdown RPCs in parallel,
 * while sharing data transformation utilities (toDataPoints, mapModelUsage) with useDashboard.
 */
export function useCosts() {
  const [state, dispatch] = useReducer(reducer, {
    summary: null,
    models: [],
    loading: true,
    error: null,
    timeRange: "7d",
    fetchCount: 0,
  });

  useEffect(() => {
    const controller = new AbortController();
    const signal = controller.signal;
    dispatch({ type: "fetch" });

    const now = new Date();
    const start = new Date(now.getTime() - timeRangeToMs(state.timeRange));

    const timeRange = {
      start: timestampFromDate(start),
      end: timestampFromDate(now),
    };

    const summaryPromise = dashboardClient.getUsageSummary({
      projectId: DEFAULT_PROJECT_ID, // FIXME: Hardcoded - see constants.ts for evolution plan
      timeRange
    }, { signal });
    const modelsPromise = dashboardClient.getModelBreakdown({
      projectId: DEFAULT_PROJECT_ID, // FIXME: Hardcoded - see constants.ts for evolution plan
      timeRange
    }, { signal });

    Promise.all([summaryPromise, modelsPromise])
      .then(([summaryRes, modelsRes]) => {
        if (signal.aborted) return;
        dispatch({
          type: "success",
          summary: {
            totalCostUsd: summaryRes.totalCostUsd,
            totalInputTokens: Number(summaryRes.totalInputTokens),
            totalOutputTokens: Number(summaryRes.totalOutputTokens),
            totalTraces: Number(summaryRes.totalTraces),
            costOverTime: toDataPoints(summaryRes.costOverTime, state.timeRange),
            tokensOverTime: toDataPoints(summaryRes.tokensOverTime, state.timeRange),
          },
          models: (modelsRes.models || []).map(mapModelUsage),
        });
      })
      .catch((err) => {
        if (!signal.aborted) dispatch({ type: "error", message: err.message });
      });

    return () => controller.abort();
  }, [state.fetchCount, state.timeRange]);

  const refresh = useCallback(() => dispatch({ type: "refresh" }), []);
  const setTimeRange = useCallback(
    (range: TimeRange) => dispatch({ type: "setTimeRange", range }),
    []
  );

  return {
    summary: state.summary,
    models: state.models,
    loading: state.loading,
    error: state.error,
    timeRange: state.timeRange,
    setTimeRange,
    refresh,
  };
}

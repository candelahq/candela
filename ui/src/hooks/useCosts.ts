"use client";

import { useCallback, useEffect, useReducer } from "react";
import { dashboardClient } from "@/lib/api";
import { DEFAULT_PROJECT_ID } from "@/lib/constants";
import { timestampFromDate } from "@bufbuild/protobuf/wkt";
import type { DataPoint } from "@/components/chart";
import type { TimeRange } from "@/hooks/useDashboard";

export interface CostSummary {
  totalCostUsd: number;
  totalInputTokens: number;
  totalOutputTokens: number;
  totalTraces: number;
  costOverTime: DataPoint[];
  tokensOverTime: DataPoint[];
}

export interface ModelBreakdown {
  model: string;
  provider: string;
  callCount: number;
  inputTokens: number;
  outputTokens: number;
  costUsd: number;
  avgLatencyMs: number;
}

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

function formatTimeLabel(ts: string, range: TimeRange): string {
  const d = new Date(ts);
  if (range === "24h") {
    return d.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" });
  }
  return d.toLocaleDateString([], { month: "short", day: "numeric" });
}

/**
 * Hook for fetching cost data — summary, time series, and model breakdown.
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
    let cancelled = false;
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
    });
    const modelsPromise = dashboardClient.getModelBreakdown({
      projectId: DEFAULT_PROJECT_ID, // FIXME: Hardcoded - see constants.ts for evolution plan
      timeRange
    });

    Promise.all([summaryPromise, modelsPromise])
      .then(([summaryRes, modelsRes]) => {
        if (cancelled) return;
        dispatch({
          type: "success",
          summary: {
            totalCostUsd: summaryRes.totalCostUsd,
            totalInputTokens: Number(summaryRes.totalInputTokens),
            totalOutputTokens: Number(summaryRes.totalOutputTokens),
            totalTraces: Number(summaryRes.totalTraces),
            costOverTime: (summaryRes.costOverTime || []).map((p) => ({
              label: formatTimeLabel(p.timestamp, state.timeRange),
              value: p.value,
            })),
            tokensOverTime: (summaryRes.tokensOverTime || []).map((p) => ({
              label: formatTimeLabel(p.timestamp, state.timeRange),
              value: p.value,
            })),
          },
          models: (modelsRes.models || []).map((m) => ({
            model: m.model,
            provider: m.provider,
            callCount: Number(m.callCount),
            inputTokens: Number(m.inputTokens),
            outputTokens: Number(m.outputTokens),
            costUsd: m.costUsd,
            avgLatencyMs: m.avgLatencyMs,
          })),
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
    summary: state.summary,
    models: state.models,
    loading: state.loading,
    error: state.error,
    timeRange: state.timeRange,
    setTimeRange,
    refresh,
  };
}

"use client";

import { useCallback, useEffect, useReducer } from "react";
import { dashboardClient, traceClient } from "@/lib/api";
import { timestampFromDate } from "@bufbuild/protobuf/wkt";
import type { DataPoint } from "@/components/chart";

export interface DashboardSummary {
  totalTraces: number;
  totalSpans: number;
  totalLlmCalls: number;
  totalInputTokens: number;
  totalOutputTokens: number;
  totalCostUsd: number;
  avgLatencyMs: number;
  errorRate: number;
  // Time series
  tracesOverTime: DataPoint[];
  costOverTime: DataPoint[];
  tokensOverTime: DataPoint[];
}

export interface RecentTrace {
  traceId: string;
  rootSpanName: string;
  primaryModel: string;
  spanCount: number;
  totalTokens: number;
  totalCostUsd: number;
  durationMs: number;
  status: number;
  startTime: string;
}

export type TimeRange = "24h" | "7d" | "30d";

type State = {
  summary: DashboardSummary | null;
  recentTraces: RecentTrace[];
  loading: boolean;
  error: string | null;
  timeRange: TimeRange;
  fetchCount: number;
};

type Action =
  | { type: "fetch" }
  | { type: "success"; summary: DashboardSummary; recentTraces: RecentTrace[] }
  | { type: "error"; message: string }
  | { type: "refresh" }
  | { type: "setTimeRange"; range: TimeRange };

function reducer(state: State, action: Action): State {
  switch (action.type) {
    case "fetch":
      return { ...state, loading: true, error: null };
    case "success":
      return { ...state, loading: false, summary: action.summary, recentTraces: action.recentTraces };
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

function toDataPoints(pts: Array<{ timestamp: string; value: number }> | undefined, range: TimeRange): DataPoint[] {
  return (pts || []).map((p) => ({
    label: formatTimeLabel(p.timestamp, range),
    value: p.value,
  }));
}

/**
 * Hook for fetching the dashboard usage summary with time-series data
 * and recent traces. Supports time range filtering.
 */
export function useDashboard() {
  const [state, dispatch] = useReducer(reducer, {
    summary: null,
    recentTraces: [],
    loading: true,
    error: null,
    timeRange: "24h",
    fetchCount: 0,
  });

  useEffect(() => {
    let cancelled = false;
    dispatch({ type: "fetch" });

    const now = new Date();
    const start = new Date(now.getTime() - timeRangeToMs(state.timeRange));

    const summaryPromise = dashboardClient.getUsageSummary({
      timeRange: {
        start: timestampFromDate(start),
        end: timestampFromDate(now),
      },
    });

    const tracesPromise = traceClient.listTraces({
      orderBy: "start_time",
      descending: true,
      pagination: { pageSize: 5 },
    });

    Promise.all([summaryPromise, tracesPromise])
      .then(([res, tracesRes]) => {
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
            tracesOverTime: toDataPoints(res.tracesOverTime, state.timeRange),
            costOverTime: toDataPoints(res.costOverTime, state.timeRange),
            tokensOverTime: toDataPoints(res.tokensOverTime, state.timeRange),
          },
          recentTraces: (tracesRes.traces || []).map((t) => {
            const durSeconds = Number(t.duration?.seconds ?? 0);
            const durNanos = Number(t.duration?.nanos ?? 0);
            return {
              traceId: t.traceId,
              rootSpanName: t.rootSpanName || "unknown",
              primaryModel: t.primaryModel || "—",
              spanCount: t.spanCount || 0,
              totalTokens: Number(t.totalTokens) || 0,
              totalCostUsd: t.totalCostUsd || 0,
              durationMs: durSeconds * 1000 + durNanos / 1e6,
              status: t.status,
              startTime: t.startTime
                ? new Date(
                    Number(t.startTime.seconds) * 1000 +
                      Math.floor(Number(t.startTime.nanos) / 1e6)
                  ).toLocaleString()
                : "—",
            };
          }),
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
    recentTraces: state.recentTraces,
    loading: state.loading,
    error: state.error,
    timeRange: state.timeRange,
    setTimeRange,
    refresh,
  };
}

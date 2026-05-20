"use client";

import { useCallback, useEffect, useReducer } from "react";
import { dashboardClient, traceClient } from "@/lib/api";
import { DEFAULT_PROJECT_ID } from "@/lib/constants";
import { timestampFromDate } from "@bufbuild/protobuf/wkt";
import type { DataPoint } from "@/components/chart";
import type { GetJobLeaderboardResponse, JobUsage, ModelUsage } from "@/gen/candela/v1/dashboard_service_pb";
import type { UserBudget, BudgetGrant } from "@/gen/candela/types/user_pb";

// ──────────────────────────────────────────
// Types
// ──────────────────────────────────────────

export interface ModelUsageRow {
  model: string;
  provider: string;
  callCount: number;
  inputTokens: number;
  outputTokens: number;
  costUsd: number;
  avgLatencyMs: number;
  cacheReadTokens: number;
  cacheCreationTokens: number;
}

export interface BudgetContext {
  budget: UserBudget | null;
  totalRemainingUsd: number;
  activeGrants: BudgetGrant[];
}

export interface DashboardSummary {
  totalTraces: number;
  totalSpans: number;
  totalLlmCalls: number;
  totalInputTokens: number;
  totalOutputTokens: number;
  totalCostUsd: number;
  avgLatencyMs: number;
  errorRate: number;
  totalCacheReadTokens: number;
  totalCacheCreationTokens: number;
  // Time series
  tracesOverTime: DataPoint[];
  costOverTime: DataPoint[];
  tokensOverTime: DataPoint[];
  jobLeaderboard: Array<{
    jobId: string;
    callCount: number;
    totalTokens: number;
    costUsd: number;
  }>;
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
  models: ModelUsageRow[];
  budgetContext: BudgetContext | null;
  loading: boolean;
  error: string | null;
  timeRange: TimeRange;
  fetchCount: number;
};

type Action =
  | { type: "fetch" }
  | {
      type: "success";
      summary: DashboardSummary;
      recentTraces: RecentTrace[];
      models: ModelUsageRow[];
      budgetContext: BudgetContext | null;
    }
  | { type: "error"; message: string }
  | { type: "refresh" }
  | { type: "setTimeRange"; range: TimeRange };

function reducer(state: State, action: Action): State {
  switch (action.type) {
    case "fetch":
      return { ...state, loading: true, error: null };
    case "success":
      return {
        ...state,
        loading: false,
        summary: action.summary,
        recentTraces: action.recentTraces,
        models: action.models,
        budgetContext: action.budgetContext,
      };
    case "error":
      return { ...state, loading: false, error: action.message };
    case "refresh":
      return { ...state, fetchCount: state.fetchCount + 1 };
    case "setTimeRange":
      return { ...state, timeRange: action.range, fetchCount: state.fetchCount + 1 };
  }
}

// ──────────────────────────────────────────
// Helpers
// ──────────────────────────────────────────

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

function mapModelUsage(m: ModelUsage): ModelUsageRow {
  return {
    model: m.model,
    provider: m.provider,
    callCount: Number(m.callCount),
    inputTokens: Number(m.inputTokens),
    outputTokens: Number(m.outputTokens),
    costUsd: m.costUsd,
    avgLatencyMs: m.avgLatencyMs,
    cacheReadTokens: Number(m.cacheReadTokens),
    cacheCreationTokens: Number(m.cacheCreationTokens),
  };
}

// ──────────────────────────────────────────
// Hook
// ──────────────────────────────────────────

/**
 * Hook for fetching the consolidated dashboard data via GetDashboardData RPC.
 *
 * This replaces the previous fan-out of GetUsageSummary + GetModelBreakdown + GetMyUsage
 * with a single round-trip. When `includeBudget` is true (the default when authenticated),
 * the response includes per-user budget/grant context.
 *
 * Recent traces are still fetched separately via ListTraces.
 */
export function useDashboard(options?: { includeBudget?: boolean }) {
  const includeBudget = options?.includeBudget ?? true;

  const [state, dispatch] = useReducer(reducer, {
    summary: null,
    recentTraces: [],
    models: [],
    budgetContext: null,
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
    const timeRange = {
      start: timestampFromDate(start),
      end: timestampFromDate(now),
    };

    // ── Primary RPC: GetDashboardData (replaces GetUsageSummary + GetModelBreakdown) ──
    const dashboardPromise = dashboardClient.getDashboardData({
      projectId: DEFAULT_PROJECT_ID,
      timeRange,
      includeBudget,
    });

    // ── Recent traces (still separate — no equivalent in GetDashboardData) ──
    const tracesPromise = traceClient.listTraces({
      projectId: DEFAULT_PROJECT_ID,
      orderBy: "start_time",
      descending: true,
      pagination: { pageSize: 5 },
    });

    // ── Job leaderboard (still separate) ──
    const jobLeaderboardPromise = dashboardClient.getJobLeaderboard({
      projectId: DEFAULT_PROJECT_ID,
      timeRange,
      limit: 10,
    }).catch(() => ({ jobs: [] })); // Degrade gracefully if job_id column missing

    Promise.all([dashboardPromise, tracesPromise, jobLeaderboardPromise])
      .then(([dashRes, tracesRes, jobRes]) => {
        if (cancelled) return;

        const res = dashRes.summary;
        const models = (dashRes.models || []).map(mapModelUsage);

        // Map budget context from the consolidated response
        let budgetCtx: BudgetContext | null = null;
        if (dashRes.budgetContext) {
          budgetCtx = {
            budget: dashRes.budgetContext.budget ?? null,
            totalRemainingUsd: dashRes.budgetContext.totalRemainingUsd,
            activeGrants: dashRes.budgetContext.activeGrants,
          };
        }

        dispatch({
          type: "success",
          summary: {
            totalTraces: Number(res?.totalTraces ?? 0),
            totalSpans: Number(res?.totalSpans ?? 0),
            totalLlmCalls: Number(res?.totalLlmCalls ?? 0),
            totalInputTokens: Number(res?.totalInputTokens ?? 0),
            totalOutputTokens: Number(res?.totalOutputTokens ?? 0),
            totalCostUsd: res?.totalCostUsd ?? 0,
            avgLatencyMs: res?.avgLatencyMs ?? 0,
            errorRate: res?.errorRate ?? 0,
            totalCacheReadTokens: Number(res?.totalCacheReadTokens ?? 0),
            totalCacheCreationTokens: Number(res?.totalCacheCreationTokens ?? 0),
            tracesOverTime: toDataPoints(res?.tracesOverTime, state.timeRange),
            costOverTime: toDataPoints(res?.costOverTime, state.timeRange),
            tokensOverTime: toDataPoints(res?.tokensOverTime, state.timeRange),
            jobLeaderboard: (jobRes as Pick<GetJobLeaderboardResponse, "jobs">).jobs.map((j: JobUsage) => ({
              jobId: j.jobId,
              callCount: Number(j.callCount),
              totalTokens: Number(j.totalTokens),
              costUsd: j.costUsd,
            })),
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
          models,
          budgetContext: budgetCtx,
        });
      })
      .catch((err) => {
        if (!cancelled) dispatch({ type: "error", message: err.message });
      });

    return () => { cancelled = true; };
  }, [state.fetchCount, state.timeRange, includeBudget]);

  const refresh = useCallback(() => dispatch({ type: "refresh" }), []);
  const setTimeRange = useCallback(
    (range: TimeRange) => dispatch({ type: "setTimeRange", range }),
    []
  );

  return {
    summary: state.summary,
    recentTraces: state.recentTraces,
    models: state.models,
    budgetContext: state.budgetContext,
    loading: state.loading,
    error: state.error,
    timeRange: state.timeRange,
    setTimeRange,
    refresh,
  };
}

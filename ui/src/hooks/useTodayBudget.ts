"use client";

import { useCallback, useEffect, useReducer } from "react";
import { dashboardClient, userClient } from "@/lib/api";
import { DEFAULT_PROJECT_ID } from "@/lib/constants";
import { timestampDate, timestampFromDate } from "@bufbuild/protobuf/wkt";

export interface TodayModelUsage {
  model: string;
  provider: string;
  callCount: number;
  inputTokens: number;
  outputTokens: number;
  costUsd: number;
  avgLatencyMs: number;
}

export interface TodayGrant {
  id: string;
  amountUsd: number;
  spentUsd: number;
  remainingUsd: number;
  percentUsed: number;
  reason: string;
  grantedBy: string;
  expiresAt: Date | null;
}

export interface TodayBudgetData {
  totalCalls: number;
  totalInputTokens: number;
  totalOutputTokens: number;
  totalCostUsd: number;
  avgLatencyMs: number;
  models: TodayModelUsage[];
  budget: {
    limitUsd: number;
    spentUsd: number;
    remainingUsd: number;
    percentUsed: number;
    periodType: string;
  } | null;
  grants: TodayGrant[];
  /** When this data was last fetched */
  fetchedAt: Date;
  /** ISO 8601 timestamp when the budget period resets (midnight UTC). */
  periodResetsAt: string | null;
}

type State = {
  data: TodayBudgetData | null;
  loading: boolean;
  error: string | null;
  fetchCount: number;
};

type Action =
  | { type: "fetch" }
  | { type: "success"; data: TodayBudgetData }
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
  }
}

/** Returns the start of today in UTC (midnight) to match budget reset boundary. */
function startOfTodayUTC(): Date {
  const now = new Date();
  return new Date(Date.UTC(now.getUTCFullYear(), now.getUTCMonth(), now.getUTCDate()));
}

/**
 * Fetches the current user's usage AND budget for TODAY, scoped to the daily
 * budget period (midnight UTC → now).
 *
 * Calls two RPCs in parallel:
 *   1. DashboardService.GetMyUsage  → token counts, cost, model breakdown (~800ms)
 *   2. UserService.GetMyBudget      → budget limit/spent, grants (~80ms)
 *
 * Auto-refreshes every 60 seconds to keep the view live.
 */
export function useTodayBudget() {
  const [state, dispatch] = useReducer(reducer, {
    data: null,
    loading: true,
    error: null,
    fetchCount: 0,
  });

  useEffect(() => {
    let cancelled = false;
    dispatch({ type: "fetch" });

    const start = startOfTodayUTC();
    const now = new Date();

    // Fire both RPCs in parallel — usage data from BigQuery, budget from Firestore.
    const usagePromise = dashboardClient.getMyUsage({
      projectId: DEFAULT_PROJECT_ID,
      timeRange: {
        start: timestampFromDate(start),
        end: timestampFromDate(now),
      },
    });

    const budgetPromise = userClient.getMyBudget({}).catch(() => null);

    Promise.all([usagePromise, budgetPromise])
      .then(([usageRes, budgetRes]) => {
        if (cancelled) return;

        // ── Budget from UserService.GetMyBudget (Firestore) ───────────────
        const budgetProto = budgetRes?.budget;
        const limit = budgetProto?.limitUsd ?? 0;
        const spent = budgetProto?.spentUsd ?? 0;

        // Map active grants from the budget response.
        const grants: TodayGrant[] = (budgetRes?.activeGrants ?? []).map((g) => {
          const amt = g.amountUsd;
          const sp = g.spentUsd;
          return {
            id: g.id,
            amountUsd: amt,
            spentUsd: sp,
            remainingUsd: amt - sp,
            percentUsed: amt > 0 ? (sp / amt) * 100 : 0,
            reason: g.reason,
            grantedBy: g.grantedBy,
            expiresAt: g.expiresAt ? timestampDate(g.expiresAt) : null,
          };
        });

        dispatch({
          type: "success",
          data: {
            // ── Usage from DashboardService.GetMyUsage (BigQuery) ─────────
            totalCalls: Number(usageRes.totalCalls),
            totalInputTokens: Number(usageRes.totalInputTokens),
            totalOutputTokens: Number(usageRes.totalOutputTokens),
            totalCostUsd: usageRes.totalCostUsd,
            avgLatencyMs: usageRes.avgLatencyMs,
            models: usageRes.models.map((m) => ({
              model: m.model,
              provider: m.provider,
              callCount: Number(m.callCount),
              inputTokens: Number(m.inputTokens),
              outputTokens: Number(m.outputTokens),
              costUsd: m.costUsd,
              avgLatencyMs: m.avgLatencyMs,
            })),
            // ── Budget from UserService.GetMyBudget ──────────────────────
            budget: budgetProto ? {
              limitUsd: limit,
              spentUsd: spent,
              remainingUsd: budgetRes?.budgetRemainingUsd ?? (limit - spent),
              percentUsed: limit > 0 ? (spent / limit) * 100 : 0,
              periodType: {
                0: "unspecified",
                1: "daily",
              }[budgetProto.periodType] || "daily",
            } : null,
            grants,
            fetchedAt: new Date(),
            periodResetsAt: budgetRes?.periodResetsAt ?? null,
          },
        });
      })
      .catch((err) => {
        if (!cancelled) dispatch({ type: "error", message: err.message });
      });

    return () => { cancelled = true; };
  }, [state.fetchCount]);

  // Auto-refresh every 60 seconds.
  useEffect(() => {
    const interval = setInterval(() => {
      dispatch({ type: "refresh" });
    }, 60_000);
    return () => clearInterval(interval);
  }, []);

  const refresh = useCallback(() => dispatch({ type: "refresh" }), []);

  return {
    data: state.data,
    loading: state.loading,
    error: state.error,
    refresh,
  };
}

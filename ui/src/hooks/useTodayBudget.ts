"use client";

import { useCallback, useEffect, useReducer } from "react";
import { dashboardClient, userClient } from "@/lib/api";
import { DEFAULT_PROJECT_ID } from "@/lib/constants";
import { timestampDate, timestampFromDate } from "@bufbuild/protobuf/wkt";
import { budgetPeriodToLabel } from "@/hooks/useUsage";
import type { ForecastData } from "@/hooks/useForecast";

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
  forecast: ForecastData | null;
  /** When this data was last fetched */
  fetchedAt: Date;
  /** ISO 8601 timestamp when the budget period resets (midnight UTC). */
  periodResetsAt: string | null;
}

export const BASE_REFRESH_INTERVAL_MS = 60_000;
export const MAX_CONSECUTIVE_ERRORS = 5;
export const MAX_BACKOFF_MS = 300_000;

type State = {
  data: TodayBudgetData | null;
  loading: boolean;
  error: string | null;
  fetchCount: number;
  consecutiveErrors: number;
};

type Action =
  | { type: "fetch" }
  | { type: "success"; data: TodayBudgetData; hasPartialError?: boolean }
  | { type: "error"; message: string }
  | { type: "refresh" };

function reducer(state: State, action: Action): State {
  switch (action.type) {
    case "fetch":
      return { ...state, loading: true, error: null };
    case "success":
      return {
        ...state,
        loading: false,
        data: action.data,
        consecutiveErrors: action.hasPartialError ? state.consecutiveErrors + 1 : 0,
      };
    case "error":
      return {
        ...state,
        loading: false,
        error: action.message,
        consecutiveErrors: state.consecutiveErrors + 1,
      };
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
export interface UseTodayBudgetOptions {
  baseIntervalMs?: number;
  maxConsecutiveErrors?: number;
  maxBackoffMs?: number;
}

export function useTodayBudget(options?: UseTodayBudgetOptions) {
  const baseIntervalMs = options?.baseIntervalMs ?? BASE_REFRESH_INTERVAL_MS;
  const maxConsecutiveErrors = options?.maxConsecutiveErrors ?? MAX_CONSECUTIVE_ERRORS;
  const maxBackoffMs = options?.maxBackoffMs ?? MAX_BACKOFF_MS;

  const [state, dispatch] = useReducer(reducer, {
    data: null,
    loading: true,
    error: null,
    fetchCount: 0,
    consecutiveErrors: 0,
  });

  useEffect(() => {
    const controller = new AbortController();
    const signal = controller.signal;
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
    }, { signal });

    let budgetError = false;
    const budgetPromise = userClient.getMyBudget({}, { signal }).catch(() => {
      budgetError = true;
      return null;
    });

    Promise.all([usagePromise, budgetPromise])
      .then(([usageRes, budgetRes]) => {
        if (signal.aborted) return;

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

        // Map budget forecast from UserService.GetMyBudget (#794)
        const forecastProto = budgetRes?.forecast;
        const forecastData: ForecastData | null = forecastProto ? {
          burnRatePerHour: forecastProto.burnRateUsdPerHour,
          projectedEodSpend: forecastProto.projectedEodSpendUsd,
          willExceedBudget: forecastProto.willExceedBudget,
          avgDailySpend: forecastProto.avgDailySpendUsd,
          estimatedExhaustionDate: forecastProto.estimatedExhaustionDate,
          daysUntilExhaustion: forecastProto.daysUntilExhaustion,
          spendHistory: (forecastProto.spendHistory ?? []).map((h) => ({
            date: h.date,
            spend_usd: h.spendUsd,
            token_count: Number(h.tokenCount),
          })),
        } : null;

        dispatch({
          type: "success",
          hasPartialError: budgetError,
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
              periodType: budgetPeriodToLabel(budgetProto.periodType),
            } : null,
            grants,
            forecast: forecastData,
            fetchedAt: new Date(),
            periodResetsAt: budgetRes?.periodResetsAt ?? null,
          },
        });
      })
      .catch((err) => {
        if (!signal.aborted) dispatch({ type: "error", message: err.message });
      });

    return () => controller.abort();
  }, [state.fetchCount]);

  // Auto-refresh with exponential backoff on errors; stops after maxConsecutiveErrors (#598)
  useEffect(() => {
    if (state.loading) return;
    if (state.consecutiveErrors >= maxConsecutiveErrors) return;

    let delay = baseIntervalMs;
    if (state.consecutiveErrors > 0) {
      delay = Math.min(
        baseIntervalMs * Math.pow(2, state.consecutiveErrors),
        maxBackoffMs
      );
    }

    const timer = setTimeout(() => {
      dispatch({ type: "refresh" });
    }, delay);

    return () => clearTimeout(timer);
  }, [
    state.loading,
    state.consecutiveErrors,
    state.fetchCount,
    baseIntervalMs,
    maxConsecutiveErrors,
    maxBackoffMs,
  ]);

  const refresh = useCallback(() => dispatch({ type: "refresh" }), []);

  return {
    data: state.data,
    loading: state.loading,
    error: state.error,
    refresh,
    consecutiveErrors: state.consecutiveErrors,
  };
}

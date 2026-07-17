"use client";

import { useCallback, useEffect, useReducer } from "react";
import { API_BASE_URL } from "@/lib/constants";
import { firebaseAuth } from "@/lib/firebase";
import type { TimeRange } from "./useDashboard";

function timeRangeToMs(range: TimeRange): number {
  switch (range) {
    case "24h": return 24 * 60 * 60 * 1000;
    case "7d": return 7 * 24 * 60 * 60 * 1000;
    case "30d": return 30 * 24 * 60 * 60 * 1000;
  }
}

export interface TenantUsageRow {
  tenantId: string;
  callCount: number;
  totalTokens: number;
  costUsd: number;
  avgLatencyMs: number;
  topModel: string;
}

export type SortField = "tenantId" | "callCount" | "totalTokens" | "costUsd" | "avgLatencyMs" | "topModel";

type State = {
  tenants: TenantUsageRow[];
  loading: boolean;
  error: string | null;
  timeRange: TimeRange;
  fetchCount: number;
  sortField: SortField;
  sortAsc: boolean;
};

type Action =
  | { type: "fetch" }
  | { type: "success"; tenants: TenantUsageRow[] }
  | { type: "error"; message: string }
  | { type: "refresh" }
  | { type: "setTimeRange"; range: TimeRange }
  | { type: "setSort"; field: SortField };

function reducer(state: State, action: Action): State {
  switch (action.type) {
    case "fetch":
      return { ...state, loading: true, error: null };
    case "success":
      return { ...state, loading: false, tenants: action.tenants };
    case "error":
      return { ...state, loading: false, error: action.message };
    case "refresh":
      return { ...state, fetchCount: state.fetchCount + 1 };
    case "setTimeRange":
      return { ...state, timeRange: action.range, fetchCount: state.fetchCount + 1 };
    case "setSort":
      return {
        ...state,
        sortField: action.field,
        sortAsc: state.sortField === action.field ? !state.sortAsc : false,
      };
  }
}

export function useTenantLeaderboard() {
  const [state, dispatch] = useReducer(reducer, {
    tenants: [],
    loading: true,
    error: null,
    timeRange: "7d",
    fetchCount: 0,
    sortField: "costUsd",
    sortAsc: false,
  });

  useEffect(() => {
    const controller = new AbortController();
    const signal = controller.signal;
    dispatch({ type: "fetch" });

    (async () => {
      try {
        const headers: Record<string, string> = {
          "Content-Type": "application/json",
        };
        const user = firebaseAuth?.currentUser;
        if (user) {
          const token = await user.getIdToken();
          headers["Authorization"] = `Bearer ${token}`;
        }

        const now = Date.now();
        const start = new Date(now - timeRangeToMs(state.timeRange)).toISOString();
        const end = new Date(now).toISOString();

        const res = await fetch(
          `${API_BASE_URL}/_local/api/leaderboard?limit=50&startTime=${encodeURIComponent(start)}&endTime=${encodeURIComponent(end)}`,
          { headers, signal },
        );

        if (!res.ok) {
          throw new Error(`HTTP ${res.status}: ${res.statusText}`);
        }

        const data = await res.json();
        if (signal.aborted) return;

        const tenants: TenantUsageRow[] = (data.tenants ?? []).map(
          (t: Record<string, unknown>) => ({
            tenantId: (t.tenant_id as string) || "unknown",
            callCount: Number(t.call_count ?? 0),
            totalTokens: Number(t.total_tokens ?? 0),
            costUsd: Number(t.cost_usd ?? 0),
            avgLatencyMs: Number(t.avg_latency_ms ?? 0),
            topModel: (t.top_model as string) || "—",
          })
        );

        dispatch({ type: "success", tenants });
      } catch (err) {
        if (!signal.aborted) {
          dispatch({
            type: "error",
            message: err instanceof Error ? err.message : "Failed to load tenant leaderboard",
          });
        }
      }
    })();

    return () => controller.abort();
  }, [state.fetchCount, state.timeRange]);

  // Client-side sorting
  const sorted = [...state.tenants].sort((a, b) => {
    const field = state.sortField;
    const av = a[field];
    const bv = b[field];
    if (typeof av === "string" && typeof bv === "string") {
      return state.sortAsc ? av.localeCompare(bv) : bv.localeCompare(av);
    }
    return state.sortAsc ? (av as number) - (bv as number) : (bv as number) - (av as number);
  });

  const refresh = useCallback(() => dispatch({ type: "refresh" }), []);
  const setTimeRange = useCallback(
    (range: TimeRange) => dispatch({ type: "setTimeRange", range }),
    []
  );
  const setSort = useCallback(
    (field: SortField) => dispatch({ type: "setSort", field }),
    []
  );

  return {
    tenants: state.tenants,
    sortedTenants: sorted,
    loading: state.loading,
    error: state.error,
    timeRange: state.timeRange,
    setTimeRange,
    sortField: state.sortField,
    sortAsc: state.sortAsc,
    setSort,
    refresh,
  };
}

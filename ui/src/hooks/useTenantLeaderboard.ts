"use client";

import { useCallback, useEffect, useReducer } from "react";
import { API_BASE_URL } from "@/lib/constants";
import { firebaseAuth } from "@/lib/firebase";
import type { TimeRange } from "./useDashboard";

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
    let cancelled = false;
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

        const res = await fetch(`${API_BASE_URL}/_local/api/leaderboard?limit=50`, {
          headers,
        });

        if (!res.ok) {
          throw new Error(`HTTP ${res.status}: ${res.statusText}`);
        }

        const data = await res.json();
        if (cancelled) return;

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
        if (!cancelled) {
          dispatch({
            type: "error",
            message: err instanceof Error ? err.message : "Failed to load tenant leaderboard",
          });
        }
      }
    })();

    return () => {
      cancelled = true;
    };
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
    tenants: sorted,
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

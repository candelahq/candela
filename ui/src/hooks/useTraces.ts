"use client";

import { useCallback, useReducer, useRef } from "react";
import { traceClient } from "@/lib/api";
import type { TraceSummaryRow, TraceFilters } from "@/types/traces";
import { DEFAULT_FILTERS } from "@/types/traces";

type State = {
  traces: TraceSummaryRow[];
  loading: boolean;
  error: string | null;
  filters: TraceFilters;
};

type Action =
  | { type: "fetch"; filters: TraceFilters }
  | { type: "success"; traces: TraceSummaryRow[] }
  | { type: "error"; message: string }
  | { type: "set_filters"; filters: TraceFilters }
  | { type: "clear_filters" };

function reducer(state: State, action: Action): State {
  switch (action.type) {
    case "fetch":
      return { ...state, loading: true, error: null, filters: action.filters };
    case "success":
      return { ...state, loading: false, traces: action.traces };
    case "error":
      return { ...state, loading: false, error: action.message };
    case "set_filters":
      return { ...state, filters: action.filters };
    case "clear_filters":
      return { ...state, loading: true, error: null, filters: DEFAULT_FILTERS };
  }
}

function mapTrace(t: {
  traceId: string;
  rootSpanName: string;
  primaryModel: string;
  primaryProvider: string;
  environment: string;
  duration?: { seconds: bigint; nanos: number };
  totalTokens: bigint;
  totalCostUsd: number;
  status: number;
  spanCount: number;
  llmCallCount: number;
  startTime?: { seconds: bigint; nanos: number };
}): TraceSummaryRow {
  const durSeconds = Number(t.duration?.seconds ?? 0);
  const durNanos = Number(t.duration?.nanos ?? 0);
  return {
    traceId: t.traceId,
    rootSpanName: t.rootSpanName || "unknown",
    primaryModel: t.primaryModel || "—",
    primaryProvider: t.primaryProvider || "—",
    environment: t.environment || "—",
    durationMs: durSeconds * 1000 + durNanos / 1e6,
    totalTokens: Number(t.totalTokens) || 0,
    totalCostUsd: t.totalCostUsd || 0,
    status: t.status,
    spanCount: t.spanCount || 0,
    llmCallCount: t.llmCallCount || 0,
    startTime: t.startTime
      ? new Date(
          Number(t.startTime.seconds) * 1000 +
            Math.floor(Number(t.startTime.nanos) / 1e6)
        ).toLocaleString()
      : "—",
  };
}

/**
 * Hook for fetching and filtering traces.
 * Encapsulates the ListTraces RPC, debounced search, and filter state.
 */
export function useTraces() {
  const [state, dispatch] = useReducer(reducer, {
    traces: [],
    loading: true,
    error: null,
    filters: DEFAULT_FILTERS,
  });
  const debounceRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  const fetchTraces = useCallback((f: TraceFilters) => {
    dispatch({ type: "fetch", filters: f });
    traceClient
      .listTraces({
        pagination: { pageSize: 100 },
        search: f.search,
        model: f.model,
        provider: f.provider,
        status: f.status === "ok" ? 1 : f.status === "error" ? 2 : 0,
        orderBy: f.orderBy,
        descending: f.descending,
      })
      .then((res) => {
        dispatch({
          type: "success",
          traces: (res.traces || []).map(mapTrace),
        });
      })
      .catch((err) => dispatch({ type: "error", message: err.message }));
  }, []);

  const updateFilters = useCallback(
    (patch: Partial<TraceFilters>) => {
      const next = { ...state.filters, ...patch };
      dispatch({ type: "set_filters", filters: next });

      const isSearch = "search" in patch;
      if (debounceRef.current) clearTimeout(debounceRef.current);

      if (isSearch) {
        debounceRef.current = setTimeout(() => fetchTraces(next), 300);
      } else {
        fetchTraces(next);
      }
    },
    [state.filters, fetchTraces]
  );

  const clearFilters = useCallback(() => {
    dispatch({ type: "clear_filters" });
    fetchTraces(DEFAULT_FILTERS);
  }, [fetchTraces]);

  const hasActiveFilters = !!(
    state.filters.search ||
    state.filters.model ||
    state.filters.provider ||
    state.filters.status
  );

  const refresh = useCallback(
    () => fetchTraces(state.filters),
    [state.filters, fetchTraces]
  );

  return {
    traces: state.traces,
    loading: state.loading,
    error: state.error,
    filters: state.filters,
    hasActiveFilters,
    updateFilters,
    clearFilters,
    refresh,
    fetchInitial: () => fetchTraces(state.filters),
  };
}

"use client";

import { useCallback, useEffect, useReducer, useRef } from "react";
import { traceClient } from "@/lib/api";
import { DEFAULT_PROJECT_ID } from "@/lib/constants";
import type { TraceSummaryRow, TraceFilters } from "@/types/traces";
import { DEFAULT_FILTERS } from "@/types/traces";
import { useScope } from "@/components/UserScopeProvider";
import { create } from "@bufbuild/protobuf";
import { TimestampSchema } from "@bufbuild/protobuf/wkt";

function makeTimeRange(range: string) {
  const now = new Date();
  const hours = range === "1h" ? 1 : range === "24h" ? 24 : range === "7d" ? 168 : 720;
  const start = new Date(now.getTime() - hours * 3600_000);
  return {
    start: create(TimestampSchema, { seconds: BigInt(Math.floor(start.getTime() / 1000)), nanos: 0 }),
    end: create(TimestampSchema, { seconds: BigInt(Math.floor(now.getTime() / 1000)), nanos: 0 }),
  };
}

type State = {
  traces: TraceSummaryRow[];
  loading: boolean;
  error: string | null;
  filters: TraceFilters;
  nextPageToken: string;
  currentPageToken: string;
  pageTokenHistory: string[];
};

type Action =
  | { type: "fetch"; filters: TraceFilters; resetPagination?: boolean }
  | { type: "success"; traces: TraceSummaryRow[]; nextPageToken: string }
  | { type: "error"; message: string }
  | { type: "set_filters"; filters: TraceFilters }
  | { type: "clear_filters" }
  | { type: "set_page_token"; direction: "next" | "prev"; token?: string };

function reducer(state: State, action: Action): State {
  switch (action.type) {
    case "fetch":
      if (action.resetPagination) {
        return { ...state, loading: true, error: null, filters: action.filters, currentPageToken: "", pageTokenHistory: [], nextPageToken: "" };
      }
      return { ...state, loading: true, error: null, filters: action.filters };
    case "success":
      return { ...state, loading: false, traces: action.traces, nextPageToken: action.nextPageToken };
    case "error":
      return { ...state, loading: false, error: action.message };
    case "set_filters":
      return { ...state, filters: action.filters, currentPageToken: "", pageTokenHistory: [], nextPageToken: "" };
    case "clear_filters":
      return { ...state, loading: true, error: null, filters: DEFAULT_FILTERS, currentPageToken: "", pageTokenHistory: [], nextPageToken: "" };
    case "set_page_token":
      if (action.direction === "next") {
        return {
          ...state,
          pageTokenHistory: [...state.pageTokenHistory, state.currentPageToken],
          currentPageToken: action.token || "",
        };
      } else {
        const prevHistory = [...state.pageTokenHistory];
        const prevToken = prevHistory.pop() || "";
        return {
          ...state,
          pageTokenHistory: prevHistory,
          currentPageToken: prevToken,
        };
      }
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
  tenantId?: string;
  jobId?: string;
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
    tenantId: t.tenantId,
    jobId: t.jobId,
  };
}

/**
 * Hook for fetching and filtering traces.
 * Encapsulates the ListTraces RPC, debounced search, and filter state.
 *
 * Scope-aware: In "personal" mode the backend already scopes by the
 * authenticated user's Firebase token.  We pass `include_budget=true`
 * as a hint header so the backend knows this is a personal-scope request.
 * Re-fetches automatically when the scope mode changes.
 */
export function useTraces() {
  const { isPersonalScope, mode } = useScope();

  const [state, dispatch] = useReducer(reducer, {
    traces: [],
    loading: true,
    error: null,
    filters: DEFAULT_FILTERS,
    nextPageToken: "",
    currentPageToken: "",
    pageTokenHistory: [],
  });
  const debounceRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  // Track the previous scope mode so we can detect changes
  const prevModeRef = useRef(mode);

  const fetchRef = useRef<AbortController | null>(null);

  const fetchTraces = useCallback((f: TraceFilters, resetPagination = false) => {
    fetchRef.current?.abort();
    const controller = new AbortController();
    fetchRef.current = controller;

    dispatch({ type: "fetch", filters: f, resetPagination });
    const pageToken = resetPagination ? "" : state.currentPageToken;

    // Build headers — the backend interprets the auth token + this hint
    // to decide whether to filter to the authenticated user's traces.
    const headers: Record<string, string> = {};
    if (f.jobId) headers["X-Candela-Job-Id"] = f.jobId;
    if (isPersonalScope) headers["X-Candela-Scope"] = "personal";

    traceClient
      .listTraces({
        projectId: DEFAULT_PROJECT_ID,
        pagination: { pageSize: 100, pageToken },
        search: f.search,
        model: f.model,
        provider: f.provider,
        status: f.status === "ok" ? 1 : f.status === "error" ? 2 : 0,
        orderBy: f.orderBy,
        descending: f.descending,
        timeRange: makeTimeRange(f.timeRange),
        environment: f.environment,
        traceGroup: f.traceGroup,
      }, {
        headers,
        signal: controller.signal,
      })
      .then((res) => {
        if (!controller.signal.aborted) {
          const nextToken = res.pagination?.nextPageToken ?? "";
          dispatch({
            type: "success",
            traces: (res.traces || []).map(mapTrace),
            nextPageToken: nextToken,
          });
        }
      })
      .catch((err) => {
        if (!controller.signal.aborted) {
          dispatch({ type: "error", message: err.message });
        }
      });
  }, [isPersonalScope, state.currentPageToken]);

  const updateFilters = useCallback(
    (patch: Partial<TraceFilters>) => {
      const next = { ...state.filters, ...patch };
      dispatch({ type: "set_filters", filters: next });

      const isSearch = "search" in patch;
      if (debounceRef.current) clearTimeout(debounceRef.current);

      if (isSearch) {
        debounceRef.current = setTimeout(() => fetchTraces(next, true), 300);
      } else {
        fetchTraces(next, true);
      }
    },
    [state.filters, fetchTraces]
  );

  const clearFilters = useCallback(() => {
    dispatch({ type: "clear_filters" });
    fetchTraces(DEFAULT_FILTERS, true);
  }, [fetchTraces]);

  const hasActiveFilters = !!(
    state.filters.search ||
    state.filters.model ||
    state.filters.provider ||
    state.filters.status ||
    state.filters.jobId ||
    state.filters.environment ||
    state.filters.traceGroup ||
    state.filters.timeRange !== DEFAULT_FILTERS.timeRange
  );

  const refresh = useCallback(
    () => fetchTraces(state.filters),
    [state.filters, fetchTraces]
  );

  // Re-fetch when scope mode changes
  useEffect(() => {
    if (prevModeRef.current !== mode) {
      prevModeRef.current = mode;
      fetchTraces(state.filters, true);
    }
  }, [mode, fetchTraces, state.filters]);

  // Only fetch on pagination token changes — filter-driven fetches
  // are handled imperatively by updateFilters/clearFilters.
  const prevTokenRef = useRef(state.currentPageToken);
  useEffect(() => {
    if (prevTokenRef.current !== state.currentPageToken) {
      prevTokenRef.current = state.currentPageToken;
      fetchTraces(state.filters);
    }
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, [state.currentPageToken]);

  const fetchNextPage = useCallback(() => {
    if (!state.nextPageToken) return;
    dispatch({ type: "set_page_token", direction: "next", token: state.nextPageToken });
  }, [state.nextPageToken]);

  const fetchPreviousPage = useCallback(() => {
    if (state.pageTokenHistory.length === 0) return;
    dispatch({ type: "set_page_token", direction: "prev" });
  }, [state.pageTokenHistory]);

  // Abort in-flight request on unmount only. Cleanup must NOT be in the
  // mode/filters effect — fetchTraces already updates fetchRef.current
  // before React runs the previous effect's cleanup, so that cleanup
  // would abort the *new* request instead of the old one.
  useEffect(() => {
    return () => {
      fetchRef.current?.abort();
      if (debounceRef.current) clearTimeout(debounceRef.current);
    };
  }, []);

  return {
    traces: state.traces,
    loading: state.loading,
    error: state.error,
    filters: state.filters,
    hasActiveFilters,
    updateFilters,
    clearFilters,
    refresh,
    fetchInitial: () => fetchTraces(state.filters, true),
    fetchNextPage,
    fetchPreviousPage,
    hasNextPage: !!state.nextPageToken,
    hasPreviousPage: state.pageTokenHistory.length > 0,
    currentPage: state.pageTokenHistory.length + 1,
  };
}

"use client";

import { useCallback, useEffect, useReducer, useRef } from "react";
import { traceClient } from "@/lib/api";
import { DEFAULT_PROJECT_ID } from "@/lib/constants";
import { SpanKind } from "@/gen/candela/types/trace_pb";
import { useScope } from "@/components/UserScopeProvider";
import { create } from "@bufbuild/protobuf";
import { TimestampSchema } from "@bufbuild/protobuf/wkt";
import type { SpanResultRow, SpanSearchFilters } from "@/types/search";
import { DEFAULT_SEARCH_FILTERS } from "@/types/search";

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
  spans: SpanResultRow[];
  loading: boolean;
  error: string | null;
  filters: SpanSearchFilters;
  nextPageToken: string;
  currentPageToken: string;
  pageTokenHistory: string[];
};

type Action =
  | { type: "fetch"; filters: SpanSearchFilters; resetPagination?: boolean }
  | { type: "success"; spans: SpanResultRow[]; nextPageToken: string }
  | { type: "error"; message: string }
  | { type: "set_filters"; filters: SpanSearchFilters }
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
      return { ...state, loading: false, spans: action.spans, nextPageToken: action.nextPageToken };
    case "error":
      return { ...state, loading: false, error: action.message };
    case "set_filters":
      return { ...state, filters: action.filters, currentPageToken: "", pageTokenHistory: [], nextPageToken: "" };
    case "clear_filters":
      return { ...state, loading: true, error: null, filters: DEFAULT_SEARCH_FILTERS, currentPageToken: "", pageTokenHistory: [], nextPageToken: "" };
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

function mapSpan(span: any): SpanResultRow {
  const durSeconds = Number(span.duration?.seconds ?? 0);
  const durNanos = Number(span.duration?.nanos ?? 0);
  
  return {
    spanId: span.spanId,
    traceId: span.traceId,
    name: span.name || "unknown",
    kind: span.kind ?? SpanKind.UNSPECIFIED,
    model: span.genai?.model || "—",
    provider: span.genai?.provider || "—",
    durationMs: durSeconds * 1000 + durNanos / 1e6,
    totalTokens: Number(span.genai?.totalTokens) || 0,
    costUsd: span.genai?.costUsd || 0,
    status: span.status ?? 0,
    startTime: span.startTime
      ? new Date(
          Number(span.startTime.seconds) * 1000 +
            Math.floor(Number(span.startTime.nanos) / 1e6)
        ).toLocaleString()
      : "—",
  };
}

export function useSpanSearch() {
  const { isPersonalScope, mode } = useScope();

  const [state, dispatch] = useReducer(reducer, {
    spans: [],
    loading: true,
    error: null,
    filters: DEFAULT_SEARCH_FILTERS,
    nextPageToken: "",
    currentPageToken: "",
    pageTokenHistory: [],
  });
  const debounceRef = useRef<ReturnType<typeof setTimeout> | null>(null);
  const prevModeRef = useRef(mode);
  const fetchRef = useRef<AbortController | null>(null);

  const fetchSpans = useCallback((f: SpanSearchFilters, resetPagination = false) => {
    fetchRef.current?.abort();
    const controller = new AbortController();
    fetchRef.current = controller;

    dispatch({ type: "fetch", filters: f, resetPagination });
    const pageToken = resetPagination ? "" : state.currentPageToken;

    const headers: Record<string, string> = {};
    if (f.jobId) headers["X-Candela-Job-Id"] = f.jobId;
    if (isPersonalScope) headers["X-Candela-Scope"] = "personal";

    traceClient
      .searchSpans({
        projectId: DEFAULT_PROJECT_ID,
        pagination: { pageSize: 100, pageToken },
        nameContains: f.nameContains,
        kind: f.kind === null ? SpanKind.UNSPECIFIED : f.kind,
        model: f.model,
        jobId: f.jobId,
        traceGroup: f.traceGroup,
        timeRange: makeTimeRange(f.timeRange),
        tenantId: "",
      }, {
        headers,
        signal: controller.signal,
      })
      .then((res) => {
        if (!controller.signal.aborted) {
          const nextToken = res.pagination?.nextPageToken ?? "";
          dispatch({
            type: "success",
            spans: (res.spans || []).map(mapSpan),
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
    (patch: Partial<SpanSearchFilters>) => {
      const next = { ...state.filters, ...patch };
      dispatch({ type: "set_filters", filters: next });

      const isSearch = "nameContains" in patch;
      if (debounceRef.current) clearTimeout(debounceRef.current);

      if (isSearch) {
        debounceRef.current = setTimeout(() => fetchSpans(next, true), 300);
      } else {
        fetchSpans(next, true);
      }
    },
    [state.filters, fetchSpans]
  );

  const clearFilters = useCallback(() => {
    dispatch({ type: "clear_filters" });
    fetchSpans(DEFAULT_SEARCH_FILTERS, true);
  }, [fetchSpans]);

  const hasActiveFilters = !!(
    state.filters.nameContains ||
    state.filters.kind !== null ||
    state.filters.model ||
    state.filters.jobId ||
    state.filters.traceGroup
  );

  const refresh = useCallback(
    () => fetchSpans(state.filters),
    [state.filters, fetchSpans]
  );

  useEffect(() => {
    if (prevModeRef.current !== mode) {
      prevModeRef.current = mode;
      fetchSpans(state.filters, true);
    }
  }, [mode, fetchSpans, state.filters]);

  useEffect(() => {
    fetchSpans(state.filters);
  }, [state.currentPageToken, fetchSpans, state.filters]);

  const fetchNextPage = useCallback(() => {
    if (!state.nextPageToken) return;
    dispatch({ type: "set_page_token", direction: "next", token: state.nextPageToken });
  }, [state.nextPageToken]);

  const fetchPreviousPage = useCallback(() => {
    if (state.pageTokenHistory.length === 0) return;
    dispatch({ type: "set_page_token", direction: "prev" });
  }, [state.pageTokenHistory]);

  useEffect(() => {
    return () => fetchRef.current?.abort();
  }, []);

  return {
    spans: state.spans,
    loading: state.loading,
    error: state.error,
    filters: state.filters,
    hasActiveFilters,
    updateFilters,
    clearFilters,
    refresh,
    fetchInitial: () => fetchSpans(state.filters, true),
    fetchNextPage,
    fetchPreviousPage,
    hasNextPage: !!state.nextPageToken,
    hasPreviousPage: state.pageTokenHistory.length > 0,
    currentPage: state.pageTokenHistory.length + 1,
  };
}

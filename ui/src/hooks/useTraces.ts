"use client";

import { useCallback, useRef, useState } from "react";
import { traceClient } from "@/lib/api";
import type { TraceSummaryRow, TraceFilters } from "@/types/traces";
import { DEFAULT_FILTERS } from "@/types/traces";

/**
 * Hook for fetching and filtering traces.
 * Encapsulates the ListTraces RPC, debounced search, and filter state.
 */
export function useTraces() {
  const [traces, setTraces] = useState<TraceSummaryRow[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [filters, setFilters] = useState<TraceFilters>(DEFAULT_FILTERS);
  const debounceRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  const fetchTraces = useCallback((f: TraceFilters) => {
    setLoading(true);
    setError(null);
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
        const mapped = (res.traces || []).map((t) => {
          const durSeconds = Number(t.duration?.seconds ?? 0);
          const durNanos = Number(t.duration?.nanos ?? 0);
          const durationMs = durSeconds * 1000 + durNanos / 1e6;

          return {
            traceId: t.traceId,
            rootSpanName: t.rootSpanName || "unknown",
            primaryModel: t.primaryModel || "—",
            primaryProvider: t.primaryProvider || "—",
            environment: t.environment || "—",
            durationMs,
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
        });
        setTraces(mapped);
      })
      .catch((err) => setError(err.message))
      .finally(() => setLoading(false));
  }, []);

  /** Update filters with debounced search, immediate for other fields. */
  const updateFilters = useCallback(
    (patch: Partial<TraceFilters>) => {
      const next = { ...filters, ...patch };
      setFilters(next);

      const isSearch = "search" in patch;
      if (debounceRef.current) clearTimeout(debounceRef.current);

      if (isSearch) {
        debounceRef.current = setTimeout(() => fetchTraces(next), 300);
      } else {
        fetchTraces(next);
      }
    },
    [filters, fetchTraces]
  );

  const clearFilters = useCallback(() => {
    setFilters(DEFAULT_FILTERS);
    fetchTraces(DEFAULT_FILTERS);
  }, [fetchTraces]);

  const hasActiveFilters = !!(
    filters.search ||
    filters.model ||
    filters.provider ||
    filters.status
  );

  const refresh = useCallback(() => fetchTraces(filters), [filters, fetchTraces]);

  return {
    traces,
    loading,
    error,
    filters,
    hasActiveFilters,
    updateFilters,
    clearFilters,
    refresh,
    /** Call on mount to trigger initial fetch. */
    fetchInitial: () => fetchTraces(filters),
  };
}

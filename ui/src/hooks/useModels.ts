"use client";

import { useCallback, useMemo, useState } from "react";
import { useDashboard, type ModelUsageRow } from "@/hooks/useDashboard";

// ──────────────────────────────────────────
// Sort logic
// ──────────────────────────────────────────

export type ModelSortKey = keyof Pick<
  ModelUsageRow,
  "model" | "provider" | "callCount" | "inputTokens" | "outputTokens" | "costUsd" | "avgLatencyMs"
>;

interface SortState {
  key: ModelSortKey;
  desc: boolean;
}

function compare(a: ModelUsageRow, b: ModelUsageRow, key: ModelSortKey): number {
  const va = a[key];
  const vb = b[key];
  if (typeof va === "string" && typeof vb === "string") {
    return va.localeCompare(vb);
  }
  return (va as number) - (vb as number);
}

// ──────────────────────────────────────────
// Hook
// ──────────────────────────────────────────

/**
 * Hook for the Models page.
 *
 * Re-uses useDashboard() to get model breakdown from GetDashboardData,
 * and adds client-side sort + search.
 */
export function useModels(options?: { includeBudget?: boolean }) {
  const dashboard = useDashboard(options);
  const [sort, setSort] = useState<SortState>({ key: "costUsd", desc: true });
  const [search, setSearch] = useState("");

  const toggleSort = useCallback((key: ModelSortKey) => {
    setSort((prev) =>
      prev.key === key ? { key, desc: !prev.desc } : { key, desc: true }
    );
  }, []);

  const filtered = useMemo(() => {
    let rows = [...dashboard.models];
    if (search) {
      const q = search.toLowerCase();
      rows = rows.filter(
        (r) =>
          r.model.toLowerCase().includes(q) ||
          r.provider.toLowerCase().includes(q)
      );
    }
    rows.sort((a, b) => {
      const c = compare(a, b, sort.key);
      return sort.desc ? -c : c;
    });
    return rows;
  }, [dashboard.models, sort, search]);

  // Aggregate totals
  const totals = useMemo(() => {
    const t = {
      totalCalls: 0,
      totalInputTokens: 0,
      totalOutputTokens: 0,
      totalCost: 0,
      totalCacheRead: 0,
      totalCacheCreation: 0,
    };
    for (const r of dashboard.models) {
      t.totalCalls += r.callCount;
      t.totalInputTokens += r.inputTokens;
      t.totalOutputTokens += r.outputTokens;
      t.totalCost += r.costUsd;
      t.totalCacheRead += r.cacheReadTokens;
      t.totalCacheCreation += r.cacheCreationTokens;
    }
    return t;
  }, [dashboard.models]);

  return {
    models: filtered,
    totals,
    loading: dashboard.loading,
    error: dashboard.error,
    timeRange: dashboard.timeRange,
    setTimeRange: dashboard.setTimeRange,
    refresh: dashboard.refresh,
    sort,
    toggleSort,
    search,
    setSearch,
    budgetContext: dashboard.budgetContext,
  };
}

"use client";

import { useCallback, useMemo, useState } from "react";
import { useDashboard, type ModelUsageRow } from "@/hooks/useDashboard";
import { getCacheEfficiency, type CacheEfficiency } from "@/lib/cacheUtils";
import { useCatalog } from "@/hooks/useCatalog";

// ──────────────────────────────────────────
// Sort logic
// ──────────────────────────────────────────

export type ModelSortKey =
  | keyof Pick<
      ModelUsageRow,
      "model" | "provider" | "callCount" | "inputTokens" | "outputTokens" | "costUsd" | "avgLatencyMs"
    >
  | "inputPrice"
  | "outputPrice";

export interface EnrichedModelRow extends ModelUsageRow {
  inputPricePerMillion: number | null;
  outputPricePerMillion: number | null;
  cacheEfficiency: CacheEfficiency | null;
}

interface SortState {
  key: ModelSortKey;
  desc: boolean;
}

function compare(a: EnrichedModelRow, b: EnrichedModelRow, key: ModelSortKey): number {
  if (key === "inputPrice") return (a.inputPricePerMillion ?? 0) - (b.inputPricePerMillion ?? 0);
  if (key === "outputPrice") return (a.outputPricePerMillion ?? 0) - (b.outputPricePerMillion ?? 0);
  const va = a[key as keyof ModelUsageRow];
  const vb = b[key as keyof ModelUsageRow];
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
 * and adds client-side sort + search. Pricing is sourced from the backend
 * catalog via useCatalog().
 */
export function useModels(options?: { includeBudget?: boolean }) {
  const dashboard = useDashboard(options);
  const catalog = useCatalog();
  const [sort, setSort] = useState<SortState>({ key: "costUsd", desc: true });
  const [search, setSearch] = useState("");

  const toggleSort = useCallback((key: ModelSortKey) => {
    setSort((prev) =>
      prev.key === key ? { key, desc: !prev.desc } : { key, desc: true }
    );
  }, []);

  // Enrich rows with catalog pricing and cache efficiency
  const enriched: EnrichedModelRow[] = useMemo(() => {
    return dashboard.models.map((m) => {
      const pricing = catalog.getPricing(m.provider, m.model);
      return {
        ...m,
        inputPricePerMillion: pricing?.inputPerMillion ?? null,
        outputPricePerMillion: pricing?.outputPerMillion ?? null,
        cacheEfficiency: getCacheEfficiency(m.cacheReadTokens, m.inputTokens),
      };
    });
  }, [dashboard.models, catalog.getPricing]);

  const filtered = useMemo(() => {
    let rows = [...enriched];
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
  }, [enriched, sort, search]);

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

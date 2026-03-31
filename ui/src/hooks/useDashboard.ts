"use client";

import { useCallback, useEffect, useState } from "react";
import { dashboardClient } from "@/lib/api";

export interface DashboardSummary {
  totalTraces: number;
  totalSpans: number;
  totalLlmCalls: number;
  totalInputTokens: number;
  totalOutputTokens: number;
  totalCostUsd: number;
  avgLatencyMs: number;
  errorRate: number;
}

/**
 * Hook for fetching the dashboard usage summary.
 * Encapsulates the GetUsageSummary RPC.
 */
export function useDashboard() {
  const [summary, setSummary] = useState<DashboardSummary | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  const fetchSummary = useCallback(() => {
    setLoading(true);
    setError(null);
    dashboardClient
      .getUsageSummary({})
      .then((res) => {
        setSummary({
          totalTraces: Number(res.totalTraces),
          totalSpans: Number(res.totalSpans),
          totalLlmCalls: Number(res.totalLlmCalls),
          totalInputTokens: Number(res.totalInputTokens),
          totalOutputTokens: Number(res.totalOutputTokens),
          totalCostUsd: res.totalCostUsd,
          avgLatencyMs: res.avgLatencyMs,
          errorRate: res.errorRate,
        });
      })
      .catch((err) => setError(err.message))
      .finally(() => setLoading(false));
  }, []);

  useEffect(() => {
    fetchSummary();
  }, [fetchSummary]);

  return { summary, loading, error, refresh: fetchSummary };
}

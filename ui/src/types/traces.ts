/** Shared types for trace views. */

export interface TraceSummaryRow {
  traceId: string;
  rootSpanName: string;
  primaryModel: string;
  primaryProvider: string;
  environment: string;
  durationMs: number;
  totalTokens: number;
  totalCostUsd: number;
  status: number;
  startTime: string;
  spanCount: number;
  llmCallCount: number;
  tenantId?: string;
  jobId?: string;
}

export interface TraceFilters {
  search: string;
  model: string;
  provider: string;
  status: "" | "ok" | "error";
  orderBy: string;
  descending: boolean;
  tenantId: string;
  jobId: string;
  timeRange: "1h" | "24h" | "7d" | "30d";
  environment: string;
  traceGroup: string;
}

export const DEFAULT_FILTERS: TraceFilters = {
  search: "",
  model: "",
  provider: "",
  status: "",
  orderBy: "start_time",
  descending: true,
  tenantId: "",
  jobId: "",
  timeRange: "24h",
  environment: "",
  traceGroup: "",
};

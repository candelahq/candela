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
}

export interface TraceFilters {
  search: string;
  model: string;
  provider: string;
  status: "" | "ok" | "error";
  orderBy: string;
  descending: boolean;
}

export const DEFAULT_FILTERS: TraceFilters = {
  search: "",
  model: "",
  provider: "",
  status: "",
  orderBy: "start_time",
  descending: true,
};

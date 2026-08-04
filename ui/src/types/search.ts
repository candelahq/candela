import { SpanKind } from "@/gen/candela/types/trace_pb";

export interface SpanResultRow {
  spanId: string;
  traceId: string;
  name: string;
  kind: SpanKind;
  model: string;
  provider: string;
  durationMs: number;
  totalTokens: number;
  costUsd: number;
  status: number;
  startTime: string;
}

export interface SpanSearchFilters {
  nameContains: string;
  kind: SpanKind | null;
  model: string;
  jobId: string;
  traceGroup: string;
  timeRange: "1h" | "24h" | "7d" | "30d";
}

export const DEFAULT_SEARCH_FILTERS: SpanSearchFilters = {
  nameContains: "",
  kind: null,
  model: "",
  jobId: "",
  traceGroup: "",
  timeRange: "24h",
};

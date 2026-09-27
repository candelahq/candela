import { DEFAULT_FILTERS, type TraceFilters } from "@/types/traces";

const VALID_TIME_RANGES = new Set(["1h", "24h", "7d", "30d"]);
const VALID_STATUSES = new Set(["ok", "error"]);

/**
 * Converts TraceFilters into a URLSearchParams object.
 * Only includes keys that deviate from DEFAULT_FILTERS to produce clean, concise URLs.
 */
export function filtersToSearchParams(filters: TraceFilters): URLSearchParams {
  const params = new URLSearchParams();

  if (filters.search) {
    params.set("search", filters.search);
  }
  if (filters.model && filters.model.trim()) {
    params.set("model", filters.model.trim());
  }
  if (filters.provider && filters.provider.trim()) {
    params.set("provider", filters.provider.trim());
  }
  if (filters.status && VALID_STATUSES.has(filters.status)) {
    params.set("status", filters.status);
  }
  if (filters.orderBy && filters.orderBy !== DEFAULT_FILTERS.orderBy) {
    params.set("orderBy", filters.orderBy);
  }
  if (filters.descending !== DEFAULT_FILTERS.descending) {
    params.set("descending", String(filters.descending));
  }
  if (filters.tenantId && filters.tenantId.trim()) {
    params.set("tenantId", filters.tenantId.trim());
  }
  if (filters.jobId && filters.jobId.trim()) {
    params.set("jobId", filters.jobId.trim());
  }
  if (filters.timeRange && filters.timeRange !== DEFAULT_FILTERS.timeRange && VALID_TIME_RANGES.has(filters.timeRange)) {
    params.set("timeRange", filters.timeRange);
  }
  if (filters.environment && filters.environment.trim()) {
    params.set("environment", filters.environment.trim());
  }
  if (filters.traceGroup && filters.traceGroup.trim()) {
    params.set("traceGroup", filters.traceGroup.trim());
  }

  return params;
}

/**
 * Converts TraceFilters to a URL query string (without the leading '?').
 */
export function filtersToQueryString(filters: TraceFilters): string {
  return filtersToSearchParams(filters).toString();
}

/**
 * Parses a query string or URLSearchParams into a typed TraceFilters object.
 * Any missing or invalid parameters fall back to their DEFAULT_FILTERS values.
 */
export function searchParamsToFilters(input: URLSearchParams | string): TraceFilters {
  const params = typeof input === "string" ? new URLSearchParams(input.startsWith("?") ? input.slice(1) : input) : input;

  const statusRaw = params.get("status") || "";
  const status: TraceFilters["status"] = VALID_STATUSES.has(statusRaw) ? (statusRaw as "ok" | "error") : "";

  const timeRangeRaw = params.get("timeRange") || "";
  const timeRange: TraceFilters["timeRange"] = VALID_TIME_RANGES.has(timeRangeRaw)
    ? (timeRangeRaw as "1h" | "24h" | "7d" | "30d")
    : DEFAULT_FILTERS.timeRange;

  const descendingParam = params.get("descending");
  const descending = descendingParam !== null ? descendingParam !== "false" : DEFAULT_FILTERS.descending;

  return {
    search: params.get("search") || DEFAULT_FILTERS.search,
    model: params.get("model") || DEFAULT_FILTERS.model,
    provider: params.get("provider") || DEFAULT_FILTERS.provider,
    status,
    orderBy: params.get("orderBy") || DEFAULT_FILTERS.orderBy,
    descending,
    tenantId: params.get("tenantId") || DEFAULT_FILTERS.tenantId,
    jobId: params.get("jobId") || DEFAULT_FILTERS.jobId,
    timeRange,
    environment: params.get("environment") || DEFAULT_FILTERS.environment,
    traceGroup: params.get("traceGroup") || DEFAULT_FILTERS.traceGroup,
  };
}

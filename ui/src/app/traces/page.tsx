"use client";

import { useEffect, useState, useCallback, useRef } from "react";
import { useRouter } from "next/navigation";
import { createClient } from "@connectrpc/connect";
import { transport } from "@/lib/connect";
import { TraceService } from "@/gen/v1/trace_service_pb";

interface TraceSummaryRow {
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

interface Filters {
  search: string;
  model: string;
  provider: string;
  status: "" | "ok" | "error";
  orderBy: string;
  descending: boolean;
}

const DEFAULT_FILTERS: Filters = {
  search: "",
  model: "",
  provider: "",
  status: "",
  orderBy: "start_time",
  descending: true,
};

export default function TracesPage() {
  const [traces, setTraces] = useState<TraceSummaryRow[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [filters, setFilters] = useState<Filters>(DEFAULT_FILTERS);
  const [filtersOpen, setFiltersOpen] = useState(false);
  const router = useRouter();
  const debounceRef = useRef<ReturnType<typeof setTimeout> | null>(null);

  const fetchTraces = useCallback(
    (f: Filters) => {
      setLoading(true);
      setError(null);
      const client = createClient(TraceService, transport);
      client
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
    },
    []
  );

  // Initial fetch
  useEffect(() => {
    fetchTraces(filters);
    // eslint-disable-next-line react-hooks/exhaustive-deps
  }, []);

  // Debounced filter changes
  const updateFilters = useCallback(
    (patch: Partial<Filters>) => {
      const next = { ...filters, ...patch };
      setFilters(next);

      // Debounce search input, immediate for dropdowns
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

  const clearFilters = () => {
    setFilters(DEFAULT_FILTERS);
    fetchTraces(DEFAULT_FILTERS);
  };

  const hasActiveFilters =
    filters.search || filters.model || filters.provider || filters.status;

  const statusLabel = (s: number) => {
    if (s === 2) return { text: "error", cls: "badge-error" };
    return { text: "ok", cls: "badge-success" };
  };

  const sortOptions = [
    { value: "start_time", label: "Time" },
    { value: "duration", label: "Latency" },
    { value: "total_cost", label: "Cost" },
    { value: "total_tokens", label: "Tokens" },
  ];

  return (
    <>
      <header className="main-header">
        <h1>Traces</h1>
        <div style={{ display: "flex", gap: 8 }}>
          <button className="btn" onClick={() => fetchTraces(filters)}>
            🔄 Refresh
          </button>
        </div>
      </header>

      <div className="main-body">
        {/* Search + Filter bar */}
        <div className="filter-bar animate-in">
          <div className="filter-search">
            <span className="filter-search-icon">🔍</span>
            <input
              type="text"
              placeholder="Search traces by name..."
              value={filters.search}
              onChange={(e) => updateFilters({ search: e.target.value })}
              className="filter-search-input"
            />
            {filters.search && (
              <button
                className="filter-clear-btn"
                onClick={() => updateFilters({ search: "" })}
              >
                ✕
              </button>
            )}
          </div>

          <button
            className={`btn ${filtersOpen ? "btn-active" : ""}`}
            onClick={() => setFiltersOpen(!filtersOpen)}
          >
            ⚙ Filters{hasActiveFilters ? " •" : ""}
          </button>

          {/* Sort */}
          <div className="filter-sort">
            <select
              value={filters.orderBy}
              onChange={(e) => updateFilters({ orderBy: e.target.value })}
              className="filter-select"
            >
              {sortOptions.map((o) => (
                <option key={o.value} value={o.value}>
                  Sort: {o.label}
                </option>
              ))}
            </select>
            <button
              className="btn filter-dir-btn"
              onClick={() => updateFilters({ descending: !filters.descending })}
              title={filters.descending ? "Descending" : "Ascending"}
            >
              {filters.descending ? "↓" : "↑"}
            </button>
          </div>
        </div>

        {/* Expanded filters */}
        {filtersOpen && (
          <div className="filter-panel animate-in">
            <div className="filter-group">
              <label className="filter-label">Model</label>
              <input
                type="text"
                placeholder="e.g. gpt-4o, gemini-2.5-pro"
                value={filters.model}
                onChange={(e) => updateFilters({ model: e.target.value })}
                className="filter-input"
              />
            </div>
            <div className="filter-group">
              <label className="filter-label">Provider</label>
              <input
                type="text"
                placeholder="e.g. openai, google, anthropic"
                value={filters.provider}
                onChange={(e) => updateFilters({ provider: e.target.value })}
                className="filter-input"
              />
            </div>
            <div className="filter-group">
              <label className="filter-label">Status</label>
              <select
                value={filters.status}
                onChange={(e) =>
                  updateFilters({
                    status: e.target.value as Filters["status"],
                  })
                }
                className="filter-select"
              >
                <option value="">All</option>
                <option value="ok">OK</option>
                <option value="error">Error</option>
              </select>
            </div>
            {hasActiveFilters && (
              <button className="btn filter-reset-btn" onClick={clearFilters}>
                ✕ Clear all
              </button>
            )}
          </div>
        )}

        {/* Error banner */}
        {error && (
          <div
            className="card animate-in"
            style={{
              borderColor: "var(--error)",
              marginBottom: 24,
              background: "rgba(248,113,113,0.05)",
            }}
          >
            <div className="card-title" style={{ color: "var(--error)" }}>
              Could not load traces
            </div>
            <div style={{ fontSize: 13, color: "var(--text-secondary)" }}>
              {error}
            </div>
          </div>
        )}

        {/* Results */}
        <div className="table-container animate-in">
          <div className="table-header">
            <span className="table-title">
              {loading
                ? "Loading..."
                : `${traces.length} trace${traces.length !== 1 ? "s" : ""}`}
            </span>
            {hasActiveFilters && (
              <span
                className="badge badge-info"
                style={{ fontSize: 11, cursor: "pointer" }}
                onClick={clearFilters}
              >
                Filtered — clear
              </span>
            )}
          </div>

          {traces.length === 0 && !loading ? (
            <div className="empty-state">
              <div className="empty-state-icon">
                {hasActiveFilters ? "🔍" : "🔍"}
              </div>
              <div className="empty-state-title">
                {hasActiveFilters
                  ? "No traces match filters"
                  : "No traces found"}
              </div>
              <div className="empty-state-desc">
                {hasActiveFilters ? (
                  <>
                    Try adjusting your filters or{" "}
                    <button
                      onClick={clearFilters}
                      style={{
                        background: "none",
                        border: "none",
                        color: "var(--accent)",
                        cursor: "pointer",
                        textDecoration: "underline",
                        padding: 0,
                        font: "inherit",
                      }}
                    >
                      clear all filters
                    </button>
                    .
                  </>
                ) : (
                  "Traces will appear here once LLM requests flow through the proxy."
                )}
              </div>
            </div>
          ) : (
            <table>
              <thead>
                <tr>
                  <th>Trace ID</th>
                  <th>Operation</th>
                  <th>Model</th>
                  <th>Spans</th>
                  <th>Tokens</th>
                  <th>Cost</th>
                  <th>Latency</th>
                  <th>Status</th>
                  <th>Time</th>
                </tr>
              </thead>
              <tbody>
                {traces.map((t) => {
                  const st = statusLabel(t.status);
                  return (
                    <tr
                      key={t.traceId}
                      style={{ cursor: "pointer" }}
                      onClick={() => router.push(`/traces/${t.traceId}`)}
                    >
                      <td>
                        <span className="mono">{t.traceId.slice(0, 12)}…</span>
                      </td>
                      <td>{t.rootSpanName}</td>
                      <td>
                        <span className="mono" style={{ fontSize: 12 }}>
                          {t.primaryModel}
                        </span>
                      </td>
                      <td>{t.spanCount}</td>
                      <td>{t.totalTokens.toLocaleString()}</td>
                      <td>${t.totalCostUsd.toFixed(4)}</td>
                      <td>{t.durationMs.toFixed(0)}ms</td>
                      <td>
                        <span className={`badge ${st.cls}`}>{st.text}</span>
                      </td>
                      <td
                        style={{ color: "var(--text-muted)", fontSize: 12 }}
                      >
                        {t.startTime}
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          )}
        </div>
      </div>
    </>
  );
}

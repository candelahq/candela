"use client";

import { useEffect, useState } from "react";
import { useRouter } from "next/navigation";
import { useTraces } from "@/hooks/useTraces";
import { ErrorBanner } from "@/components/ErrorBanner";
import type { TraceFilters } from "@/types/traces";

const sortOptions = [
  { value: "start_time", label: "Time" },
  { value: "duration", label: "Latency" },
  { value: "total_cost", label: "Cost" },
  { value: "total_tokens", label: "Tokens" },
];

const statusLabel = (s: number) => {
  if (s === 2) return { text: "error", cls: "badge-error" };
  return { text: "ok", cls: "badge-success" };
};

export default function TracesPage() {
  const router = useRouter();
  const {
    traces,
    loading,
    error,
    filters,
    hasActiveFilters,
    updateFilters,
    clearFilters,
    refresh,
    fetchInitial,
  } = useTraces();

  const [filtersOpen, setFiltersOpen] = useState(false);

  // eslint-disable-next-line react-hooks/exhaustive-deps
  useEffect(() => { fetchInitial(); }, []);

  return (
    <>
      <header className="main-header">
        <h1>Traces</h1>
        <div style={{ display: "flex", gap: 8 }}>
          <button className="btn" onClick={refresh}>
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
                    status: e.target.value as TraceFilters["status"],
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
          <ErrorBanner title="Could not load traces">
            {error}
          </ErrorBanner>
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
              <div className="empty-state-icon">🔍</div>
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
                      className="clickable-row"
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
                      <td style={{ color: "var(--text-muted)", fontSize: 12 }}>
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

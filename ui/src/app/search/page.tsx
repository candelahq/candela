"use client";

import { useEffect } from "react";
import { useRouter } from "next/navigation";
import { useSpanSearch } from "@/hooks/useSpanSearch";
import { ScopeToggle } from "@/components/ScopeToggle";
import { ErrorBanner } from "@/components/ErrorBanner";
import { SpanKind } from "@/gen/candela/types/trace_pb";

const kindLabels: Record<number, string> = {
  [SpanKind.LLM]: "LLM",
  [SpanKind.AGENT]: "Agent",
  [SpanKind.TOOL]: "Tool",
  [SpanKind.RETRIEVAL]: "Retrieval",
  [SpanKind.EMBEDDING]: "Embedding",
  [SpanKind.CHAIN]: "Chain",
  [SpanKind.GENERAL]: "General",
};

const kindColors: Record<number, string> = {
  [SpanKind.LLM]: "#f0a030",
  [SpanKind.AGENT]: "#a78bfa",
  [SpanKind.TOOL]: "#34d399",
  [SpanKind.RETRIEVAL]: "#60a5fa",
  [SpanKind.EMBEDDING]: "#f472b6",
  [SpanKind.CHAIN]: "#fbbf24",
  [SpanKind.GENERAL]: "#6b7280",
};

function kindLabelForSearch(kind: SpanKind) {
  return kindLabels[kind] || "Unknown";
}

function kindColorForSearch(kind: SpanKind) {
  return kindColors[kind] || "var(--text-muted)";
}

const statusLabel = (s: number) => {
  if (s === 2) return { text: "error", cls: "badge-error" };
  return { text: "ok", cls: "badge-success" };
};

export default function SearchPage() {
  const router = useRouter();
  const {
    spans,
    loading,
    error,
    filters,
    hasActiveFilters,
    updateFilters,
    clearFilters,
    refresh,
    fetchInitial,
    fetchNextPage,
    fetchPreviousPage,
    hasNextPage,
    hasPreviousPage,
    currentPage,
  } = useSpanSearch();

  // eslint-disable-next-line react-hooks/exhaustive-deps
  useEffect(() => { fetchInitial(); }, []);

  return (
    <>
      <header className="main-header">
        <h1>Search Spans</h1>
        <div style={{ display: "flex", gap: 8, alignItems: "center" }}>
          {/* Time range buttons */}
          <div className="time-range-selector" style={{ display: "flex", gap: "4px" }}>
            {(["1h", "24h", "7d", "30d"] as const).map((r) => (
              <button
                key={r}
                className={`btn btn-sm ${filters.timeRange === r ? "btn-active" : "btn-ghost"}`}
                onClick={() => updateFilters({ timeRange: r })}
                style={{ padding: "4px 8px", fontSize: "12px", borderRadius: "4px" }}
              >
                {r}
              </button>
            ))}
          </div>
          <ScopeToggle />
          <button className="btn" onClick={refresh}>
            🔄 Refresh
          </button>
        </div>
      </header>

      <div className="main-body">
        {/* Search bar - larger, prominent */}
        <div className="search-hero animate-in">
          <input
            type="text"
            className="search-hero-input"
            placeholder="Search spans by name, tool, or operation..."
            value={filters.nameContains}
            onChange={(e) => updateFilters({ nameContains: e.target.value })}
          />
        </div>

        {/* Filter chips row */}
        <div className="filter-bar animate-in" style={{ display: "flex", gap: "12px", flexWrap: "wrap", marginBottom: "16px" }}>
          <div className="filter-group" style={{ minWidth: "140px" }}>
            <select
              value={filters.kind === null ? "all" : filters.kind.toString()}
              onChange={(e) =>
                updateFilters({
                  kind: e.target.value === "all" ? null : parseInt(e.target.value, 10),
                })
              }
              className="filter-select"
            >
              <option value="all">Kind: All</option>
              <option value={SpanKind.LLM}>LLM</option>
              <option value={SpanKind.AGENT}>Agent</option>
              <option value={SpanKind.TOOL}>Tool</option>
              <option value={SpanKind.RETRIEVAL}>Retrieval</option>
              <option value={SpanKind.EMBEDDING}>Embedding</option>
              <option value={SpanKind.CHAIN}>Chain</option>
              <option value={SpanKind.GENERAL}>General</option>
            </select>
          </div>
          <div className="filter-group">
            <input
              type="text"
              placeholder="Model (e.g. gpt-4o)"
              value={filters.model}
              onChange={(e) => updateFilters({ model: e.target.value })}
              className="filter-input"
            />
          </div>
          <div className="filter-group">
            <input
              type="text"
              placeholder="Job ID"
              value={filters.jobId}
              onChange={(e) => updateFilters({ jobId: e.target.value })}
              className="filter-input"
            />
          </div>
          <div className="filter-group">
            <input
              type="text"
              placeholder="Trace Group"
              value={filters.traceGroup}
              onChange={(e) => updateFilters({ traceGroup: e.target.value })}
              className="filter-input"
            />
          </div>
          {hasActiveFilters && (
            <button className="btn filter-reset-btn" onClick={clearFilters}>
              ✕ Clear all
            </button>
          )}
        </div>

        {/* Error banner */}
        {error && (
          <ErrorBanner title="Could not load spans">
            {error}
          </ErrorBanner>
        )}

        {/* Results */}
        <div className="table-container animate-in">
          <div className="table-header">
            <span className="table-title">
              {loading
                ? "Loading..."
                : `Page ${currentPage}`}
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

          {spans.length === 0 && !loading ? (
            <div className="empty-state">
              <div className="empty-state-icon">🔍</div>
              <div className="empty-state-title">
                {hasActiveFilters
                  ? "No spans match filters"
                  : "No spans found"}
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
                  "Spans will appear here once LLM requests flow through the proxy."
                )}
              </div>
            </div>
          ) : (
            <table>
              <thead>
                <tr>
                  <th>Span ID</th>
                  <th>Name</th>
                  <th>Kind</th>
                  <th>Model</th>
                  <th>Tokens</th>
                  <th>Cost</th>
                  <th>Latency</th>
                  <th>Status</th>
                  <th>Trace</th>
                  <th>Time</th>
                </tr>
              </thead>
              <tbody>
                {spans.map((s) => {
                  const st = statusLabel(s.status);
                  return (
                    <tr
                      key={s.spanId}
                      className="clickable-row"
                      tabIndex={0}
                      role="link"
                      onClick={() => router.push(`/traces/${s.traceId}`)}
                      onKeyDown={(e) => { if (e.key === "Enter") router.push(`/traces/${s.traceId}`); }}
                    >
                      <td>
                        <span className="mono">{s.spanId.slice(0, 12)}…</span>
                      </td>
                      <td>{s.name}</td>
                      <td>
                        <span className="kind-badge" style={{ background: kindColorForSearch(s.kind) }}>
                          {kindLabelForSearch(s.kind)}
                        </span>
                      </td>
                      <td>
                        <span className="mono" style={{ fontSize: 12 }}>
                          {s.model}
                        </span>
                      </td>
                      <td>{s.totalTokens.toLocaleString()}</td>
                      <td>${s.costUsd.toFixed(4)}</td>
                      <td>{s.durationMs.toFixed(0)}ms</td>
                      <td>
                        <span className={`badge ${st.cls}`}>{st.text}</span>
                      </td>
                      <td>
                        <button
                          onClick={(e) => {
                            e.stopPropagation();
                            router.push(`/traces/${s.traceId}`);
                          }}
                          style={{
                            background: "none",
                            border: "none",
                            color: "var(--accent)",
                            cursor: "pointer",
                            textDecoration: "underline",
                            font: "inherit",
                            padding: 0
                          }}
                        >
                          View Trace
                        </button>
                      </td>
                      <td style={{ color: "var(--text-muted)", fontSize: 12 }}>
                        {s.startTime}
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          )}

          {/* Pagination controls */}
          {(hasPreviousPage || hasNextPage) && (
            <div className="pagination-controls" style={{ display: "flex", justifyContent: "space-between", alignItems: "center", padding: "16px 20px", borderTop: "1px solid var(--border-subtle)" }}>
              <button
                className="btn btn-sm btn-ghost"
                onClick={fetchPreviousPage}
                disabled={!hasPreviousPage}
              >
                ← Previous
              </button>
              <span className="pagination-info" style={{ fontSize: "13px", color: "var(--text-secondary)" }}>
                Page {currentPage}
              </span>
              <button
                className="btn btn-sm btn-ghost"
                onClick={fetchNextPage}
                disabled={!hasNextPage}
              >
                Next →
              </button>
            </div>
          )}
        </div>
      </div>
    </>
  );
}

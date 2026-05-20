"use client";

import { useModels, type ModelSortKey, type EnrichedModelRow } from "@/hooks/useModels";
import { type CacheEfficiency } from "@/lib/modelPricing";
import { TimeRangeSelector } from "@/components/TimeRangeSelector";
import { ScopeToggle } from "@/components/ScopeToggle";
import { useScope } from "@/components/UserScopeProvider";
import { ErrorBanner } from "@/components/ErrorBanner";
import { SkeletonCard } from "@/components/SkeletonCard";

// ──────────────────────────────────────────
// Helpers
// ──────────────────────────────────────────

function fmtTokens(n: number): string {
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`;
  if (n >= 1_000) return `${(n / 1_000).toFixed(1)}k`;
  return n.toLocaleString();
}

// ──────────────────────────────────────────
// Sort header
// ──────────────────────────────────────────

function SortTh({
  label,
  sortKey,
  currentKey,
  desc,
  onSort,
  align,
}: {
  label: string;
  sortKey: ModelSortKey;
  currentKey: ModelSortKey;
  desc: boolean;
  onSort: (k: ModelSortKey) => void;
  align?: "right";
}) {
  const active = currentKey === sortKey;
  return (
    <th
      onClick={() => onSort(sortKey)}
      style={{ cursor: "pointer", textAlign: align ?? "left" }}
    >
      {label}{" "}
      {active && (
        <span style={{ fontSize: 10, opacity: 0.6 }}>
          {desc ? "▼" : "▲"}
        </span>
      )}
    </th>
  );
}

// ──────────────────────────────────────────
// Page
// ──────────────────────────────────────────

export default function ModelsPage() {
  const { includeBudget } = useScope();
  const {
    models,
    totals,
    loading,
    error,
    timeRange,
    setTimeRange,
    refresh,
    sort,
    toggleSort,
    search,
    setSearch,
    budgetContext,
  } = useModels({ includeBudget });

  return (
    <>
      <header className="main-header">
        <h1>Models</h1>
        <div style={{ display: "flex", gap: 8, alignItems: "center" }}>
          <ScopeToggle />
          <TimeRangeSelector value={timeRange} onChange={setTimeRange} />
          <button className="btn" onClick={refresh}>🔄</button>
        </div>
      </header>

      <div className="main-body">
        {error && <ErrorBanner title="Models Error">{error}</ErrorBanner>}

        {/* Summary cards */}
        {loading && models.length === 0 ? (
          <div className="stats-grid animate-in">
            <SkeletonCard />
            <SkeletonCard />
            <SkeletonCard />
            <SkeletonCard />
          </div>
        ) : (
          <div className="stats-grid animate-in">
            <div className="card">
              <div className="card-title">Models Used</div>
              <div className="card-value">{models.length}</div>
              <div className="card-subtitle">Unique models</div>
            </div>
            <div className="card">
              <div className="card-title">Total Calls</div>
              <div className="card-value">{totals.totalCalls.toLocaleString()}</div>
              <div className="card-subtitle">Across all models</div>
            </div>
            <div className="card">
              <div className="card-title">Total Tokens</div>
              <div className="card-value">
                {fmtTokens(totals.totalInputTokens + totals.totalOutputTokens)}
              </div>
              <div className="card-subtitle">
                {fmtTokens(totals.totalInputTokens)} in / {fmtTokens(totals.totalOutputTokens)} out
              </div>
            </div>
            <div className="card">
              <div className="card-title">Total Cost</div>
              <div className="card-value">${totals.totalCost.toFixed(2)}</div>
              <div className="card-subtitle">
                {budgetContext?.budget
                  ? `$${budgetContext.totalRemainingUsd.toFixed(2)} remaining`
                  : "Estimated USD"}
              </div>
            </div>
          </div>
        )}

        {/* Budget bar (if available) */}
        {budgetContext?.budget && (
          <div className="card animate-in" style={{ marginBottom: 16, animationDelay: "0.03s" }}>
            <div className="card-title">Budget</div>
            <div className="budget-bar-container">
              <div className="budget-bar-track">
                <div
                  className="budget-bar-fill"
                  style={{
                    width: `${(totals.totalCost + budgetContext.totalRemainingUsd) > 0 ? Math.min(100, (totals.totalCost / (totals.totalCost + budgetContext.totalRemainingUsd)) * 100) : 0}%`,
                  }}
                />
              </div>
              <div className="budget-bar-labels">
                <span>${totals.totalCost.toFixed(2)} spent</span>
                <span>${budgetContext.totalRemainingUsd.toFixed(2)} remaining</span>
              </div>
            </div>
          </div>
        )}

        {/* Search */}
        <div className="models-search animate-in" style={{ animationDelay: "0.05s" }}>
          <input
            type="text"
            className="models-search-input"
            placeholder="Search models or providers…"
            value={search}
            onChange={(e) => setSearch(e.target.value)}
          />
        </div>

        {/* Models table */}
        <div className="table-container animate-in" style={{ animationDelay: "0.08s" }}>
          <div className="table-header">
            <span className="table-title">Model Breakdown</span>
            <span style={{ fontSize: 12, color: "var(--text-muted)" }}>
              {models.length} model{models.length !== 1 ? "s" : ""}
            </span>
          </div>
          {models.length === 0 ? (
            <div className="empty-state" style={{ minHeight: 200 }}>
              <div className="empty-state-icon">🤖</div>
              <div className="empty-state-title">
                {search ? "No matching models" : "No model data"}
              </div>
              <div className="empty-state-desc">
                {search
                  ? "Try a different search term."
                  : "Send requests through Candela to see model usage analytics."}
              </div>
            </div>
          ) : (
            <table>
              <thead>
                <tr>
                  <SortTh label="Model" sortKey="model" {...sort} currentKey={sort.key} onSort={toggleSort} />
                  <SortTh label="Provider" sortKey="provider" {...sort} currentKey={sort.key} onSort={toggleSort} />
                  <SortTh label="In $/M" sortKey="inputPrice" {...sort} currentKey={sort.key} onSort={toggleSort} align="right" />
                  <SortTh label="Out $/M" sortKey="outputPrice" {...sort} currentKey={sort.key} onSort={toggleSort} align="right" />
                  <SortTh label="Calls" sortKey="callCount" {...sort} currentKey={sort.key} onSort={toggleSort} align="right" />
                  <SortTh label="Input Tokens" sortKey="inputTokens" {...sort} currentKey={sort.key} onSort={toggleSort} align="right" />
                  <SortTh label="Output Tokens" sortKey="outputTokens" {...sort} currentKey={sort.key} onSort={toggleSort} align="right" />
                  <SortTh label="Cost" sortKey="costUsd" {...sort} currentKey={sort.key} onSort={toggleSort} align="right" />
                  <SortTh label="Avg Latency" sortKey="avgLatencyMs" {...sort} currentKey={sort.key} onSort={toggleSort} align="right" />
                  <th style={{ textAlign: "center" }}>Cache</th>
                  <th style={{ textAlign: "right" }}>Cost %</th>
                </tr>
              </thead>
              <tbody>
                {models.map((m) => {
                  const pct = totals.totalCost > 0 ? (m.costUsd / totals.totalCost) * 100 : 0;
                  return (
                    <tr key={`${m.model}-${m.provider}`}>
                      <td>
                        <span className="mono" style={{ fontSize: 12 }}>{m.model}</span>
                      </td>
                      <td>
                        <span className="badge badge-info">{m.provider}</span>
                      </td>
                      <td style={{ textAlign: "right", fontFamily: "monospace", fontSize: 11, color: "var(--text-secondary)" }}>
                        {m.inputPricePerMillion != null ? `$${m.inputPricePerMillion.toFixed(3)}` : "—"}
                      </td>
                      <td style={{ textAlign: "right", fontFamily: "monospace", fontSize: 11, color: "var(--text-secondary)" }}>
                        {m.outputPricePerMillion != null ? `$${m.outputPricePerMillion.toFixed(3)}` : "—"}
                      </td>
                      <td style={{ textAlign: "right" }}>{m.callCount.toLocaleString()}</td>
                      <td style={{ textAlign: "right" }}>{fmtTokens(m.inputTokens)}</td>
                      <td style={{ textAlign: "right" }}>{fmtTokens(m.outputTokens)}</td>
                      <td style={{ textAlign: "right" }}>${m.costUsd.toFixed(4)}</td>
                      <td style={{ textAlign: "right" }}>{m.avgLatencyMs.toFixed(0)}ms</td>
                      <td style={{ textAlign: "center" }}>
                        {m.cacheEfficiency ? (
                          <CacheBadge eff={m.cacheEfficiency} />
                        ) : (
                          <span style={{ color: "var(--text-muted)", fontSize: 11 }}>—</span>
                        )}
                      </td>
                      <td style={{ textAlign: "right" }}>
                        <div style={{ display: "flex", alignItems: "center", gap: 8, justifyContent: "flex-end" }}>
                          <div style={{ width: 60, height: 4, background: "var(--bg-tertiary)", borderRadius: 2 }}>
                            <div
                              style={{
                                height: "100%",
                                width: `${Math.min(100, pct)}%`,
                                background: "var(--accent)",
                                borderRadius: 2,
                              }}
                            />
                          </div>
                          <span style={{ fontSize: 11, color: "var(--text-muted)", minWidth: 32 }}>
                            {pct.toFixed(1)}%
                          </span>
                        </div>
                      </td>
                    </tr>
                  );
                })}
              </tbody>
            </table>
          )}
        </div>

        {/* Cache stats */}
        {(totals.totalCacheRead > 0 || totals.totalCacheCreation > 0) && (
          <div className="card animate-in" style={{ marginTop: 16, animationDelay: "0.12s" }}>
            <div className="card-title">Cache Performance</div>
            <div className="settings-grid">
              <div className="settings-row">
                <span className="settings-label">Cache Read Tokens</span>
                <span className="settings-value">{fmtTokens(totals.totalCacheRead)}</span>
              </div>
              <div className="settings-row">
                <span className="settings-label">Cache Creation Tokens</span>
                <span className="settings-value">{fmtTokens(totals.totalCacheCreation)}</span>
              </div>
              <div className="settings-row">
                <span className="settings-label">Effective Hit Rate</span>
                <span className="settings-value">
                  {totals.totalInputTokens > 0
                    ? `${((totals.totalCacheRead / totals.totalInputTokens) * 100).toFixed(1)}%`
                    : "—"}
                </span>
              </div>
            </div>
          </div>
        )}
      </div>
    </>
  );
}

// ──────────────────────────────────────────
// Cache badge inline component
// ──────────────────────────────────────────

function CacheBadge({ eff }: { eff: CacheEfficiency }) {
  const color = eff.color;
  return (
    <span
      style={{
        display: "inline-block",
        padding: "2px 8px",
        fontSize: 10,
        fontWeight: 700,
        fontFamily: "monospace",
        color,
        background: `color-mix(in srgb, ${color} 12%, transparent)`,
        border: `1px solid color-mix(in srgb, ${color} 30%, transparent)`,
        borderRadius: 6,
      }}
    >
      {eff.label} {(eff.rate * 100).toFixed(0)}%
    </span>
  );
}

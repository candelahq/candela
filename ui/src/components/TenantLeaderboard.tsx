"use client";

import { useTenantLeaderboard, type SortField } from "@/hooks/useTenantLeaderboard";
import { TimeRangeSelector } from "@/components/TimeRangeSelector";
import { ErrorBanner } from "@/components/ErrorBanner";

const COLUMNS: { label: string; field: SortField; style?: React.CSSProperties }[] = [
  { label: "Tenant", field: "tenantId" },
  { label: "Calls", field: "callCount" },
  { label: "Tokens", field: "totalTokens" },
  { label: "Cost (USD)", field: "costUsd" },
  { label: "Top Model", field: "topModel" },
  { label: "Avg Latency", field: "avgLatencyMs" },
];

export function TenantLeaderboard() {
  const {
    tenants,
    loading,
    error,
    timeRange,
    setTimeRange,
    sortField,
    sortAsc,
    setSort,
    refresh,
  } = useTenantLeaderboard();

  const sortIndicator = (field: SortField) => {
    if (sortField !== field) return "";
    return sortAsc ? " ↑" : " ↓";
  };

  return (
    <>
      <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", marginBottom: 16 }}>
        <div>
          <span className="table-title">Tenant Cost Leaderboard</span>
          <span style={{ marginLeft: 8, fontSize: 12, color: "var(--text-muted)" }}>
            {tenants.length} tenant{tenants.length !== 1 ? "s" : ""}
          </span>
        </div>
        <div style={{ display: "flex", gap: 8, alignItems: "center" }}>
          <TimeRangeSelector value={timeRange} onChange={setTimeRange} />
          <button className="btn" onClick={refresh}>
            🔄
          </button>
        </div>
      </div>

      {error && (
        <ErrorBanner title="Tenant Leaderboard Error">
          {error}
        </ErrorBanner>
      )}

      {/* Summary cards */}
      {tenants.length > 0 && (
        <div className="stats-grid animate-in" style={{ marginBottom: 20 }}>
          <div className="card">
            <div className="card-title">Top Tenant</div>
            <div className="card-value">{tenants[0]?.tenantId ?? "—"}</div>
            <div className="card-subtitle">
              {tenants[0] ? `$${tenants[0].costUsd.toFixed(2)} total` : "No data"}
            </div>
          </div>
          <div className="card">
            <div className="card-title">Active Tenants</div>
            <div className="card-value">{tenants.length}</div>
            <div className="card-subtitle">With activity in this period</div>
          </div>
          <div className="card">
            <div className="card-title">Total Tenant Cost</div>
            <div className="card-value">
              ${tenants.reduce((acc, t) => acc + t.costUsd, 0).toFixed(2)}
            </div>
            <div className="card-subtitle">Across all tenants</div>
          </div>
        </div>
      )}

      <div className="table-container animate-in" style={{ animationDelay: "0.05s" }}>
        {tenants.length === 0 && !loading ? (
          <div className="empty-state">
            <div className="empty-state-icon">🏢</div>
            <div className="empty-state-title">No tenant data yet</div>
            <div className="empty-state-desc">
              When downstream tenants send requests with X-Candela-Tenant-Id headers
              or W3C Baggage (candela.tenant_id), they will appear here ranked by cost.
            </div>
          </div>
        ) : (
          <table>
            <thead>
              <tr>
                {COLUMNS.map((col) => (
                  <th
                    key={col.field}
                    onClick={() => setSort(col.field)}
                    style={{
                      cursor: "pointer",
                      userSelect: "none",
                      ...col.style,
                    }}
                  >
                    {col.label}{sortIndicator(col.field)}
                  </th>
                ))}
                <th style={{ width: 120 }}>Cost Share</th>
              </tr>
            </thead>
            <tbody>
              {loading ? (
                Array.from({ length: 5 }).map((_, i) => (
                  <tr key={i}>
                    <td colSpan={7} style={{ height: 48 }}>
                      <div className="skeleton" style={{ height: 16, width: "100%" }} />
                    </td>
                  </tr>
                ))
              ) : (
                tenants.map((t) => {
                  const totalCost = tenants.reduce((acc, x) => acc + x.costUsd, 0);
                  const percent = totalCost > 0 ? (t.costUsd / totalCost) * 100 : 0;
                  return (
                    <tr key={t.tenantId}>
                      <td className="mono" style={{ fontWeight: 600 }}>{t.tenantId}</td>
                      <td>{t.callCount.toLocaleString()}</td>
                      <td>{t.totalTokens >= 1000 ? `${(t.totalTokens / 1000).toFixed(1)}k` : t.totalTokens.toLocaleString()}</td>
                      <td style={{ fontWeight: 600 }}>${t.costUsd.toFixed(4)}</td>
                      <td>
                        <span className="badge badge-success" style={{ fontSize: 10 }}>{t.topModel}</span>
                      </td>
                      <td>{t.avgLatencyMs.toFixed(0)}ms</td>
                      <td>
                        <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
                          <div style={{ flex: 1, height: 4, background: "var(--bg-tertiary)", borderRadius: 2 }}>
                            <div style={{ height: "100%", width: `${Math.min(100, percent)}%`, background: "var(--accent)", borderRadius: 2 }} />
                          </div>
                          <span style={{ fontSize: 11, color: "var(--text-muted)", width: 30 }}>{percent.toFixed(0)}%</span>
                        </div>
                      </td>
                    </tr>
                  );
                })
              )}
            </tbody>
          </table>
        )}
      </div>

      <style jsx>{`
        .skeleton {
          background: linear-gradient(90deg, var(--bg-tertiary) 25%, var(--bg-elevated) 50%, var(--bg-tertiary) 75%);
          background-size: 200% 100%;
          animation: loading 1.5s infinite;
        }
        @keyframes loading {
          0% { background-position: 200% 0; }
          100% { background-position: -200% 0; }
        }
      `}</style>
    </>
  );
}

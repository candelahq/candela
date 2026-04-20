"use client";

import { useUsage } from "@/hooks/useUsage";
import { BudgetGauge } from "@/components/BudgetGauge";
import { TimeRangeSelector } from "@/components/TimeRangeSelector";
import { ErrorBanner } from "@/components/ErrorBanner";
import { SkeletonCard } from "@/components/SkeletonCard";

export default function UsagePage() {
  const { data, loading, error, timeRange, setTimeRange, refresh } = useUsage();

  return (
    <>
      <header className="main-header">
        <h1>My Usage</h1>
        <div style={{ display: "flex", gap: 8, alignItems: "center" }}>
          <TimeRangeSelector value={timeRange} onChange={setTimeRange} />
          <button className="btn" onClick={refresh}>
            🔄
          </button>
        </div>
      </header>

      <div className="main-body">
        {error && (
          <ErrorBanner title="Connection Error">
            Failed to fetch usage data: {error}
          </ErrorBanner>
        )}

        {/* Budget visualization */}
        {loading && !data ? (
          <div className="card" style={{ height: 200, marginBottom: 24, background: "var(--bg-secondary)" }}>
            <div className="skeleton" style={{ height: "100%", width: "100%", borderRadius: "var(--radius-lg)" }} />
          </div>
        ) : data?.budget ? (
          <div style={{ marginBottom: 24 }}>
            <BudgetGauge
              spent={data.budget.spentUsd}
              limit={data.budget.limitUsd}
              remaining={data.budget.remainingUsd}
              percent={data.budget.percentUsed}
              period={data.budget.periodType}
            />
          </div>
        ) : null}

        {/* Usage Stats Grid */}
        <div className="stats-grid animate-in" style={{ animationDelay: "0.1s" }}>
          <div className="card">
            <div className="card-title">Total Requests</div>
            <div className="card-value">{loading && !data ? "—" : data?.totalCalls?.toLocaleString()}</div>
            <div className="card-subtitle">Calls made by you</div>
          </div>
          <div className="card">
            <div className="card-title">Estimated Cost</div>
            <div className="card-value">{loading && !data ? "—" : `$${data?.totalCostUsd?.toFixed(4)}`}</div>
            <div className="card-subtitle">USD equivalent</div>
          </div>
          <div className="card">
            <div className="card-title">Total Tokens</div>
            <div className="card-value">
              {loading && !data ? "—" : ((data?.totalInputTokens || 0) + (data?.totalOutputTokens || 0)).toLocaleString()}
            </div>
            <div className="card-subtitle">In + Out</div>
          </div>
          <div className="card">
            <div className="card-title">Avg Latency</div>
            <div className="card-value">{loading && !data ? "—" : `${data?.avgLatencyMs?.toFixed(0)}ms`}</div>
            <div className="card-subtitle">Across all models</div>
          </div>
        </div>

        {/* Model Breakdown */}
        <div className="table-container animate-in" style={{ animationDelay: "0.2s", marginTop: 24 }}>
          <div className="table-header">
            <span className="table-title">Model Breakdown</span>
          </div>
          {!loading && data?.models.length === 0 ? (
            <div className="empty-state">
              <div className="empty-state-icon">🤖</div>
              <div className="empty-state-title">No model usage yet</div>
              <div className="empty-state-desc">
                Requests you send through the proxy will be attributed to your account and shown here with cost estimates.
              </div>
            </div>
          ) : (
            <table>
              <thead>
                <tr>
                  <th>Model</th>
                  <th>Provider</th>
                  <th>Requests</th>
                  <th>Total Cost</th>
                  <th>% of Total</th>
                </tr>
              </thead>
              <tbody>
                {loading && !data ? (
                  Array.from({ length: 3 }).map((_, i) => (
                    <tr key={i}>
                      <td colSpan={5} style={{ height: 40 }}><div className="skeleton" style={{ height: 16, width: "100%" }} /></td>
                    </tr>
                  ))
                ) : (
                  data?.models.map((m) => {
                    const percent = data.totalCostUsd > 0 ? (m.costUsd / data.totalCostUsd) * 100 : 0;
                    return (
                      <tr key={`${m.provider}-${m.model}`}>
                        <td className="mono">{m.model}</td>
                        <td>{m.provider}</td>
                        <td>{m.callCount.toLocaleString()}</td>
                        <td>${m.costUsd.toFixed(4)}</td>
                        <td>
                          <div style={{ display: "flex", alignItems: "center", gap: 8 }}>
                            <div style={{ flex: 1, height: 4, background: "var(--bg-tertiary)", borderRadius: 2 }}>
                              <div style={{ height: "100%", width: `${percent}%`, background: "var(--accent)", borderRadius: 2 }} />
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

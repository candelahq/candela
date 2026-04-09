"use client";

import { useCosts } from "@/hooks/useCosts";
import { AreaChart } from "@/components/chart";
import { TimeRangeSelector } from "@/components/TimeRangeSelector";
import { ErrorBanner } from "@/components/ErrorBanner";
import { SkeletonCard } from "@/components/SkeletonCard";


// ──────────────────────────────────────────
// Page
// ──────────────────────────────────────────

export default function CostsPage() {
  const { summary, models, loading, error, timeRange, setTimeRange, refresh } =
    useCosts();

  const maxCost = Math.max(...models.map((m) => m.costUsd), 0.001);

  return (
    <>
      <header className="main-header">
        <div>
          <h1>Costs</h1>
          <span className="mono" style={{ color: "var(--text-muted)", fontSize: 12 }}>
            Token usage &amp; spending
          </span>
        </div>
        <div style={{ display: "flex", gap: 8, alignItems: "center" }}>
          <TimeRangeSelector value={timeRange} onChange={setTimeRange} />
          <button className="btn" onClick={refresh}>
            🔄
          </button>
        </div>
      </header>

      <div className="main-body">
        {error && (
          <ErrorBanner title="Could not load cost data">
            {error}
          </ErrorBanner>
        )}

        {/* Summary cards */}
        {loading && !summary ? (
          <div className="stats-grid animate-in">
            <SkeletonCard />
            <SkeletonCard />
            <SkeletonCard />
            <SkeletonCard />
          </div>
        ) : (
          <div className="stats-grid animate-in">
            <div className="card">
              <div className="card-title">Total Cost</div>
              <div className="card-value">
                {summary ? `$${summary.totalCostUsd.toFixed(4)}` : "—"}
              </div>
              <div className="card-subtitle">
                {timeRange === "24h" ? "Last 24 hours" : timeRange === "7d" ? "Last 7 days" : "Last 30 days"}
              </div>
            </div>
            <div className="card">
              <div className="card-title">Total Requests</div>
              <div className="card-value">
                {summary ? Number(summary.totalTraces).toLocaleString() : "—"}
              </div>
              <div className="card-subtitle">LLM calls</div>
            </div>
            <div className="card">
              <div className="card-title">Input Tokens</div>
              <div className="card-value">
                {summary ? summary.totalInputTokens.toLocaleString() : "—"}
              </div>
              <div className="card-subtitle">Prompt tokens</div>
            </div>
            <div className="card">
              <div className="card-title">Output Tokens</div>
              <div className="card-value">
                {summary ? summary.totalOutputTokens.toLocaleString() : "—"}
              </div>
              <div className="card-subtitle">Completion tokens</div>
            </div>
          </div>
        )}

        {/* Charts */}
        <div className="chart-grid animate-in" style={{ animationDelay: "0.05s" }}>
          <div className="chart-card">
            <div className="chart-card-header">
              <span className="chart-card-title">Cost Over Time</span>
              {summary && (
                <span className="mono" style={{ fontSize: 12, color: "var(--text-muted)" }}>
                  ${summary.totalCostUsd.toFixed(4)} total
                </span>
              )}
            </div>
            <AreaChart
              data={summary?.costOverTime ?? []}
              height={220}
              color="var(--success)"
              formatValue={(v) => `$${v.toFixed(4)}`}
              emptyMessage="No cost data for this period"
            />
          </div>

          <div className="chart-card">
            <div className="chart-card-header">
              <span className="chart-card-title">Tokens Over Time</span>
              {summary && (
                <span className="mono" style={{ fontSize: 12, color: "var(--text-muted)" }}>
                  {(summary.totalInputTokens + summary.totalOutputTokens).toLocaleString()} total
                </span>
              )}
            </div>
            <AreaChart
              data={summary?.tokensOverTime ?? []}
              height={220}
              color="var(--info)"
              formatValue={(v) => v >= 1000 ? `${(v / 1000).toFixed(1)}k` : Math.round(v).toString()}
              emptyMessage="No token data for this period"
            />
          </div>
        </div>

        {/* Model breakdown */}
        <div className="table-container animate-in" style={{ animationDelay: "0.1s" }}>
          <div className="table-header">
            <span className="table-title">Cost by Model</span>
            <span className="mono" style={{ fontSize: 12, color: "var(--text-muted)" }}>
              {models.length} model{models.length !== 1 ? "s" : ""}
            </span>
          </div>
          {models.length === 0 ? (
            <div className="empty-state">
              <div className="empty-state-icon">💰</div>
              <div className="empty-state-title">No cost data yet</div>
              <div className="empty-state-desc">
                Cost breakdowns will appear here once traces start flowing
                through the proxy.
              </div>
            </div>
          ) : (
            <table>
              <thead>
                <tr>
                  <th>Model</th>
                  <th>Provider</th>
                  <th>Calls</th>
                  <th>Input Tokens</th>
                  <th>Output Tokens</th>
                  <th>Cost</th>
                  <th>Avg Latency</th>
                  <th style={{ width: 120 }}>Share</th>
                </tr>
              </thead>
              <tbody>
                {[...models]
                  .sort((a, b) => b.costUsd - a.costUsd)
                  .map((m) => (
                    <tr key={`${m.model}-${m.provider}`}>
                      <td>
                        <span className="mono" style={{ fontSize: 12 }}>
                          {m.model}
                        </span>
                      </td>
                      <td>{m.provider}</td>
                      <td>{m.callCount.toLocaleString()}</td>
                      <td>{m.inputTokens.toLocaleString()}</td>
                      <td>{m.outputTokens.toLocaleString()}</td>
                      <td style={{ fontWeight: 600 }}>
                        ${m.costUsd.toFixed(4)}
                      </td>
                      <td>{m.avgLatencyMs.toFixed(0)}ms</td>
                      <td>
                        <div
                          className="model-bar"
                          style={{
                            width: `${(m.costUsd / maxCost) * 100}%`,
                          }}
                        />
                      </td>
                    </tr>
                  ))}
              </tbody>
            </table>
          )}
        </div>
      </div>
    </>
  );
}

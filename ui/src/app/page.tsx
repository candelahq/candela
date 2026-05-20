"use client";

import Link from "next/link";
import { useDashboard } from "@/hooks/useDashboard";
import { AreaChart } from "@/components/chart";
import { TimeRangeSelector } from "@/components/TimeRangeSelector";
import { ScopeToggle } from "@/components/ScopeToggle";
import { useScope } from "@/components/UserScopeProvider";
import { ErrorBanner } from "@/components/ErrorBanner";
import { SkeletonCard } from "@/components/SkeletonCard";
import { SpanStatus } from "@/gen/candela/types/trace_pb";

// ──────────────────────────────────────────
// Status helpers
// ──────────────────────────────────────────

const statusLabel = (s: number) => {
  if (s === SpanStatus.ERROR) return { text: "error", cls: "badge-error" };
  return { text: "ok", cls: "badge-success" };
};

// ──────────────────────────────────────────
// Page
// ──────────────────────────────────────────

export default function DashboardPage() {
  const { includeBudget } = useScope();
  const {
    summary,
    recentTraces,
    loading,
    error,
    timeRange,
    setTimeRange,
    refresh,
  } = useDashboard({ includeBudget });

  const totalTokens = summary
    ? (summary.totalInputTokens + summary.totalOutputTokens).toLocaleString()
    : "—";

  return (
    <>
      <header className="main-header">
        <h1>Dashboard</h1>
        <div style={{ display: "flex", gap: 8, alignItems: "center" }}>
          <ScopeToggle />
          <TimeRangeSelector value={timeRange} onChange={setTimeRange} />
          <button className="btn" onClick={refresh}>
            🔄
          </button>
        </div>
      </header>

      <div className="main-body">
        {error && (
          <ErrorBanner title="Dashboard Error">
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
              <div className="card-title">Total Traces</div>
              <div className="card-value">
                {summary ? Number(summary.totalTraces).toLocaleString() : "—"}
              </div>
              <div className="card-subtitle">
                {timeRange === "24h" ? "Last 24 hours" : timeRange === "7d" ? "Last 7 days" : "Last 30 days"}
              </div>
            </div>
            <div className="card">
              <div className="card-title">Total Tokens</div>
              <div className="card-value">{totalTokens}</div>
              <div className="card-subtitle">Input + Output</div>
            </div>
            <div className="card">
              <div className="card-title">Cached Tokens</div>
              <div className="card-value">
                {summary ? (summary.totalCacheReadTokens || 0).toLocaleString() : "—"}
              </div>
              <div className="card-subtitle">
                {summary && summary.totalInputTokens > 0
                  ? `${((summary.totalCacheReadTokens / summary.totalInputTokens) * 100).toFixed(1)}% cache hit rate`
                  : "Cache read tokens"}
              </div>
            </div>
            <div className="card">
              <div className="card-title">Total Cost</div>
              <div className="card-value">
                {summary ? `$${(summary.totalCostUsd || 0).toFixed(2)}` : "—"}
              </div>
              <div className="card-subtitle">Estimated USD</div>
            </div>
            <div className="card">
              <div className="card-title">Avg Latency</div>
              <div className="card-value">
                {summary ? `${(summary.avgLatencyMs || 0).toFixed(0)}ms` : "—"}
              </div>
              <div className="card-subtitle">Across all models</div>
            </div>
          </div>
        )}

        {/* Charts */}
        <div className="chart-grid animate-in" style={{ animationDelay: "0.05s" }}>
          <div className="chart-card">
            <div className="chart-card-header">
              <span className="chart-card-title">Traces</span>
            </div>
            <AreaChart
              data={summary?.tracesOverTime ?? []}
              color="var(--accent)"
              formatValue={(v) => Math.round(v).toString()}
              emptyMessage="No trace data for this period"
            />
          </div>

          <div className="chart-card">
            <div className="chart-card-header">
              <span className="chart-card-title">Cost (USD)</span>
            </div>
            <AreaChart
              data={summary?.costOverTime ?? []}
              color="var(--success)"
              formatValue={(v) => `$${v.toFixed(4)}`}
              emptyMessage="No cost data for this period"
            />
          </div>

          <div className="chart-card">
            <div className="chart-card-header">
              <span className="chart-card-title">Tokens</span>
            </div>
            <AreaChart
              data={summary?.tokensOverTime ?? []}
              color="var(--info)"
              formatValue={(v) => v >= 1000 ? `${(v / 1000).toFixed(1)}k` : Math.round(v).toString()}
              emptyMessage="No token data for this period"
            />
          </div>
        </div>

        {/* Job Leaderboard */}
        <div className="table-container animate-in" style={{ animationDelay: "0.08s", marginBottom: 24 }}>
          <div className="table-header">
            <span className="table-title">Top Experiments (Job Leaderboard)</span>
          </div>
          {!summary || summary.jobLeaderboard.length === 0 ? (
            <div className="empty-state" style={{ minHeight: 120 }}>
              <div className="empty-state-title">No job data yet</div>
            </div>
          ) : (
            <table>
              <thead>
                <tr>
                  <th>Job ID</th>
                  <th>Requests</th>
                  <th>Tokens</th>
                  <th>Total Cost</th>
                  <th style={{ width: 120 }}>Cost Distribution</th>
                </tr>
              </thead>
              <tbody>
                {summary.jobLeaderboard.map((j) => {
                  const percent = summary.totalCostUsd > 0 ? (j.costUsd / summary.totalCostUsd) * 100 : 0;
                  return (
                    <tr key={j.jobId}>
                      <td className="mono">{j.jobId}</td>
                      <td>{j.callCount.toLocaleString()}</td>
                      <td>{j.totalTokens.toLocaleString()}</td>
                      <td>${j.costUsd.toFixed(4)}</td>
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
                })}
              </tbody>
            </table>
          )}
        </div>

        {/* Recent Traces */}
        <div className="table-container animate-in" style={{ animationDelay: "0.1s" }}>
          <div className="table-header">
            <span className="table-title">Recent Traces</span>
            <Link href="/traces" className="btn">
              View all →
            </Link>
          </div>
          {recentTraces.length === 0 ? (
            <div className="empty-state">
              <div className="empty-state-icon">🕯️</div>
              <div className="empty-state-title">No traces yet</div>
              <div className="empty-state-desc">
                Send your first LLM request through the Candela proxy to see traces
                appear here. Configure a provider in your candela.yaml and point your
                SDK to <code className="mono">http://localhost:8181/proxy/</code>.
              </div>
            </div>
          ) : (
            <table>
              <thead>
                <tr>
                  <th>Trace ID</th>
                  <th>Operation</th>
                  <th>Model</th>
                  <th>Tokens</th>
                  <th>Cost</th>
                  <th>Latency</th>
                  <th>Status</th>
                  <th>Time</th>
                </tr>
              </thead>
              <tbody>
                {recentTraces.map((t) => {
                  const st = statusLabel(t.status);
                  return (
                    <tr key={t.traceId}>
                      <td>
                        <Link href={`/traces/${t.traceId}`} className="mono">
                          {t.traceId.slice(0, 12)}…
                        </Link>
                      </td>
                      <td>{t.rootSpanName}</td>
                      <td>
                        <span className="mono" style={{ fontSize: 12 }}>
                          {t.primaryModel}
                        </span>
                      </td>
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

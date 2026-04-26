"use client";

import { useTodayBudget, type TodayModelUsage } from "@/hooks/useTodayBudget";
import { ErrorBanner } from "@/components/ErrorBanner";
import { useEffect, useState } from "react";

/** Format a number with k/M suffixes for large token counts. */
function fmtTokens(n: number): string {
  if (n >= 1_000_000) return `${(n / 1_000_000).toFixed(1)}M`;
  if (n >= 1_000) return `${(n / 1_000).toFixed(1)}k`;
  return n.toLocaleString();
}

/** Mini radial gauge for the hero section. */
function BudgetRing({ percent, spent, limit, remaining }: {
  percent: number;
  spent: number;
  limit: number;
  remaining: number;
}) {
  const [offset, setOffset] = useState(2 * Math.PI * 45);
  const r = 45;
  const circumference = 2 * Math.PI * r;

  useEffect(() => {
    const progress = Math.min(percent, 100) / 100;
    const t = setTimeout(() => setOffset(circumference - progress * circumference), 80);
    return () => clearTimeout(t);
  }, [percent, circumference]);

  const color =
    percent >= 100 ? "var(--error)" :
    percent >= 80 ? "var(--warning)" :
    "var(--accent)";

  return (
    <div className="today-ring-wrap">
      <svg viewBox="0 0 120 120" className="today-ring-svg">
        {/* Subtle glow */}
        <defs>
          <filter id="glow">
            <feGaussianBlur stdDeviation="3" result="blur" />
            <feMerge>
              <feMergeNode in="blur" />
              <feMergeNode in="SourceGraphic" />
            </feMerge>
          </filter>
        </defs>
        {/* Track */}
        <circle cx="60" cy="60" r={r} fill="none" stroke="var(--bg-tertiary)" strokeWidth="10" />
        {/* Progress */}
        <circle
          cx="60" cy="60" r={r}
          fill="none"
          stroke={color}
          strokeWidth="10"
          strokeDasharray={circumference}
          strokeDashoffset={offset}
          strokeLinecap="round"
          filter="url(#glow)"
          style={{ transition: "stroke-dashoffset 1.8s cubic-bezier(0.4, 0, 0.2, 1), stroke 0.3s" }}
          transform="rotate(-90 60 60)"
        />
        <text x="60" y="55" textAnchor="middle" className="today-ring-pct" style={{ fill: color }}>
          {Math.round(percent)}%
        </text>
        <text x="60" y="70" textAnchor="middle" className="today-ring-label">
          used
        </text>
      </svg>

      <div className="today-ring-meta">
        <div className="today-ring-stat">
          <span className="today-ring-stat-label">Spent</span>
          <span className="today-ring-stat-value" style={{ color }}>${spent.toFixed(2)}</span>
        </div>
        <div className="today-ring-divider" />
        <div className="today-ring-stat">
          <span className="today-ring-stat-label">Limit</span>
          <span className="today-ring-stat-value">${limit.toFixed(2)}</span>
        </div>
        <div className="today-ring-divider" />
        <div className="today-ring-stat">
          <span className="today-ring-stat-label">Left</span>
          <span className="today-ring-stat-value" style={{ color: remaining <= 0 ? "var(--error)" : "var(--success)" }}>
            ${Math.max(0, remaining).toFixed(2)}
          </span>
        </div>
      </div>
    </div>
  );
}

/** Horizontal stacked bar for a single model's token split. */
function TokenBar({ model }: { model: TodayModelUsage }) {
  const total = model.inputTokens + model.outputTokens;
  const inPct = total > 0 ? (model.inputTokens / total) * 100 : 0;

  return (
    <div className="today-model-row">
      <div className="today-model-info">
        <span className="today-model-name mono">{model.model}</span>
        <span className="today-model-provider">{model.provider}</span>
      </div>
      <div className="today-model-metrics">
        <span className="today-model-metric">
          <span className="today-model-metric-label">Requests</span>
          <span className="today-model-metric-value">{model.callCount.toLocaleString()}</span>
        </span>
        <span className="today-model-metric">
          <span className="today-model-metric-label">In</span>
          <span className="today-model-metric-value">{fmtTokens(model.inputTokens)}</span>
        </span>
        <span className="today-model-metric">
          <span className="today-model-metric-label">Out</span>
          <span className="today-model-metric-value">{fmtTokens(model.outputTokens)}</span>
        </span>
        <span className="today-model-metric">
          <span className="today-model-metric-label">Cost</span>
          <span className="today-model-metric-value">${model.costUsd.toFixed(4)}</span>
        </span>
      </div>
      <div className="today-token-bar">
        <div className="today-token-bar-in" style={{ width: `${inPct}%` }} />
        <div className="today-token-bar-out" style={{ width: `${100 - inPct}%` }} />
      </div>
    </div>
  );
}

export default function TodayPage() {
  const { data, loading, error, refresh } = useTodayBudget();

  const totalTokens = data
    ? (data.totalInputTokens + data.totalOutputTokens)
    : 0;

  const todayStr = new Date().toLocaleDateString(undefined, {
    weekday: "long",
    month: "long",
    day: "numeric",
    year: "numeric",
  });

  return (
    <>
      <header className="main-header">
        <div>
          <h1>Today</h1>
          <span className="today-date">{todayStr}</span>
        </div>
        <div style={{ display: "flex", gap: 8, alignItems: "center" }}>
          {data?.fetchedAt && (
            <span className="today-updated">
              Updated {data.fetchedAt.toLocaleTimeString()}
            </span>
          )}
          <button className="btn" onClick={refresh} title="Refresh" aria-label="Refresh data">
            <span role="img" aria-hidden="true">🔄</span>
          </button>
        </div>
      </header>

      <div className="main-body">
        {error && (
          <ErrorBanner title="Connection Error">
            Failed to fetch today&apos;s budget data: {error}
          </ErrorBanner>
        )}

        {/* Hero Budget Ring */}
        {loading && !data ? (
          <div className="today-hero-skeleton animate-in">
            <div className="skeleton" style={{ width: 160, height: 160, borderRadius: "50%" }} />
            <div style={{ display: "flex", gap: 32, marginTop: 16 }}>
              <div className="skeleton" style={{ width: 80, height: 40 }} />
              <div className="skeleton" style={{ width: 80, height: 40 }} />
              <div className="skeleton" style={{ width: 80, height: 40 }} />
            </div>
          </div>
        ) : data?.budget ? (
          <div className="today-hero animate-in">
            <BudgetRing
              percent={data.budget.percentUsed}
              spent={data.budget.spentUsd}
              limit={data.budget.limitUsd}
              remaining={data.budget.remainingUsd}
            />
            {data.budget.percentUsed >= 90 && (
              <div className="today-hero-alert">
                {data.budget.percentUsed >= 100
                  ? "🚫 Daily budget exhausted. Requests may be blocked until midnight UTC."
                  : "⚠️ Approaching daily budget limit."}
              </div>
            )}
          </div>
        ) : !loading ? (
          <div className="today-hero animate-in">
            <div className="today-no-budget">
              <span className="today-no-budget-icon">🕯️</span>
              <span className="today-no-budget-title">No daily budget configured</span>
              <span className="today-no-budget-desc">
                An admin can set your daily spending limit from the Budgets page.
              </span>
            </div>
          </div>
        ) : null}

        {/* Quick Stats */}
        <div className="stats-grid animate-in" style={{ animationDelay: "0.08s", marginTop: 24 }}>
          <div className="card today-stat-card">
            <div className="card-title">Requests</div>
            <div className="card-value">{data ? data.totalCalls.toLocaleString() : "—"}</div>
            <div className="card-subtitle">API calls today</div>
          </div>
          <div className="card today-stat-card">
            <div className="card-title">Tokens</div>
            <div className="card-value">{loading && !data ? "—" : fmtTokens(totalTokens)}</div>
            <div className="card-subtitle">
              {data ? `${fmtTokens(data.totalInputTokens)} in · ${fmtTokens(data.totalOutputTokens)} out` : "In + Out"}
            </div>
          </div>
          <div className="card today-stat-card">
            <div className="card-title">Cost</div>
            <div className="card-value">{data ? `$${data.totalCostUsd.toFixed(4)}` : "—"}</div>
            <div className="card-subtitle">Estimated USD today</div>
          </div>
          <div className="card today-stat-card">
            <div className="card-title">Avg Latency</div>
            <div className="card-value">{data ? `${data.avgLatencyMs.toFixed(0)}ms` : "—"}</div>
            <div className="card-subtitle">Across all models</div>
          </div>
        </div>

        {/* Per-Model Token Breakdown */}
        <div className="table-container animate-in" style={{ animationDelay: "0.15s", marginTop: 24 }}>
          <div className="table-header">
            <span className="table-title">Token Spend by Model</span>
            <div className="today-token-legend">
              <span className="today-legend-item"><span className="today-legend-dot today-legend-in" />Input</span>
              <span className="today-legend-item"><span className="today-legend-dot today-legend-out" />Output</span>
            </div>
          </div>

          {!loading && data?.models.length === 0 ? (
            <div className="empty-state">
              <div className="empty-state-icon">🤖</div>
              <div className="empty-state-title">No activity yet today</div>
              <div className="empty-state-desc">
                Send requests through the Candela proxy and your per-model token usage will appear here in real time.
              </div>
            </div>
          ) : loading && !data ? (
            <div style={{ padding: 20 }}>
              {[1,2,3].map((i) => (
                <div key={i} className="skeleton" style={{ height: 56, marginBottom: 8, borderRadius: "var(--radius-md)" }} />
              ))}
            </div>
          ) : (
            <div className="today-model-list">
              {[...(data?.models ?? [])]
                .sort((a, b) => b.costUsd - a.costUsd)
                .map((m) => <TokenBar key={`${m.provider}-${m.model}`} model={m} />)}
            </div>
          )}
        </div>
      </div>

      <style jsx>{`
        /* ── Hero Budget Section ── */
        .today-hero {
          background: linear-gradient(135deg, var(--bg-secondary) 0%, var(--bg-tertiary) 50%, var(--bg-secondary) 100%);
          border: 1px solid var(--border-subtle);
          border-radius: var(--radius-lg);
          padding: 32px;
          display: flex;
          flex-direction: column;
          align-items: center;
        }
        .today-hero-skeleton {
          display: flex;
          flex-direction: column;
          align-items: center;
          padding: 32px;
          background: var(--bg-secondary);
          border: 1px solid var(--border-subtle);
          border-radius: var(--radius-lg);
        }
        .today-hero-alert {
          margin-top: 20px;
          padding: 10px 16px;
          border-radius: var(--radius-md);
          font-size: 13px;
          font-weight: 500;
          animation: pulse 2s ease-in-out infinite;
          background: rgba(248, 113, 113, 0.1);
          border: 1px solid var(--error);
          color: var(--error);
        }
        @keyframes pulse {
          0%, 100% { opacity: 1; }
          50% { opacity: 0.7; }
        }

        /* ── Ring Gauge ── */
        .today-ring-wrap {
          display: flex;
          flex-direction: column;
          align-items: center;
          gap: 20px;
        }
        .today-ring-svg {
          width: 180px;
          height: 180px;
          filter: drop-shadow(0 0 20px rgba(240, 160, 48, 0.15));
        }
        .today-ring-pct {
          font-weight: 800;
          font-size: 22px;
          letter-spacing: -0.02em;
        }
        .today-ring-label {
          font-size: 10px;
          fill: var(--text-muted);
          text-transform: uppercase;
          letter-spacing: 0.1em;
          font-weight: 600;
        }
        .today-ring-meta {
          display: flex;
          gap: 24px;
          align-items: center;
        }
        .today-ring-stat {
          display: flex;
          flex-direction: column;
          align-items: center;
          gap: 4px;
        }
        .today-ring-stat-label {
          font-size: 10px;
          font-weight: 600;
          text-transform: uppercase;
          letter-spacing: 0.06em;
          color: var(--text-muted);
        }
        .today-ring-stat-value {
          font-size: 22px;
          font-weight: 700;
          letter-spacing: -0.02em;
        }
        .today-ring-divider {
          width: 1px;
          height: 32px;
          background: var(--border);
        }

        /* ── Date & Updated ── */
        .today-date {
          font-size: 12px;
          color: var(--text-muted);
          margin-left: 12px;
          font-weight: 400;
        }
        .today-updated {
          font-size: 11px;
          color: var(--text-muted);
          padding: 4px 10px;
          background: var(--bg-tertiary);
          border-radius: var(--radius-sm);
        }

        /* ── No Budget ── */
        .today-no-budget {
          display: flex;
          flex-direction: column;
          align-items: center;
          gap: 8px;
          padding: 24px;
        }
        .today-no-budget-icon {
          font-size: 40px;
          opacity: 0.5;
        }
        .today-no-budget-title {
          font-size: 16px;
          font-weight: 600;
        }
        .today-no-budget-desc {
          font-size: 13px;
          color: var(--text-muted);
          text-align: center;
          max-width: 320px;
        }

        /* ── Stat Cards ── */
        .today-stat-card {
          background: linear-gradient(135deg, var(--bg-secondary) 0%, var(--bg-tertiary) 100%);
          position: relative;
          overflow: hidden;
        }
        .today-stat-card::before {
          content: '';
          position: absolute;
          top: 0;
          left: 0;
          right: 0;
          height: 2px;
          background: linear-gradient(90deg, var(--accent), transparent);
          opacity: 0;
          transition: opacity 0.3s;
        }
        .today-stat-card:hover::before {
          opacity: 1;
        }

        /* ── Model Token Breakdown ── */
        .today-model-list {
          padding: 8px 0;
        }
        .today-model-row {
          padding: 12px 20px;
          border-bottom: 1px solid var(--border-subtle);
          transition: background 0.15s;
        }
        .today-model-row:last-child {
          border-bottom: none;
        }
        .today-model-row:hover {
          background: var(--bg-hover);
        }
        .today-model-info {
          display: flex;
          align-items: baseline;
          gap: 8px;
          margin-bottom: 6px;
        }
        .today-model-name {
          font-size: 13px;
          font-weight: 600;
          color: var(--text-primary);
        }
        .today-model-provider {
          font-size: 11px;
          color: var(--text-muted);
          padding: 1px 6px;
          background: var(--bg-tertiary);
          border-radius: 4px;
        }
        .today-model-metrics {
          display: flex;
          gap: 20px;
          margin-bottom: 8px;
        }
        .today-model-metric {
          display: flex;
          flex-direction: column;
          gap: 1px;
        }
        .today-model-metric-label {
          font-size: 10px;
          font-weight: 600;
          text-transform: uppercase;
          letter-spacing: 0.04em;
          color: var(--text-muted);
        }
        .today-model-metric-value {
          font-size: 13px;
          font-weight: 600;
          color: var(--text-primary);
          font-variant-numeric: tabular-nums;
        }

        /* ── Token bar ── */
        .today-token-bar {
          display: flex;
          height: 6px;
          border-radius: 3px;
          overflow: hidden;
          background: var(--bg-tertiary);
        }
        .today-token-bar-in {
          background: var(--info);
          transition: width 0.6s ease;
        }
        .today-token-bar-out {
          background: var(--accent);
          transition: width 0.6s ease;
        }

        /* ── Legend ── */
        .today-token-legend {
          display: flex;
          gap: 16px;
          align-items: center;
        }
        .today-legend-item {
          display: flex;
          align-items: center;
          gap: 6px;
          font-size: 11px;
          color: var(--text-muted);
          font-weight: 500;
        }
        .today-legend-dot {
          width: 8px;
          height: 8px;
          border-radius: 2px;
        }
        .today-legend-in {
          background: var(--info);
        }
        .today-legend-out {
          background: var(--accent);
        }

        /* ── Skeletons ── */
        .skeleton {
          background: linear-gradient(90deg, var(--bg-tertiary) 25%, var(--bg-elevated) 50%, var(--bg-tertiary) 75%);
          background-size: 200% 100%;
          animation: shimmer 1.5s infinite;
          border-radius: var(--radius-md);
        }
        @keyframes shimmer {
          0% { background-position: 200% 0; }
          100% { background-position: -200% 0; }
        }
      `}</style>
    </>
  );
}

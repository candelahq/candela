"use client";

import { useTodayBudget } from "@/hooks/useTodayBudget";
import { useForecast } from "@/hooks/useForecast";
import { ErrorBanner } from "@/components/ErrorBanner";
import { useEffect, useMemo, useState } from "react";
import { BudgetRing } from "@/components/today/BudgetRing";
import { TokenBar } from "@/components/today/TokenBar";
import { GrantCard } from "@/components/today/GrantCard";
import { ForecastBar } from "@/components/today/ForecastBar";
import { fmtTokens } from "@/components/today/utils";
import "./today.css";

export default function TodayPage() {
  const { data, loading, error, refresh } = useTodayBudget();
  const { forecast } = useForecast();

  const totalTokens = data
    ? (data.totalInputTokens + data.totalOutputTokens)
    : 0;

  // Live UTC clock — ticks every 30s so users know what "today UTC" means.
  // tick=0 during SSR; the first interval callback bumps it to 1, which also
  // serves as the "mounted" signal (avoiding synchronous setState in effect body).
  const [tick, setTick] = useState(0);
  useEffect(() => {
    // Fire immediately via setTimeout(0) so the first tick is async (not
    // synchronous in the effect body), satisfying the ESLint rule.
    const immediate = setTimeout(() => setTick(1), 0);
    const timer = setInterval(() => setTick((t) => t + 1), 30_000);
    return () => { clearTimeout(immediate); clearInterval(timer); };
  }, []);

  // Use UTC to match the budget reset boundary (midnight UTC).
  const todayStr = tick > 0
    ? new Date().toLocaleDateString(undefined, {
        weekday: "long",
        month: "long",
        day: "numeric",
        year: "numeric",
        timeZone: "UTC",
      })
    : "";

  const utcTimeStr = tick > 0
    ? new Date().toLocaleTimeString(undefined, {
        hour: "2-digit",
        minute: "2-digit",
        timeZone: "UTC",
        hour12: false,
      })
    : "--:--";

  const sortedModels = useMemo(
    () => [...(data?.models ?? [])].sort((a, b) => b.costUsd - a.costUsd),
    [data?.models],
  );

  return (
    <>
      <header className="main-header">
        <div>
          <h1>Today <span className="today-utc-badge">UTC</span></h1>
          <span className="today-date">{todayStr}</span>
          <span className="today-utc-clock">
            {utcTimeStr} UTC · {data?.periodResetsAt
              ? `Resets ${new Date(data.periodResetsAt).toLocaleTimeString(undefined, { hour: "2-digit", minute: "2-digit", hour12: true })} local time`
              : "Resets at midnight UTC"}
          </span>
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

        {/* Forecast: burn rate, EOD projection, exhaustion warning, sparkline */}
        {forecast && <ForecastBar forecast={forecast} />}

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

        {/* Active Grants */}
        {!loading && data && data.grants.length > 0 && (
          <div className="table-container animate-in" style={{ animationDelay: "0.12s", marginTop: 24 }}>
            <div className="table-header">
              <span className="table-title">Active Grants</span>
              <span className="today-grant-count">{data.grants.length} active</span>
            </div>
            <div className="today-grant-list">
              {data.grants.map((g) => (
                <GrantCard key={g.id} grant={g} />
              ))}
            </div>
          </div>
        )}

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
              {sortedModels.map((m) => (
                <TokenBar key={`${m.provider}-${m.model}`} model={m} />
              ))}
            </div>
          )}
        </div>
      </div>

    </>
  );
}

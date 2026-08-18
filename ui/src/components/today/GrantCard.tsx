"use client";

import type { TodayGrant } from "@/hooks/useTodayBudget";

export function GrantCard({ grant }: { grant: TodayGrant }) {
  const pct = Math.min(grant.percentUsed, 100);
  const color =
    pct >= 100 ? "var(--error)" :
    pct >= 80 ? "var(--warning)" :
    "var(--accent)";

  return (
    <div className="today-grant-card">
      <div className="today-grant-header">
        <span className="today-grant-reason">{grant.reason || "Grant"}</span>
        {grant.expiresAt && (
          <span className="today-grant-expiry">
            Expires {grant.expiresAt.toLocaleDateString(undefined, { month: "short", day: "numeric" })}
          </span>
        )}
      </div>
      <div className="today-grant-bar-wrap" role="progressbar"
           aria-valuenow={pct} aria-valuemin={0} aria-valuemax={100}
           aria-label={`${grant.reason || "Grant"}: ${pct.toFixed(0)}% used`}>
        <div className="today-grant-bar-fill" style={{ width: `${pct}%`, background: color }} />
      </div>
      <div className="today-grant-stats">
        <span className="today-grant-stat">
          <span className="today-grant-stat-label">Used</span>
          <span className="today-grant-stat-value">${grant.spentUsd.toFixed(2)}</span>
        </span>
        <span className="today-grant-stat">
          <span className="today-grant-stat-label">Left</span>
          <span className="today-grant-stat-value" style={{ color: grant.remainingUsd <= 0 ? "var(--error)" : "var(--success)" }}>
            ${Math.max(0, grant.remainingUsd).toFixed(2)}
          </span>
        </span>
        <span className="today-grant-stat">
          <span className="today-grant-stat-label">Total</span>
          <span className="today-grant-stat-value">${grant.amountUsd.toFixed(2)}</span>
        </span>
        {grant.grantedBy && (
          <span className="today-grant-stat">
            <span className="today-grant-stat-label">By</span>
            <span className="today-grant-stat-value today-grant-by">{grant.grantedBy.split("@")[0]}</span>
          </span>
        )}
      </div>
    </div>
  );
}

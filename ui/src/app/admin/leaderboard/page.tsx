"use client";

import { useLeaderboard } from "@/hooks/useLeaderboard";
import { TimeRangeSelector } from "@/components/TimeRangeSelector";
import { ErrorBanner } from "@/components/ErrorBanner";

export default function LeaderboardPage() {
  const { rankings, loading, error, timeRange, setTimeRange, refresh } = useLeaderboard();

  return (
    <>
      <header className="main-header">
        <div style={{ display: "flex", alignItems: "center", gap: 12 }}>
          <h1>Team Leaderboard</h1>
          <span className="badge badge-info">Admin Only</span>
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
          <ErrorBanner title="Leaderboard Error">
            Could not fetch team data: {error}
          </ErrorBanner>
        )}

        <div className="leaderboard-summary stats-grid animate-in">
          <div className="card">
            <div className="card-title">Top Spender</div>
            <div className="card-value">
              {rankings.length > 0 ? rankings[0].displayName : "—"}
            </div>
            <div className="card-subtitle">
              {rankings.length > 0 ? `$${rankings[0].costUsd.toFixed(2)} total` : "No data"}
            </div>
          </div>
          <div className="card">
            <div className="card-title">Active Users</div>
            <div className="card-value">{rankings.length}</div>
            <div className="card-subtitle">Making requests in this period</div>
          </div>
          <div className="card">
            <div className="card-title">Avg Cost / Call</div>
            <div className="card-value">
              ${rankings.length > 0 && rankings.reduce((acc, curr) => acc + curr.callCount, 0) > 0
                ? (rankings.reduce((acc, curr) => acc + curr.costUsd, 0) / rankings.reduce((acc, curr) => acc + curr.callCount, 0)).toFixed(4)
                : "0.0000"
              }
            </div>
            <div className="card-subtitle">Team average</div>
          </div>
        </div>

        <div className="table-container animate-in" style={{ animationDelay: "0.1s", marginTop: 24 }}>
          <div className="table-header">
            <span className="table-title">Rankings by Cost</span>
          </div>
          {rankings.length === 0 && !loading ? (
            <div className="empty-state">
              <div className="empty-state-icon">🏆</div>
              <div className="empty-state-title">No rankings yet</div>
              <div className="empty-state-desc">
                When team members start making requests through the proxy, they will appear here ranked by total cost.
              </div>
            </div>
          ) : (
            <table>
              <thead>
                <tr>
                  <th style={{ width: 60 }}>Rank</th>
                  <th>User</th>
                  <th>Requests</th>
                  <th>Tokens</th>
                  <th>Cost (USD)</th>
                  <th>Avg Latency</th>
                  <th>Top Model</th>
                </tr>
              </thead>
              <tbody>
                {loading ? (
                  Array.from({ length: 5 }).map((_, i) => (
                    <tr key={i}>
                      <td colSpan={7} style={{ height: 48 }}><div className="skeleton" style={{ height: 16, width: "100%" }} /></td>
                    </tr>
                  ))
                ) : (
                  rankings.map((u, i) => (
                    <tr key={u.userId}>
                      <td style={{ fontWeight: 600, color: i === 0 ? "var(--accent)" : "var(--text-muted)" }}>
                        #{i + 1}
                      </td>
                      <td>
                        <div style={{ display: "flex", flexDirection: "column" }}>
                          <span style={{ fontWeight: 600 }}>{u.displayName}</span>
                          <span style={{ fontSize: 11, color: "var(--text-muted)" }}>{u.email}</span>
                        </div>
                      </td>
                      <td>{u.callCount.toLocaleString()}</td>
                      <td>{(u.totalTokens / 1000).toFixed(1)}k</td>
                      <td style={{ fontWeight: 600 }}>${u.costUsd.toFixed(2)}</td>
                      <td>{u.avgLatencyMs.toFixed(0)}ms</td>
                      <td>
                        <span className="badge badge-success" style={{ fontSize: 10 }}>{u.topModel}</span>
                      </td>
                    </tr>
                  ))
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

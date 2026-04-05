"use client";

import Link from "next/link";
import { useDashboard } from "@/hooks/useDashboard";

export default function DashboardPage() {
  const { summary, error } = useDashboard();

  const totalTokens = summary
    ? (summary.totalInputTokens + summary.totalOutputTokens).toLocaleString()
    : "—";

  return (
    <>
      <header className="main-header">
        <h1>Dashboard</h1>
        <span className="mono" style={{ color: "var(--text-muted)" }}>
          Overview
        </span>
      </header>

      <div className="main-body">
        {error && (
          <div
            className="card animate-in"
            style={{
              borderColor: "var(--error)",
              marginBottom: 24,
              background: "rgba(248,113,113,0.05)",
            }}
          >
            <div className="card-title" style={{ color: "var(--error)" }}>
              Backend Unavailable
            </div>
            <div style={{ fontSize: 13, color: "var(--text-secondary)" }}>
              Could not connect to Candela backend at{" "}
              <code className="mono">localhost:8181</code>. Start the server
              with <code className="mono">go run ./cmd/candela-server</code>.
            </div>
          </div>
        )}

        <div className="stats-grid animate-in">
          <div className="card">
            <div className="card-title">Total Traces</div>
            <div className="card-value">
              {summary ? Number(summary.totalTraces).toLocaleString() : "—"}
            </div>
            <div className="card-subtitle">All time</div>
          </div>
          <div className="card">
            <div className="card-title">Total Tokens</div>
            <div className="card-value">{totalTokens}</div>
            <div className="card-subtitle">Input + Output</div>
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

        <div className="table-container animate-in" style={{ animationDelay: "0.1s" }}>
          <div className="table-header">
            <span className="table-title">Recent Traces</span>
            <Link href="/traces" className="btn">
              View all →
            </Link>
          </div>
          <div className="empty-state">
            <div className="empty-state-icon">🕯️</div>
            <div className="empty-state-title">No traces yet</div>
            <div className="empty-state-desc">
              Send your first LLM request through the Candela proxy to see traces
              appear here. Configure a provider in your candela.yaml and point your
              SDK to <code className="mono">http://localhost:8181/proxy/</code>.
            </div>
          </div>
        </div>
      </div>
    </>
  );
}

"use client";

import { useEffect, useState } from "react";
import Link from "next/link";
import { createClient } from "@connectrpc/connect";
import { transport } from "@/lib/connect";
import { DashboardService } from "@/gen/v1/dashboard_service_pb";

interface Stats {
  totalTraces: string;
  totalTokens: string;
  totalCost: string;
  avgLatency: string;
  errorRate: string;
}

export default function DashboardPage() {
  const [stats, setStats] = useState<Stats | null>(null);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    const client = createClient(DashboardService, transport);
    client
      .getUsageSummary({})
      .then((res) => {
        const totalTokens = Number(res.totalInputTokens) + Number(res.totalOutputTokens);
        setStats({
          totalTraces: Number(res.totalTraces).toLocaleString(),
          totalTokens: totalTokens.toLocaleString(),
          totalCost: `$${(res.totalCostUsd || 0).toFixed(2)}`,
          avgLatency: `${(res.avgLatencyMs || 0).toFixed(0)}ms`,
          errorRate: `${((res.errorRate || 0) * 100).toFixed(1)}%`,
        });
      })
      .catch((err) => {
        setError(err.message);
      });
  }, []);

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
              <code className="mono">localhost:8080</code>. Start the server
              with <code className="mono">go run ./cmd/candela-server</code>.
            </div>
          </div>
        )}

        <div className="stats-grid animate-in">
          <div className="card">
            <div className="card-title">Total Traces</div>
            <div className="card-value">{stats?.totalTraces ?? "—"}</div>
            <div className="card-subtitle">All time</div>
          </div>
          <div className="card">
            <div className="card-title">Total Tokens</div>
            <div className="card-value">{stats?.totalTokens ?? "—"}</div>
            <div className="card-subtitle">Input + Output</div>
          </div>
          <div className="card">
            <div className="card-title">Total Cost</div>
            <div className="card-value">{stats?.totalCost ?? "—"}</div>
            <div className="card-subtitle">Estimated USD</div>
          </div>
          <div className="card">
            <div className="card-title">Avg Latency</div>
            <div className="card-value">{stats?.avgLatency ?? "—"}</div>
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
              SDK to <code className="mono">http://localhost:8080/proxy/</code>.
            </div>
          </div>
        </div>
      </div>
    </>
  );
}

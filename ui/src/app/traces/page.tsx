"use client";

import { useEffect, useState } from "react";
import { createClient } from "@connectrpc/connect";
import { transport } from "@/lib/connect";
import { TraceService } from "@/gen/v1/trace_service_pb";

interface TraceSummaryRow {
  traceId: string;
  rootSpanName: string;
  primaryModel: string;
  environment: string;
  durationMs: number;
  totalTokens: number;
  totalCostUsd: number;
  status: number;
  startTime: string;
  spanCount: number;
}

export default function TracesPage() {
  const [traces, setTraces] = useState<TraceSummaryRow[]>([]);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);

  useEffect(() => {
    const client = createClient(TraceService, transport);
    client
      .listTraces({ pagination: { pageSize: 50 } })
      .then((res) => {
        const mapped = (res.traces || []).map((t) => {
          const durSeconds = Number(t.duration?.seconds ?? 0);
          const durNanos = Number(t.duration?.nanos ?? 0);
          const durationMs = durSeconds * 1000 + durNanos / 1e6;

          return {
            traceId: t.traceId,
            rootSpanName: t.rootSpanName || "unknown",
            primaryModel: t.primaryModel || "—",
            environment: t.environment || "—",
            durationMs,
            totalTokens: Number(t.totalTokens) || 0,
            totalCostUsd: t.totalCostUsd || 0,
            status: t.status,
            spanCount: t.spanCount || 0,
            startTime: t.startTime
              ? new Date(
                  Number(t.startTime.seconds) * 1000 +
                    Math.floor(Number(t.startTime.nanos) / 1e6)
                ).toLocaleString()
              : "—",
          };
        });
        setTraces(mapped);
      })
      .catch((err) => setError(err.message))
      .finally(() => setLoading(false));
  }, []);

  const statusLabel = (s: number) => {
    if (s === 2) return { text: "error", cls: "badge-error" };
    return { text: "ok", cls: "badge-success" };
  };

  return (
    <>
      <header className="main-header">
        <h1>Traces</h1>
        <div style={{ display: "flex", gap: 8 }}>
          <button className="btn" onClick={() => window.location.reload()}>
            🔄 Refresh
          </button>
        </div>
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
              Could not load traces
            </div>
            <div style={{ fontSize: 13, color: "var(--text-secondary)" }}>
              {error}
            </div>
          </div>
        )}

        <div className="table-container animate-in">
          <div className="table-header">
            <span className="table-title">
              {loading ? "Loading..." : `${traces.length} traces`}
            </span>
          </div>

          {traces.length === 0 && !loading ? (
            <div className="empty-state">
              <div className="empty-state-icon">🔍</div>
              <div className="empty-state-title">No traces found</div>
              <div className="empty-state-desc">
                Traces will appear here once LLM requests flow through the proxy.
              </div>
            </div>
          ) : (
            <table>
              <thead>
                <tr>
                  <th>Trace ID</th>
                  <th>Operation</th>
                  <th>Model</th>
                  <th>Env</th>
                  <th>Spans</th>
                  <th>Tokens</th>
                  <th>Cost</th>
                  <th>Latency</th>
                  <th>Status</th>
                  <th>Time</th>
                </tr>
              </thead>
              <tbody>
                {traces.map((t) => {
                  const st = statusLabel(t.status);
                  return (
                    <tr key={t.traceId} style={{ cursor: "pointer" }}>
                      <td>
                        <span className="mono">{t.traceId.slice(0, 12)}…</span>
                      </td>
                      <td>{t.rootSpanName}</td>
                      <td>
                        <span className="mono" style={{ fontSize: 12 }}>
                          {t.primaryModel}
                        </span>
                      </td>
                      <td>
                        <span className="badge badge-info">{t.environment}</span>
                      </td>
                      <td>{t.spanCount}</td>
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

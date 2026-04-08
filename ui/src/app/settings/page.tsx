"use client";

import { useEffect, useReducer } from "react";
import { dashboardClient } from "@/lib/api";
import { timestampFromDate } from "@bufbuild/protobuf/wkt";

// ──────────────────────────────────────────
// State
// ──────────────────────────────────────────

type HealthStatus = "checking" | "connected" | "offline";

interface SettingsState {
  health: HealthStatus;
  backendVersion: string;
  traceCount: number | null;
  storageBackend: string;
  providers: string[];
}

type Action =
  | { type: "checking" }
  | { type: "connected"; traceCount: number }
  | { type: "offline" };

function reducer(state: SettingsState, action: Action): SettingsState {
  switch (action.type) {
    case "checking":
      return { ...state, health: "checking" };
    case "connected":
      return { ...state, health: "connected", traceCount: action.traceCount };
    case "offline":
      return { ...state, health: "offline" };
  }
}

// ──────────────────────────────────────────
// Page
// ──────────────────────────────────────────

export default function SettingsPage() {
  const [state, dispatch] = useReducer(reducer, {
    health: "checking",
    backendVersion: "v0.1.0",
    traceCount: null,
    storageBackend: "DuckDB",
    providers: ["OpenAI", "Google (Gemini)", "Anthropic"],
  });

  useEffect(() => {
    dispatch({ type: "checking" });

    const now = new Date();
    const start = new Date(now.getTime() - 30 * 24 * 60 * 60 * 1000);

    dashboardClient
      .getUsageSummary({
        timeRange: {
          start: timestampFromDate(start),
          end: timestampFromDate(now),
        },
      })
      .then((res) => {
        dispatch({ type: "connected", traceCount: Number(res.totalTraces) });
      })
      .catch(() => {
        dispatch({ type: "offline" });
      });
  }, []);

  const healthDot =
    state.health === "connected"
      ? "var(--success)"
      : state.health === "offline"
        ? "var(--error)"
        : "var(--warning)";

  const healthLabel =
    state.health === "connected"
      ? "Connected"
      : state.health === "offline"
        ? "Offline"
        : "Checking…";

  return (
    <>
      <header className="main-header">
        <h1>Settings</h1>
      </header>

      <div className="main-body">
        {/* Backend Connection */}
        <div className="card animate-in" style={{ marginBottom: 16 }}>
          <div className="card-title">Backend Connection</div>
          <div className="settings-grid">
            <div className="settings-row">
              <span className="settings-label">Status</span>
              <span className="settings-value">
                <span
                  className="health-dot"
                  style={{ background: healthDot }}
                />
                {healthLabel}
              </span>
            </div>
            <div className="settings-row">
              <span className="settings-label">API URL</span>
              <span className="settings-value mono">http://localhost:8181</span>
            </div>
            <div className="settings-row">
              <span className="settings-label">Protocol</span>
              <span className="settings-value">ConnectRPC v2</span>
            </div>
            <div className="settings-row">
              <span className="settings-label">Version</span>
              <span className="settings-value mono">{state.backendVersion}</span>
            </div>
            {state.traceCount !== null && (
              <div className="settings-row">
                <span className="settings-label">Total Traces (30d)</span>
                <span className="settings-value">
                  {state.traceCount.toLocaleString()}
                </span>
              </div>
            )}
          </div>
        </div>

        {/* Storage */}
        <div
          className="card animate-in"
          style={{ marginBottom: 16, animationDelay: "0.05s" }}
        >
          <div className="card-title">Storage Backend</div>
          <div className="settings-grid">
            <div className="settings-row">
              <span className="settings-label">Primary</span>
              <span className="settings-value">
                <span className="badge badge-info">{state.storageBackend}</span>
              </span>
            </div>
            <div className="settings-row">
              <span className="settings-label">Architecture</span>
              <span className="settings-value">CQRS — Fan-out writes</span>
            </div>
          </div>
        </div>

        {/* Configured Providers */}
        <div
          className="card animate-in"
          style={{ animationDelay: "0.1s" }}
        >
          <div className="card-title">Configured Providers</div>
          <div className="settings-providers">
            {state.providers.map((p) => (
              <div key={p} className="settings-provider-chip">
                <span className="settings-provider-dot" />
                {p}
              </div>
            ))}
          </div>
          <div
            style={{
              fontSize: 12,
              color: "var(--text-muted)",
              marginTop: 12,
            }}
          >
            Provider configuration is managed in{" "}
            <code className="mono">config.yaml</code> on the server.
          </div>
        </div>
      </div>
    </>
  );
}

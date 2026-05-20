"use client";

import { useEffect, useReducer } from "react";
import { dashboardClient } from "@/lib/api";
import { useCurrentUser } from "@/hooks/useCurrentUser";
import { timestampFromDate } from "@bufbuild/protobuf/wkt";
import { API_BASE_URL } from "@/lib/constants";

// ──────────────────────────────────────────
// State
// ──────────────────────────────────────────

type HealthStatus = "checking" | "connected" | "offline";

interface SettingsState {
  health: HealthStatus;
  backendVersion: string;
  traceCount: number | null;
  llmCalls: number | null;
  totalCost: number | null;
  modelCount: number | null;
  storageBackend: string;
  providers: string[];
}

type Action =
  | { type: "checking" }
  | {
      type: "connected";
      traceCount: number;
      llmCalls: number;
      totalCost: number;
      providers: string[];
      modelCount: number;
    }
  | { type: "offline" };

function reducer(state: SettingsState, action: Action): SettingsState {
  switch (action.type) {
    case "checking":
      return { ...state, health: "checking" };
    case "connected":
      return {
        ...state,
        health: "connected",
        traceCount: action.traceCount,
        llmCalls: action.llmCalls,
        totalCost: action.totalCost,
        providers: action.providers,
        modelCount: action.modelCount,
      };
    case "offline":
      return { ...state, health: "offline" };
  }
}

// ──────────────────────────────────────────
// Page
// ──────────────────────────────────────────

export default function SettingsPage() {
  const { user, isAdmin, isLoading: userLoading } = useCurrentUser();

  const [state, dispatch] = useReducer(reducer, {
    health: "checking",
    backendVersion: process.env.NEXT_PUBLIC_APP_VERSION ?? "v0.1.0-dev",
    traceCount: null,
    llmCalls: null,
    totalCost: null,
    modelCount: null,
    storageBackend: "DuckDB",
    providers: [],
  });

  useEffect(() => {
    dispatch({ type: "checking" });

    const now = new Date();
    const start = new Date(now.getTime() - 30 * 24 * 60 * 60 * 1000);

    dashboardClient
      .getDashboardData({
        timeRange: {
          start: timestampFromDate(start),
          end: timestampFromDate(now),
        },
        includeBudget: true,
      })
      .then((res) => {
        const uniqueProviders = [
          ...new Set((res.models || []).map((m) => m.provider).filter(Boolean)),
        ];
        dispatch({
          type: "connected",
          traceCount: Number(res.summary?.totalTraces ?? 0),
          llmCalls: Number(res.summary?.totalLlmCalls ?? 0),
          totalCost: res.summary?.totalCostUsd ?? 0,
          providers: uniqueProviders.length > 0 ? uniqueProviders : ["OpenAI", "Google (Gemini)", "Anthropic"],
          modelCount: (res.models || []).length,
        });
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
        {/* User Account Card */}
        {!userLoading && user && (
          <div className="card animate-in user-account-card" style={{ marginBottom: 16 }}>
            <div className="card-title">Your Account</div>
            <div className="user-account-body">
              <div className="user-account-avatar">
                {user.email.charAt(0).toUpperCase()}
              </div>
              <div className="user-account-info">
                <div className="settings-grid">
                  <div className="settings-row">
                    <span className="settings-label">Email</span>
                    <span className="settings-value mono">{user.email}</span>
                  </div>
                  <div className="settings-row">
                    <span className="settings-label">Display Name</span>
                    <span className="settings-value">
                      {user.displayName || user.email.split("@")[0]}
                    </span>
                  </div>
                  <div className="settings-row">
                    <span className="settings-label">Role</span>
                    <span className="settings-value">
                      <span className={`badge ${isAdmin ? "badge-warning" : "badge-info"}`}>
                        {isAdmin ? "Admin" : "Developer"}
                      </span>
                    </span>
                  </div>
                </div>
              </div>
            </div>
          </div>
        )}

        {/* Backend Connection */}
        <div className="card animate-in" style={{ marginBottom: 16, animationDelay: "0.05s" }}>
          <div className="card-title">Backend Connection</div>
          <div className="settings-grid">
            <div className="settings-row">
              <span className="settings-label">Status</span>
              <span className="settings-value">
                <span className="health-dot" style={{ background: healthDot }} />
                {healthLabel}
              </span>
            </div>
            <div className="settings-row">
              <span className="settings-label">API URL</span>
              <span className="settings-value mono">{API_BASE_URL}</span>
            </div>
            <div className="settings-row">
              <span className="settings-label">Protocol</span>
              <span className="settings-value">ConnectRPC v2</span>
            </div>
            <div className="settings-row">
              <span className="settings-label">Version</span>
              <span className="settings-value mono">{state.backendVersion}</span>
            </div>
          </div>
        </div>

        {/* System Overview (live data) */}
        <div className="card animate-in" style={{ marginBottom: 16, animationDelay: "0.08s" }}>
          <div className="card-title">System Overview (30 days)</div>
          <div className="settings-grid">
            {state.traceCount !== null && (
              <div className="settings-row">
                <span className="settings-label">Total Traces</span>
                <span className="settings-value">
                  {state.traceCount.toLocaleString()}
                </span>
              </div>
            )}
            {state.llmCalls !== null && (
              <div className="settings-row">
                <span className="settings-label">LLM Calls</span>
                <span className="settings-value">
                  {state.llmCalls.toLocaleString()}
                </span>
              </div>
            )}
            {state.totalCost !== null && (
              <div className="settings-row">
                <span className="settings-label">Total Cost</span>
                <span className="settings-value">
                  ${state.totalCost.toFixed(2)}
                </span>
              </div>
            )}
            {state.modelCount !== null && (
              <div className="settings-row">
                <span className="settings-label">Active Models</span>
                <span className="settings-value">
                  {state.modelCount}
                </span>
              </div>
            )}
          </div>
        </div>

        {/* Storage */}
        <div className="card animate-in" style={{ marginBottom: 16, animationDelay: "0.1s" }}>
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
        <div className="card animate-in" style={{ animationDelay: "0.12s" }}>
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

"use client";

import { useState, useEffect, useCallback } from "react";
import { useAuth } from "@/components/AuthProvider";

export const ONBOARDING_COMPLETED_KEY = "candela:onboarding_completed";

export function openOnboardingWizard() {
  if (typeof window !== "undefined") {
    window.dispatchEvent(new CustomEvent("candela:open-onboarding"));
  }
}

interface StepConfig {
  id: string;
  title: string;
  shortLabel: string;
}

const STEPS: StepConfig[] = [
  { id: "auth", title: "1. Authenticate", shortLabel: "Auth" },
  { id: "server", title: "2. Configure Server", shortLabel: "Server" },
  { id: "proxy", title: "3. Set Up Proxy", shortLabel: "Proxy" },
  { id: "verify", title: "4. Verify Connectivity", shortLabel: "Verify" },
];

const CONFIG_TABS = [
  {
    id: "python",
    label: "Python SDK",
    icon: "🐍",
    snippet: (url: string) =>
      `import openai\n\nclient = openai.OpenAI(\n    base_url="${url}/openai/v1",\n    api_key="YOUR_CANDELA_API_KEY",\n)`,
  },
  {
    id: "curl",
    label: "cURL",
    icon: "🌐",
    snippet: (url: string) =>
      `curl ${url}/openai/v1/chat/completions \\\n  -H "Authorization: Bearer YOUR_CANDELA_API_KEY" \\\n  -H "Content-Type: application/json" \\\n  -d '{\n    "model": "gpt-4o",\n    "messages": [{"role": "user", "content": "Hello"}]\n  }'`,
  },
  {
    id: "vscode",
    label: "VS Code / Continue",
    icon: "📝",
    snippet: (url: string) =>
      `{\n  "models": [\n    {\n      "title": "GPT-4o via Candela",\n      "provider": "openai",\n      "model": "gpt-4o",\n      "apiBase": "${url}/openai/v1",\n      "apiKey": "YOUR_CANDELA_API_KEY"\n    }\n  ]\n}`,
  },
  {
    id: "cursor",
    label: "Cursor",
    icon: "⚡",
    snippet: (url: string) =>
      `{\n  "openai.api_base": "${url}/openai/v1",\n  "openai.api_key": "YOUR_CANDELA_API_KEY"\n}`,
  },
  {
    id: "env",
    label: "Env Vars",
    icon: "🔑",
    snippet: (url: string) =>
      `export OPENAI_API_BASE=${url}/openai/v1\nexport OPENAI_API_KEY=YOUR_CANDELA_API_KEY`,
  },
];

interface OnboardingWizardProps {
  forceOpen?: boolean;
  onClose?: () => void;
}

export function OnboardingWizard({ forceOpen, onClose }: OnboardingWizardProps) {
  const { user, configured, signIn } = useAuth();
  const [isOpen, setIsOpen] = useState(() => {
    if (forceOpen) return true;
    if (typeof window !== "undefined") {
      try {
        return localStorage.getItem(ONBOARDING_COMPLETED_KEY) !== "true";
      } catch {
        return false;
      }
    }
    return false;
  });
  const [prevForceOpen, setPrevForceOpen] = useState(forceOpen);
  if (forceOpen !== prevForceOpen) {
    setPrevForceOpen(forceOpen);
    if (forceOpen) {
      setIsOpen(true);
    }
  }

  const [currentStep, setCurrentStep] = useState(0);
  const [activeTab, setActiveTab] = useState(CONFIG_TABS[0].id);
  const [copied, setCopied] = useState(false);

  // Connectivity probe state
  const [probeStatus, setProbeStatus] = useState<"idle" | "testing" | "connected" | "error">("idle");
  const [probeLatency, setProbeLatency] = useState<number | null>(null);
  const [probeError, setProbeError] = useState<string | null>(null);

  const proxyUrl = typeof window !== "undefined" ? window.location.origin : "http://localhost:8181";

  // Listen for external trigger (e.g. from /setup page)
  useEffect(() => {
    const handleOpen = () => setIsOpen(true);
    window.addEventListener("candela:open-onboarding", handleOpen);
    return () => window.removeEventListener("candela:open-onboarding", handleOpen);
  }, []);

  const handleClose = useCallback((markCompleted = false) => {
    if (markCompleted && typeof window !== "undefined") {
      try {
        localStorage.setItem(ONBOARDING_COMPLETED_KEY, "true");
      } catch (e) {
        console.warn("Failed to persist onboarding state", e);
      }
    }
    setIsOpen(false);
    onClose?.();
  }, [onClose]);

  const handleSkip = () => handleClose(true);
  const handleFinish = () => handleClose(true);

  const handleCopy = async () => {
    const tab = CONFIG_TABS.find((t) => t.id === activeTab) ?? CONFIG_TABS[0];
    const text = tab.snippet(proxyUrl);
    try {
      if (navigator.clipboard?.writeText) {
        await navigator.clipboard.writeText(text);
        setCopied(true);
        setTimeout(() => setCopied(false), 2000);
      } else {
        const textarea = document.createElement("textarea");
        textarea.value = text;
        textarea.style.position = "fixed";
        textarea.style.opacity = "0";
        document.body.appendChild(textarea);
        textarea.select();
        const success = document.execCommand("copy");
        document.body.removeChild(textarea);
        if (success) {
          setCopied(true);
          setTimeout(() => setCopied(false), 2000);
        }
      }
    } catch {
      // Do not set copied on error
    }
  };

  const handleTestConnectivity = async () => {
    setProbeStatus("testing");
    setProbeError(null);
    setProbeLatency(null);

    const start = performance.now();
    const controller = new AbortController();
    const timeoutId = setTimeout(() => controller.abort(), 4000);

    try {
      let res: Response;
      try {
        res = await fetch(`${proxyUrl}/healthz`, {
          method: "GET",
          signal: controller.signal,
        });
      } catch {
        res = await fetch(`${proxyUrl}/`, {
          method: "HEAD",
          signal: controller.signal,
        });
      }

      const elapsed = Math.round(performance.now() - start);

      if (res.ok) {
        setProbeStatus("connected");
        setProbeLatency(elapsed);
      } else {
        setProbeStatus("error");
        setProbeError(`Server returned HTTP ${res.status}`);
        setProbeLatency(elapsed);
      }
    } catch (err: unknown) {
      const elapsed = Math.round(performance.now() - start);
      setProbeStatus("error");
      if (err instanceof DOMException && err.name === "AbortError") {
        setProbeError("Connection timed out after 4s");
      } else {
        setProbeError(err instanceof Error ? err.message : "Failed to connect to proxy endpoint");
      }
      setProbeLatency(elapsed);
    } finally {
      clearTimeout(timeoutId);
    }
  };

  if (!isOpen) return null;

  const currentTabConfig = CONFIG_TABS.find((t) => t.id === activeTab) ?? CONFIG_TABS[0];
  const snippet = currentTabConfig.snippet(proxyUrl);

  return (
    <div
      className="modal-overlay"
      role="dialog"
      aria-modal="true"
      aria-labelledby="onboarding-wizard-title"
      onClick={(e) => {
        if (e.target === e.currentTarget) handleClose(false);
      }}
    >
      <div className="onboarding-modal animate-in" onClick={(e) => e.stopPropagation()}>
        {/* Stepper Header */}
        <div className="onboarding-stepper">
          {STEPS.map((step, idx) => {
            const isActive = idx === currentStep;
            const isCompleted = idx < currentStep;
            return (
              <div key={step.id} style={{ display: "flex", alignItems: "center", flex: 1 }}>
                <button
                  type="button"
                  className={`onboarding-step-item ${isActive ? "onboarding-step-item-active" : ""} ${isCompleted ? "onboarding-step-item-completed" : ""}`}
                  onClick={() => setCurrentStep(idx)}
                  aria-current={isActive ? "step" : undefined}
                >
                  <span className="onboarding-step-badge">
                    {isCompleted ? "✓" : idx + 1}
                  </span>
                  <span>{step.shortLabel}</span>
                </button>
                {idx < STEPS.length - 1 && (
                  <div className={`onboarding-step-divider ${isCompleted ? "onboarding-step-divider-completed" : ""}`} />
                )}
              </div>
            );
          })}
          <button
            type="button"
            className="modal-close"
            onClick={() => handleClose(false)}
            aria-label="Close onboarding wizard"
          >
            ×
          </button>
        </div>

        {/* Step Content */}
        <div className="onboarding-body">
          {currentStep === 0 && (
            <div>
              <div id="onboarding-wizard-title" className="onboarding-step-title">
                <span>🔐</span> Authenticate Your Workspace
              </div>
              <p className="onboarding-step-desc">
                Candela enforces access control, model policies, and per-user cost tracking.
              </p>

              <div className="onboarding-card">
                <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", marginBottom: 12 }}>
                  <span style={{ fontSize: 14, fontWeight: 600 }}>Auth Status</span>
                  {user ? (
                    <span className="onboarding-status-pill onboarding-status-active">
                      ● Signed in
                    </span>
                  ) : (
                    <span className="onboarding-status-pill onboarding-status-inactive">
                      ● Local Mode
                    </span>
                  )}
                </div>

                {user ? (
                  <div>
                    <div style={{ fontSize: 13, color: "var(--text-secondary)", marginBottom: 4 }}>
                      Authenticated Account:
                    </div>
                    <code style={{ fontSize: 13, color: "var(--accent)" }}>{user.email || user.uid}</code>
                    <p style={{ fontSize: 12, color: "var(--text-muted)", marginTop: 8 }}>
                      Your traces and budgets are automatically attributed to this identity.
                    </p>
                  </div>
                ) : (
                  <div>
                    <p style={{ fontSize: 13, color: "var(--text-secondary)", lineHeight: 1.5, marginBottom: 12 }}>
                      {configured
                        ? "Authentication is available for team sync. Sign in to link your personal budget."
                        : "Candela is currently operating in Solo / Local mode without cloud authentication."}
                    </p>
                    {configured && (
                      <button
                        type="button"
                        className="btn btn-sm"
                        onClick={() => signIn()}
                        style={{ background: "var(--accent)", color: "#000", fontWeight: 600 }}
                      >
                        Sign In with Google
                      </button>
                    )}
                  </div>
                )}
              </div>
            </div>
          )}

          {currentStep === 1 && (
            <div>
              <div id="onboarding-wizard-title" className="onboarding-step-title">
                <span>⚙️</span> Server Configuration
              </div>
              <p className="onboarding-step-desc">
                Review your active Candela server proxy endpoint and target environment.
              </p>

              <div className="onboarding-card">
                <label className="filter-label" style={{ marginBottom: 6 }}>
                  Active Proxy Base URL
                </label>
                <div className="setup-proxy-url" style={{ marginBottom: 12 }}>
                  <span className="setup-proxy-dot" />
                  <code>{proxyUrl}</code>
                </div>
                <p style={{ fontSize: 13, color: "var(--text-secondary)", lineHeight: 1.5 }}>
                  All outgoing calls to LLM providers (OpenAI, Anthropic, Google Vertex AI) route through this local transparent proxy to calculate token costs and evaluate budget limits.
                </p>
              </div>
            </div>
          )}

          {currentStep === 2 && (
            <div>
              <div id="onboarding-wizard-title" className="onboarding-step-title">
                <span>🔌</span> Set Up Your Proxy Client
              </div>
              <p className="onboarding-step-desc">
                Configure your application, IDE, or agent framework to route requests via Candela.
              </p>

              {/* Tabs */}
              <div className="setup-tabs" style={{ marginBottom: 12 }}>
                {CONFIG_TABS.map((tab) => (
                  <button
                    key={tab.id}
                    type="button"
                    className={`setup-tab ${activeTab === tab.id ? "setup-tab-active" : ""}`}
                    onClick={() => setActiveTab(tab.id)}
                  >
                    <span>{tab.icon}</span>
                    <span>{tab.label}</span>
                  </button>
                ))}
              </div>

              {/* Snippet Card */}
              <div className="setup-snippet-container">
                <div className="setup-snippet-header">
                  <span className="setup-snippet-lang">{currentTabConfig.label}</span>
                  <button
                    type="button"
                    className={`setup-copy-btn ${copied ? "setup-copy-btn-copied" : ""}`}
                    onClick={handleCopy}
                  >
                    {copied ? "✓ Copied!" : "📋 Copy"}
                  </button>
                </div>
                <pre className="setup-snippet-code" style={{ maxHeight: 180, overflowY: "auto" }}>
                  <code>{snippet}</code>
                </pre>
              </div>
            </div>
          )}

          {currentStep === 3 && (
            <div>
              <div id="onboarding-wizard-title" className="onboarding-step-title">
                <span>🚀</span> Verify Connectivity
              </div>
              <p className="onboarding-step-desc">
                Test communication with the proxy service to confirm telemetry collection is ready.
              </p>

              <div className="onboarding-card">
                <div style={{ display: "flex", justifyContent: "space-between", alignItems: "center", marginBottom: 16 }}>
                  <div>
                    <div style={{ fontSize: 14, fontWeight: 600 }}>Connection Probe</div>
                    <div style={{ fontSize: 12, color: "var(--text-secondary)" }}>
                      Target: <code>{proxyUrl}/healthz</code>
                    </div>
                  </div>
                  <button
                    type="button"
                    className="btn btn-sm"
                    onClick={handleTestConnectivity}
                    disabled={probeStatus === "testing"}
                    style={{ background: "var(--accent)", color: "#000", fontWeight: 600 }}
                  >
                    {probeStatus === "testing" ? "Testing..." : "Test Connection"}
                  </button>
                </div>

                {probeStatus === "connected" && (
                  <div style={{ padding: 12, borderRadius: 6, background: "rgba(52, 211, 153, 0.15)", border: "1px solid rgba(52, 211, 153, 0.3)", color: "var(--success)" }}>
                    <div style={{ fontWeight: 600, display: "flex", alignItems: "center", gap: 6 }}>
                      <span>✓</span> Connected successfully!
                    </div>
                    <div style={{ fontSize: 12, marginTop: 4, color: "var(--text-secondary)" }}>
                      Candela proxy is reachable. Latency: {probeLatency ?? "<1"} ms
                    </div>
                  </div>
                )}

                {probeStatus === "error" && (
                  <div style={{ padding: 12, borderRadius: 6, background: "rgba(248, 113, 113, 0.15)", border: "1px solid rgba(248, 113, 113, 0.3)", color: "var(--error)" }}>
                    <div style={{ fontWeight: 600, display: "flex", alignItems: "center", gap: 6 }}>
                      <span>✕</span> Connection error
                    </div>
                    <div style={{ fontSize: 12, marginTop: 4, color: "var(--text-secondary)" }}>
                      {probeError || "Ensure the Candela server is running on the configured port."}
                    </div>
                  </div>
                )}

                {probeStatus === "idle" && (
                  <div style={{ fontSize: 13, color: "var(--text-muted)" }}>
                    Click &ldquo;Test Connection&rdquo; to send a ping request to the Candela proxy server.
                  </div>
                )}
              </div>
            </div>
          )}
        </div>

        {/* Footer Actions */}
        <div className="onboarding-actions">
          <button
            type="button"
            className="btn btn-ghost"
            onClick={handleSkip}
            style={{ color: "var(--text-muted)", fontSize: 13 }}
          >
            Skip for now
          </button>

          <div className="onboarding-actions-right">
            {currentStep > 0 && (
              <button
                type="button"
                className="btn btn-secondary"
                onClick={() => setCurrentStep((s) => s - 1)}
              >
                Back
              </button>
            )}

            {currentStep < STEPS.length - 1 ? (
              <button
                type="button"
                className="btn"
                style={{ background: "var(--accent)", color: "#000", fontWeight: 600 }}
                onClick={() => setCurrentStep((s) => s + 1)}
              >
                Next →
              </button>
            ) : (
              <button
                type="button"
                className="btn"
                style={{ background: "var(--success)", color: "#000", fontWeight: 600 }}
                onClick={handleFinish}
              >
                Finish Setup ✓
              </button>
            )}
          </div>
        </div>
      </div>
    </div>
  );
}

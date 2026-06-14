"use client";

import { useState, useEffect, useCallback } from "react";

// ──────────────────────────────────────────
// Config snippets
// ──────────────────────────────────────────

interface ConfigTab {
  id: string;
  label: string;
  icon: string;
  language: string;
  snippet: (proxyUrl: string) => string;
}

const CONFIG_TABS: ConfigTab[] = [
  {
    id: "python",
    label: "Python SDK",
    icon: "🐍",
    language: "python",
    snippet: (url) =>
      `import openai

client = openai.OpenAI(
    base_url="${url}/openai/v1",
    api_key="YOUR_CANDELA_API_KEY",  # Your Candela API key
)`,
  },
  {
    id: "curl",
    label: "curl",
    icon: "🌐",
    language: "bash",
    snippet: (url) =>
      `curl ${url}/openai/v1/chat/completions \\
  -H "Authorization: Bearer YOUR_CANDELA_API_KEY" \\
  -H "Content-Type: application/json" \\
  -d '{
    "model": "gpt-4o",
    "messages": [{"role": "user", "content": "Hello"}]
  }'`,
  },
  {
    id: "vscode",
    label: "VS Code / Continue",
    icon: "📝",
    language: "json",
    snippet: (url) =>
      `{
  "models": [
    {
      "title": "GPT-4o via Candela",
      "provider": "openai",
      "model": "gpt-4o",
      "apiBase": "${url}/openai/v1",
      "apiKey": "YOUR_CANDELA_API_KEY"
    }
  ]
}`,
  },
  {
    id: "cursor",
    label: "Cursor",
    icon: "⚡",
    language: "json",
    snippet: (url) =>
      `{
  "openai.api_base": "${url}/openai/v1",
  "openai.api_key": "YOUR_CANDELA_API_KEY"
}`,
  },
  {
    id: "env",
    label: "Env Variables",
    icon: "🔑",
    language: "bash",
    snippet: (url) =>
      `export OPENAI_API_BASE=${url}/openai/v1
export OPENAI_API_KEY=YOUR_CANDELA_API_KEY`,
  },
];

// ──────────────────────────────────────────
// Page
// ──────────────────────────────────────────

export default function SetupPage() {
  const [activeTab, setActiveTab] = useState(CONFIG_TABS[0].id);
  const [proxyUrl, setProxyUrl] = useState("https://your-candela-host");
  const [copied, setCopied] = useState(false);

  useEffect(() => {
    setProxyUrl(window.location.origin);
  }, []);

  const currentTab = CONFIG_TABS.find((t) => t.id === activeTab)!;
  const snippetText = currentTab.snippet(proxyUrl);

  const handleCopy = useCallback(async () => {
    try {
      await navigator.clipboard.writeText(snippetText);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    } catch {
      // Fallback for older browsers
      const textarea = document.createElement("textarea");
      textarea.value = snippetText;
      document.body.appendChild(textarea);
      textarea.select();
      document.execCommand("copy");
      document.body.removeChild(textarea);
      setCopied(true);
      setTimeout(() => setCopied(false), 2000);
    }
  }, [snippetText]);

  // Reset copied state on tab switch
  useEffect(() => {
    setCopied(false);
  }, [activeTab]);

  return (
    <>
      <header className="main-header">
        <h1>Setup</h1>
      </header>

      <div className="main-body">
        {/* Intro */}
        <div className="card animate-in" style={{ marginBottom: 16 }}>
          <div className="card-title">Connect Your Tools</div>
          <p style={{ fontSize: 14, color: "var(--text-secondary)", lineHeight: 1.6 }}>
            Configure your AI tools to route through Candela. Select a tool below, copy
            the configuration snippet, and replace{" "}
            <code className="setup-inline-code">YOUR_CANDELA_API_KEY</code> with your
            actual API key.
          </p>
        </div>

        {/* Proxy URL display */}
        <div
          className="card animate-in"
          style={{ marginBottom: 16, animationDelay: "0.05s" }}
        >
          <div className="card-title">Your Proxy URL</div>
          <div className="setup-proxy-url">
            <span className="setup-proxy-dot" />
            <code>{proxyUrl}</code>
          </div>
        </div>

        {/* Tabs + Snippet */}
        <div
          className="card animate-in"
          style={{ animationDelay: "0.1s", padding: 0, overflow: "hidden" }}
        >
          {/* Tab bar */}
          <div className="setup-tabs">
            {CONFIG_TABS.map((tab) => (
              <button
                key={tab.id}
                className={`setup-tab ${activeTab === tab.id ? "setup-tab-active" : ""}`}
                onClick={() => setActiveTab(tab.id)}
              >
                <span className="setup-tab-icon">{tab.icon}</span>
                <span className="setup-tab-label">{tab.label}</span>
              </button>
            ))}
          </div>

          {/* Snippet area */}
          <div className="setup-snippet-container">
            <div className="setup-snippet-header">
              <span className="setup-snippet-lang">{currentTab.language}</span>
              <button
                className={`setup-copy-btn ${copied ? "setup-copy-btn-copied" : ""}`}
                onClick={handleCopy}
              >
                {copied ? (
                  <>
                    <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2.5" strokeLinecap="round" strokeLinejoin="round">
                      <polyline points="20 6 9 17 4 12" />
                    </svg>
                    Copied!
                  </>
                ) : (
                  <>
                    <svg width="14" height="14" viewBox="0 0 24 24" fill="none" stroke="currentColor" strokeWidth="2" strokeLinecap="round" strokeLinejoin="round">
                      <rect x="9" y="9" width="13" height="13" rx="2" ry="2" />
                      <path d="M5 15H4a2 2 0 0 1-2-2V4a2 2 0 0 1 2-2h9a2 2 0 0 1 2 2v1" />
                    </svg>
                    Copy
                  </>
                )}
              </button>
            </div>
            <pre className="setup-snippet-code"><code>{snippetText}</code></pre>
          </div>
        </div>
      </div>
    </>
  );
}

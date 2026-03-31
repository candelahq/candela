export default function SettingsPage() {
  return (
    <>
      <header className="main-header">
        <h1>Settings</h1>
      </header>

      <div className="main-body">
        <div className="card animate-in" style={{ marginBottom: 16 }}>
          <div className="card-title">Backend Connection</div>
          <div style={{ marginTop: 12, display: "flex", flexDirection: "column", gap: 12 }}>
            <div style={{ display: "flex", justifyContent: "space-between", fontSize: 13 }}>
              <span style={{ color: "var(--text-secondary)" }}>API URL</span>
              <span className="mono">http://localhost:8080</span>
            </div>
            <div style={{ display: "flex", justifyContent: "space-between", fontSize: 13 }}>
              <span style={{ color: "var(--text-secondary)" }}>Protocol</span>
              <span>ConnectRPC</span>
            </div>
            <div style={{ display: "flex", justifyContent: "space-between", fontSize: 13 }}>
              <span style={{ color: "var(--text-secondary)" }}>Version</span>
              <span className="mono">v0.1.0</span>
            </div>
          </div>
        </div>

        <div className="card animate-in" style={{ animationDelay: "0.05s" }}>
          <div className="card-title">Configured Providers</div>
          <div className="empty-state" style={{ padding: "32px 24px" }}>
            <div className="empty-state-desc">
              Provider configuration is managed in{" "}
              <code className="mono">candela.yaml</code> on the server.
            </div>
          </div>
        </div>
      </div>
    </>
  );
}

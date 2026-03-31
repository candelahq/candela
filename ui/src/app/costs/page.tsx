"use client";

export default function CostsPage() {
  return (
    <>
      <header className="main-header">
        <h1>Costs</h1>
        <span className="mono" style={{ color: "var(--text-muted)" }}>
          Token usage &amp; spending
        </span>
      </header>

      <div className="main-body">
        <div className="stats-grid animate-in">
          <div className="card">
            <div className="card-title">Today</div>
            <div className="card-value">$0.00</div>
            <div className="card-subtitle">0 requests</div>
          </div>
          <div className="card">
            <div className="card-title">This Week</div>
            <div className="card-value">$0.00</div>
            <div className="card-subtitle">0 requests</div>
          </div>
          <div className="card">
            <div className="card-title">This Month</div>
            <div className="card-value">$0.00</div>
            <div className="card-subtitle">0 requests</div>
          </div>
          <div className="card">
            <div className="card-title">All Time</div>
            <div className="card-value">$0.00</div>
            <div className="card-subtitle">0 requests</div>
          </div>
        </div>

        <div className="table-container animate-in" style={{ animationDelay: "0.1s" }}>
          <div className="table-header">
            <span className="table-title">Cost by Model</span>
          </div>
          <div className="empty-state">
            <div className="empty-state-icon">💰</div>
            <div className="empty-state-title">No cost data yet</div>
            <div className="empty-state-desc">
              Cost breakdowns will appear here once traces start flowing
              through the proxy.
            </div>
          </div>
        </div>
      </div>
    </>
  );
}

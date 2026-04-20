"use client";

import { useEffect, useState } from "react";

interface BudgetGaugeProps {
  spent: number;
  limit: number;
  remaining: number;
  percent: number;
  period: string;
}

export function BudgetGauge({ spent, limit, remaining, percent, period }: BudgetGaugeProps) {
  const [offset, setOffset] = useState(251.2); // Full circle offset (2 * PI * r)

  useEffect(() => {
    // Animate the circle filling up
    const r = 40;
    const circumference = 2 * Math.PI * r;
    const progress = Math.min(percent, 100) / 100;
    const newOffset = circumference - progress * circumference;

    const timeout = setTimeout(() => {
      setOffset(newOffset);
    }, 100);

    return () => clearTimeout(timeout);
  }, [percent]);

  const getStatusColor = () => {
    if (percent >= 100) return "var(--error)";
    if (percent >= 80) return "var(--warning)";
    return "var(--accent)";
  };

  const statusColor = getStatusColor();

  return (
    <div className="card gauge-card animate-in">
      <div className="gauge-container">
        <svg viewBox="0 0 100 100" className="gauge-svg">
          {/* Background circle */}
          <circle
            cx="50"
            cy="50"
            r="40"
            fill="none"
            stroke="var(--bg-tertiary)"
            strokeWidth="8"
          />
          {/* Progress circle */}
          <circle
            cx="50"
            cy="50"
            r="40"
            fill="none"
            stroke={statusColor}
            strokeWidth="8"
            strokeDasharray="251.2"
            strokeDashoffset={offset}
            strokeLinecap="round"
            style={{ transition: "stroke-dashoffset 1.5s cubic-bezier(0.4, 0, 0.2, 1), stroke 0.3s ease" }}
            transform="rotate(-90 50 50)"
          />
          <text
            x="50"
            y="52"
            textAnchor="middle"
            className="gauge-text"
            style={{ fill: statusColor }}
          >
            {Math.round(percent)}%
          </text>
        </svg>
      </div>

      <div className="gauge-details">
        <div className="gauge-title">Personal Budget</div>
        <div className="gauge-period">Current {period} limit</div>

        <div className="gauge-stats">
          <div className="gauge-stat">
            <span className="gauge-stat-label">Spent</span>
            <span className="gauge-stat-value">${spent.toFixed(2)}</span>
          </div>
          <div className="gauge-divider" />
          <div className="gauge-stat">
            <span className="gauge-stat-label">Remaining</span>
            <span className="gauge-stat-value">${Math.max(0, remaining).toFixed(2)}</span>
          </div>
        </div>

        {percent >= 90 && (
          <div className="gauge-alert">
            ⚠️ You are approaching your budget limit.
          </div>
        )}
      </div>

      <style jsx>{`
        .gauge-card {
          display: flex;
          align-items: center;
          gap: 24px;
          padding: 32px;
          background: linear-gradient(135deg, var(--bg-secondary) 0%, var(--bg-tertiary) 100%);
        }
        .gauge-container {
          width: 140px;
          height: 140px;
          flex-shrink: 0;
        }
        .gauge-svg {
          width: 100%;
          height: 100%;
        }
        .gauge-text {
          font-weight: 700;
          font-size: 18px;
          letter-spacing: -0.02em;
        }
        .gauge-details {
          flex: 1;
        }
        .gauge-title {
          font-size: 20px;
          font-weight: 700;
          margin-bottom: 4px;
          letter-spacing: -0.01em;
        }
        .gauge-period {
          font-size: 13px;
          color: var(--text-muted);
          text-transform: capitalize;
          margin-bottom: 24px;
        }
        .gauge-stats {
          display: flex;
          gap: 32px;
        }
        .gauge-stat {
          display: flex;
          flex-direction: column;
          gap: 4px;
        }
        .gauge-stat-label {
          font-size: 11px;
          font-weight: 600;
          text-transform: uppercase;
          letter-spacing: 0.05em;
          color: var(--text-muted);
        }
        .gauge-stat-value {
          font-size: 24px;
          font-weight: 700;
          color: var(--text-primary);
        }
        .gauge-divider {
          width: 1px;
          background: var(--border-subtle);
        }
        .gauge-alert {
          margin-top: 20px;
          padding: 8px 12px;
          background: rgba(251, 191, 36, 0.1);
          border: 1px solid var(--warning);
          border-radius: var(--radius-md);
          font-size: 12px;
          color: var(--warning);
          animation: shake 0.5s ease-in-out;
        }
        @keyframes shake {
          0%, 100% { transform: translateX(0); }
          25% { transform: translateX(-4px); }
          75% { transform: translateX(4px); }
        }
      `}</style>
    </div>
  );
}

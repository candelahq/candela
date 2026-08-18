"use client";

import { useState, useEffect } from "react";

export function BudgetRing({ percent, spent, limit, remaining }: {
  percent: number;
  spent: number;
  limit: number;
  remaining: number;
}) {
  const [offset, setOffset] = useState(2 * Math.PI * 45);
  const r = 45;
  const circumference = 2 * Math.PI * r;

  useEffect(() => {
    const progress = Math.min(percent, 100) / 100;
    const t = setTimeout(() => setOffset(circumference - progress * circumference), 80);
    return () => clearTimeout(t);
  }, [percent, circumference]);

  const color =
    percent >= 100 ? "var(--error)" :
    percent >= 80 ? "var(--warning)" :
    "var(--accent)";

  return (
    <div className="today-ring-wrap">
      <svg viewBox="0 0 120 120" className="today-ring-svg">
        {/* Subtle glow */}
        <defs>
          <filter id="glow">
            <feGaussianBlur stdDeviation="3" result="blur" />
            <feMerge>
              <feMergeNode in="blur" />
              <feMergeNode in="SourceGraphic" />
            </feMerge>
          </filter>
        </defs>
        {/* Track */}
        <circle cx="60" cy="60" r={r} fill="none" stroke="var(--bg-tertiary)" strokeWidth="10" />
        {/* Progress */}
        <circle
          cx="60" cy="60" r={r}
          fill="none"
          stroke={color}
          strokeWidth="10"
          strokeDasharray={circumference}
          strokeDashoffset={offset}
          strokeLinecap="round"
          filter="url(#glow)"
          style={{ transition: "stroke-dashoffset 1.8s cubic-bezier(0.4, 0, 0.2, 1), stroke 0.3s" }}
          transform="rotate(-90 60 60)"
        />
        <text x="60" y="55" textAnchor="middle" className="today-ring-pct" style={{ fill: color }}>
          {Math.round(percent)}%
        </text>
        <text x="60" y="70" textAnchor="middle" className="today-ring-label">
          used
        </text>
      </svg>

      <div className="today-ring-meta">
        <div className="today-ring-stat">
          <span className="today-ring-stat-label">Spent</span>
          <span className="today-ring-stat-value" style={{ color }}>${spent.toFixed(2)}</span>
        </div>
        <div className="today-ring-divider" />
        <div className="today-ring-stat">
          <span className="today-ring-stat-label">Limit</span>
          <span className="today-ring-stat-value">${limit.toFixed(2)}</span>
        </div>
        <div className="today-ring-divider" />
        <div className="today-ring-stat">
          <span className="today-ring-stat-label">Left</span>
          <span className="today-ring-stat-value" style={{ color: remaining <= 0 ? "var(--error)" : "var(--success)" }}>
            ${Math.max(0, remaining).toFixed(2)}
          </span>
        </div>
      </div>
    </div>
  );
}

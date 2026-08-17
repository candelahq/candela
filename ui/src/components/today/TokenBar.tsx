"use client";

import type { TodayModelUsage } from "@/hooks/useTodayBudget";
import { fmtTokens } from "./utils";

export function TokenBar({ model }: { model: TodayModelUsage }) {
  const total = model.inputTokens + model.outputTokens;
  const inPct = total > 0 ? (model.inputTokens / total) * 100 : 0;

  return (
    <div className="today-model-row">
      <div className="today-model-info">
        <span className="today-model-name mono">{model.model}</span>
        <span className="today-model-provider">{model.provider}</span>
      </div>
      <div className="today-model-metrics">
        <span className="today-model-metric">
          <span className="today-model-metric-label">Requests</span>
          <span className="today-model-metric-value">{model.callCount.toLocaleString()}</span>
        </span>
        <span className="today-model-metric">
          <span className="today-model-metric-label">In</span>
          <span className="today-model-metric-value">{fmtTokens(model.inputTokens)}</span>
        </span>
        <span className="today-model-metric">
          <span className="today-model-metric-label">Out</span>
          <span className="today-model-metric-value">{fmtTokens(model.outputTokens)}</span>
        </span>
        <span className="today-model-metric">
          <span className="today-model-metric-label">Cost</span>
          <span className="today-model-metric-value">${model.costUsd.toFixed(4)}</span>
        </span>
      </div>
      <div className="today-token-bar">
        <div className="today-token-bar-in" style={{ width: `${inPct}%` }} />
        <div className="today-token-bar-out" style={{ width: `${100 - inPct}%` }} />
      </div>
    </div>
  );
}

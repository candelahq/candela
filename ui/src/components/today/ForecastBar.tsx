"use client";

import type { ForecastData } from "@/hooks/useForecast";

interface Props {
  forecast: ForecastData;
}

/** Renders burn rate, projected EOD, and exhaustion warning below the BudgetRing. */
export function ForecastBar({ forecast }: Props) {
  const {
    burnRatePerHour,
    projectedEodSpend,
    willExceedBudget,
    avgDailySpend,
    daysUntilExhaustion,
    estimatedExhaustionDate,
    spendHistory,
  } = forecast;

  // Don't render if no meaningful data (no budget configured).
  if (burnRatePerHour === 0 && avgDailySpend === 0) {
    return null;
  }

  return (
    <div className="forecast-bar">
      {/* Burn rate + projected EOD */}
      <div className="forecast-metrics">
        <div className="forecast-metric">
          <span className="forecast-metric-icon">🔥</span>
          <span className="forecast-metric-value">
            ${burnRatePerHour.toFixed(2)}/hr
          </span>
          <span className="forecast-metric-label">burn rate</span>
        </div>

        <span className="forecast-divider">·</span>

        <div className="forecast-metric">
          <span className="forecast-metric-label">Projected EOD:</span>
          <span
            className={`forecast-metric-value ${willExceedBudget ? "forecast-danger" : ""}`}
          >
            ${projectedEodSpend.toFixed(2)}
          </span>
          {willExceedBudget && (
            <span className="forecast-exceed-badge">⚠️ WILL EXCEED</span>
          )}
        </div>
      </div>

      {/* Exhaustion warning — only when ≤ 7 days */}
      {daysUntilExhaustion >= 0 && daysUntilExhaustion <= 7 && (
        <div
          className={`forecast-exhaustion ${
            daysUntilExhaustion <= 3
              ? "forecast-exhaustion-critical"
              : "forecast-exhaustion-warn"
          }`}
        >
          {daysUntilExhaustion === 0 ? (
            <span>🚫 Budget exhausted today</span>
          ) : (
            <span>
              ⏳ Budget exhausts in{" "}
              <strong>
                {daysUntilExhaustion} day{daysUntilExhaustion !== 1 ? "s" : ""}
              </strong>{" "}
              ({formatExhaustionDate(estimatedExhaustionDate)})
            </span>
          )}
        </div>
      )}

      {/* 7-day sparkline */}
      {spendHistory.length > 0 && (
        <div className="forecast-sparkline">
          <span className="forecast-sparkline-label">7-day spend</span>
          <div className="forecast-sparkline-bars">
            {spendHistory.map((day) => {
              const maxSpend = Math.max(
                ...spendHistory.map((d) => d.spend_usd),
                0.01,
              );
              const pct = (day.spend_usd / maxSpend) * 100;
              return (
                <div
                  key={day.date}
                  className="forecast-sparkline-bar-wrapper"
                  title={`${day.date}: $${day.spend_usd.toFixed(2)}`}
                >
                  <div
                    className="forecast-sparkline-bar"
                    style={{ height: `${Math.max(pct, 4)}%` }}
                  />
                  <span className="forecast-sparkline-day">
                    {dayLabel(day.date)}
                  </span>
                </div>
              );
            })}
          </div>
          {avgDailySpend > 0 && (
            <span className="forecast-avg-label">
              avg ${avgDailySpend.toFixed(2)}/day
            </span>
          )}
        </div>
      )}
    </div>
  );
}

function dayLabel(dateStr: string): string {
  try {
    const d = new Date(dateStr + "T00:00:00Z");
    return d.toLocaleDateString(undefined, { weekday: "short" }).slice(0, 3);
  } catch {
    return dateStr.slice(-2);
  }
}

function formatExhaustionDate(dateStr: string): string {
  if (!dateStr) return "";
  try {
    const d = new Date(dateStr + "T00:00:00Z");
    return d.toLocaleDateString(undefined, {
      weekday: "short",
      month: "short",
      day: "numeric",
    });
  } catch {
    return dateStr;
  }
}

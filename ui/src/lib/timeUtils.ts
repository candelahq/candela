/**
 * Shared time utilities for time ranges, labels, and chart data formatting.
 */

export type TimeRange = "24h" | "7d" | "30d";

export interface TimeSeriesInput {
  timestamp: string;
  value: number;
}

export interface DataPoint {
  label: string;
  value: number;
}

/**
 * Converts a TimeRange literal into duration in milliseconds.
 */
export function timeRangeToMs(range: TimeRange): number {
  switch (range) {
    case "24h":
      return 24 * 60 * 60 * 1000;
    case "7d":
      return 7 * 24 * 60 * 60 * 1000;
    case "30d":
      return 30 * 24 * 60 * 60 * 1000;
  }
}

/**
 * Formats an ISO 8601 timestamp string into a concise label appropriate for the selected TimeRange.
 * For 24h ranges, formats as local time (HH:MM).
 * For 7d/30d ranges, formats as local date (Month Day).
 */
export function formatTimeLabel(ts: string, range: TimeRange): string {
  const d = new Date(ts);
  if (range === "24h") {
    return d.toLocaleTimeString([], { hour: "2-digit", minute: "2-digit" });
  }
  return d.toLocaleDateString([], { month: "short", day: "numeric" });
}

/**
 * Transforms an array of timestamped data points into chart DataPoints with formatted labels.
 */
export function toDataPoints(
  pts: TimeSeriesInput[] | undefined | null,
  range: TimeRange
): DataPoint[] {
  return (pts || []).map((p) => ({
    label: formatTimeLabel(p.timestamp, range),
    value: p.value,
  }));
}

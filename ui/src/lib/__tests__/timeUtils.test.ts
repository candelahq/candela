import { describe, it, expect } from "vitest";
import { timeRangeToMs, formatTimeLabel, toDataPoints } from "../timeUtils";

describe("timeUtils", () => {
  describe("timeRangeToMs", () => {
    it("converts 24h to milliseconds", () => {
      expect(timeRangeToMs("24h")).toBe(24 * 60 * 60 * 1000);
    });

    it("converts 7d to milliseconds", () => {
      expect(timeRangeToMs("7d")).toBe(7 * 24 * 60 * 60 * 1000);
    });

    it("converts 30d to milliseconds", () => {
      expect(timeRangeToMs("30d")).toBe(30 * 24 * 60 * 60 * 1000);
    });
  });

  describe("formatTimeLabel", () => {
    it("formats 24h with time", () => {
      const ts = "2026-04-15T14:30:00Z";
      const label = formatTimeLabel(ts, "24h");
      // Should include hours and minutes
      expect(label).toMatch(/\d{1,2}:\d{2}/);
    });

    it("formats 7d with month and day", () => {
      const ts = "2026-04-15T14:30:00Z";
      const label = formatTimeLabel(ts, "7d");
      expect(label).toMatch(/Apr|15/);
    });

    it("formats 30d with month and day", () => {
      const ts = "2026-04-15T14:30:00Z";
      const label = formatTimeLabel(ts, "30d");
      expect(label).toMatch(/Apr|15/);
    });
  });

  describe("toDataPoints", () => {
    it("handles undefined or empty points gracefully", () => {
      expect(toDataPoints(undefined, "24h")).toEqual([]);
      expect(toDataPoints(null, "7d")).toEqual([]);
      expect(toDataPoints([], "30d")).toEqual([]);
    });

    it("maps series items to formatted DataPoint objects", () => {
      const pts = [
        { timestamp: "2026-04-15T10:00:00Z", value: 42 },
        { timestamp: "2026-04-15T11:00:00Z", value: 100 },
      ];
      const result = toDataPoints(pts, "24h");
      expect(result).toHaveLength(2);
      expect(result[0].value).toBe(42);
      expect(result[0].label).toMatch(/\d{1,2}:\d{2}/);
      expect(result[1].value).toBe(100);
    });
  });
});

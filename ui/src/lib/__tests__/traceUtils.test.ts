import { describe, it, expect } from "vitest";
import { statusLabel } from "../traceUtils";
import { SpanStatus } from "@/gen/candela/types/trace_pb";

describe("traceUtils", () => {
  describe("statusLabel", () => {
    it("returns error badge for SpanStatus.ERROR", () => {
      const result = statusLabel(SpanStatus.ERROR);
      expect(result).toEqual({ text: "error", cls: "badge-error" });
    });

    it("returns error badge for integer 2", () => {
      const result = statusLabel(2);
      expect(result).toEqual({ text: "error", cls: "badge-error" });
    });

    it("returns ok badge for SpanStatus.OK", () => {
      const result = statusLabel(SpanStatus.OK);
      expect(result).toEqual({ text: "ok", cls: "badge-success" });
    });

    it("returns ok badge for SpanStatus.UNSPECIFIED (default fallback)", () => {
      const result = statusLabel(SpanStatus.UNSPECIFIED);
      expect(result).toEqual({ text: "ok", cls: "badge-success" });
    });

    it("returns ok badge for integer 1", () => {
      const result = statusLabel(1);
      expect(result).toEqual({ text: "ok", cls: "badge-success" });
    });
  });
});

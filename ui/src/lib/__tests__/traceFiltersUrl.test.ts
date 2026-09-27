import { describe, it, expect } from "vitest";
import {
  filtersToSearchParams,
  filtersToQueryString,
  searchParamsToFilters,
} from "../traceFiltersUrl";
import { DEFAULT_FILTERS, type TraceFilters } from "@/types/traces";

describe("traceFiltersUrl", () => {
  it("serializes default filters to an empty search params string", () => {
    const params = filtersToSearchParams(DEFAULT_FILTERS);
    expect(params.toString()).toBe("");
    expect(filtersToQueryString(DEFAULT_FILTERS)).toBe("");
  });

  it("serializes non-default filter values", () => {
    const filters: TraceFilters = {
      ...DEFAULT_FILTERS,
      model: "gpt-4o",
      provider: "openai",
      status: "error",
      timeRange: "7d",
      search: "auth-check",
      descending: false,
      orderBy: "duration",
      jobId: "job-42",
      environment: "production",
      tenantId: "acme",
      traceGroup: "evals",
    };

    const qs = filtersToQueryString(filters);
    const params = new URLSearchParams(qs);

    expect(params.get("model")).toBe("gpt-4o");
    expect(params.get("provider")).toBe("openai");
    expect(params.get("status")).toBe("error");
    expect(params.get("timeRange")).toBe("7d");
    expect(params.get("search")).toBe("auth-check");
    expect(params.get("descending")).toBe("false");
    expect(params.get("orderBy")).toBe("duration");
    expect(params.get("jobId")).toBe("job-42");
    expect(params.get("environment")).toBe("production");
    expect(params.get("tenantId")).toBe("acme");
    expect(params.get("traceGroup")).toBe("evals");
  });

  it("deserializes empty query string to DEFAULT_FILTERS", () => {
    const parsed = searchParamsToFilters("");
    expect(parsed).toEqual(DEFAULT_FILTERS);

    const parsedWithQuestionMark = searchParamsToFilters("?");
    expect(parsedWithQuestionMark).toEqual(DEFAULT_FILTERS);
  });

  it("deserializes partial query parameters and applies defaults to missing keys", () => {
    const parsed = searchParamsToFilters("model=claude-3-5-sonnet&timeRange=30d");
    expect(parsed.model).toBe("claude-3-5-sonnet");
    expect(parsed.timeRange).toBe("30d");
    expect(parsed.provider).toBe("");
    expect(parsed.status).toBe("");
    expect(parsed.orderBy).toBe("start_time");
    expect(parsed.descending).toBe(true);
  });

  it("sanitizes invalid status or timeRange values to defaults", () => {
    const parsed = searchParamsToFilters("status=invalid&timeRange=999y");
    expect(parsed.status).toBe("");
    expect(parsed.timeRange).toBe("24h");
  });

  it("handles descending boolean parsing correctly", () => {
    expect(searchParamsToFilters("descending=false").descending).toBe(false);
    expect(searchParamsToFilters("descending=true").descending).toBe(true);
    expect(searchParamsToFilters("").descending).toBe(true);
  });

  it("roundtrips arbitrary filter combinations", () => {
    const custom: TraceFilters = {
      search: "payment",
      model: "gemini-2.5-flash",
      provider: "google",
      status: "ok",
      orderBy: "total_cost",
      descending: false,
      tenantId: "t-1",
      jobId: "j-2",
      timeRange: "1h",
      environment: "staging",
      traceGroup: "batch-1",
    };

    const qs = filtersToQueryString(custom);
    const restored = searchParamsToFilters(qs);
    expect(restored).toEqual(custom);
  });
});

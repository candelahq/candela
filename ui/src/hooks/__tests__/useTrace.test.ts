import { describe, it, expect, vi, beforeEach, afterEach } from "vitest";
import { renderHook, waitFor, act } from "@testing-library/react";
import { useTrace } from "../useTrace";
import { SpanStatus, SpanKind } from "@/gen/candela/types/trace_pb";

const mockGetTrace = vi.fn();

vi.mock("@/lib/api", () => ({
  traceClient: {
    getTrace: (...args: unknown[]) => mockGetTrace(...args),
  },
}));

function mockTraceResponse() {
  return {
    trace: {
      traceId: "trace-123",
      rootSpanName: "chat-pipeline",
      totalTokens: BigInt(500),
      totalCostUsd: 0.05,
      spanCount: 2,
      environment: "production",
      startTime: { seconds: BigInt(1700000000), nanos: 0 },
      duration: { seconds: BigInt(1), nanos: 500_000_000 },
      spans: [
        {
          spanId: "span-root",
          parentSpanId: "",
          name: "root-operation",
          kind: SpanKind.AGENT,
          status: SpanStatus.OK,
          startTime: { seconds: BigInt(1700000000), nanos: 0 },
          duration: { seconds: BigInt(1), nanos: 500_000_000 },
          tokens: BigInt(500),
          costUsd: 0.05,
        },
        {
          spanId: "span-child",
          parentSpanId: "span-root",
          name: "llm-call",
          kind: SpanKind.LLM,
          status: SpanStatus.OK,
          startTime: { seconds: BigInt(1700000000), nanos: 200_000_000 },
          duration: { seconds: BigInt(0), nanos: 800_000_000 },
          tokens: BigInt(400),
          costUsd: 0.04,
        },
      ],
    },
  };
}

describe("useTrace", () => {
  beforeEach(() => {
    vi.clearAllMocks();
  });

  afterEach(() => {
    vi.restoreAllMocks();
  });

  it("starts in loading state", () => {
    mockGetTrace.mockReturnValue(new Promise(() => {}));

    const { result } = renderHook(() => useTrace("trace-123"));
    expect(result.current.loading).toBe(true);
    expect(result.current.trace).toBeNull();
  });

  it("loads trace tree and handles selection and collapsing", async () => {
    mockGetTrace.mockResolvedValue(mockTraceResponse());

    const { result } = renderHook(() => useTrace("trace-123"));

    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });

    expect(result.current.trace?.traceId).toBe("trace-123");
    expect(result.current.trace?.flatSpans).toHaveLength(2);
    expect(result.current.selectedNode).toBeUndefined();

    // Select span
    act(() => {
      result.current.toggleSpan("span-root");
    });

    expect(result.current.selectedSpanId).toBe("span-root");
    expect(result.current.selectedNode?.span.spanId).toBe("span-root");

    // Toggle same span deselects it
    act(() => {
      result.current.toggleSpan("span-root");
    });
    expect(result.current.selectedSpanId).toBeNull();
    expect(result.current.selectedNode).toBeUndefined();

    // Toggle collapse
    act(() => {
      result.current.toggleCollapse("span-root");
    });
    expect(result.current.collapsedIds.has("span-root")).toBe(true);

    // Expand all
    act(() => {
      result.current.expandAll();
    });
    expect(result.current.collapsedIds.size).toBe(0);

    // Collapse all
    act(() => {
      result.current.collapseAll();
    });
    expect(result.current.collapsedIds.has("span-root")).toBe(true);
  });

  it("handles not found response", async () => {
    mockGetTrace.mockResolvedValue({ trace: null });

    const { result } = renderHook(() => useTrace("non-existent"));

    await waitFor(() => {
      expect(result.current.loading).toBe(false);
    });

    expect(result.current.error).toBe("Trace not found");
  });
});

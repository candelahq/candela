"use client";

import { useEffect, useReducer, useState } from "react";
import { traceClient } from "@/lib/api";
import type { Span } from "@/gen/types/trace_pb";
import { SpanKind } from "@/gen/types/trace_pb";

// ──────────────────────────────────────────
// Types
// ──────────────────────────────────────────

export interface SpanNode {
  span: Span;
  children: SpanNode[];
  depth: number;
  offsetMs: number;
  durationMs: number;
}

export interface TraceData {
  traceId: string;
  rootSpanName: string;
  totalDurationMs: number;
  totalTokens: number;
  totalCostUsd: number;
  spanCount: number;
  environment: string;
  startTime: string;
  tree: SpanNode[];
  flatSpans: SpanNode[];
}

// ──────────────────────────────────────────
// Helpers
// ──────────────────────────────────────────

export function tsToMs(ts?: { seconds: bigint; nanos: number }): number {
  if (!ts) return 0;
  return Number(ts.seconds) * 1000 + (ts.nanos ?? 0) / 1e6;
}

export function durationToMs(d?: { seconds: bigint; nanos: number }): number {
  if (!d) return 0;
  return Number(d.seconds) * 1000 + (d.nanos ?? 0) / 1e6;
}

export function kindLabel(kind: SpanKind): string {
  const map: Record<SpanKind, string> = {
    [SpanKind.UNSPECIFIED]: "span",
    [SpanKind.LLM]: "LLM",
    [SpanKind.AGENT]: "Agent",
    [SpanKind.TOOL]: "Tool",
    [SpanKind.RETRIEVAL]: "RAG",
    [SpanKind.EMBEDDING]: "Embed",
    [SpanKind.CHAIN]: "Chain",
    [SpanKind.GENERAL]: "General",
  };
  return map[kind] ?? "span";
}

export function kindColor(kind: SpanKind): string {
  const map: Record<SpanKind, string> = {
    [SpanKind.UNSPECIFIED]: "var(--text-secondary)",
    [SpanKind.LLM]: "#a78bfa",
    [SpanKind.AGENT]: "#f59e0b",
    [SpanKind.TOOL]: "#34d399",
    [SpanKind.RETRIEVAL]: "#60a5fa",
    [SpanKind.EMBEDDING]: "#f472b6",
    [SpanKind.CHAIN]: "#fb923c",
    [SpanKind.GENERAL]: "var(--text-secondary)",
  };
  return map[kind] ?? "var(--text-secondary)";
}

function buildTree(spans: Span[], traceStartMs: number): SpanNode[] {
  const nodeMap = new Map<string, SpanNode>();

  for (const span of spans) {
    nodeMap.set(span.spanId, {
      span,
      children: [],
      depth: 0,
      offsetMs: tsToMs(span.startTime) - traceStartMs,
      durationMs: durationToMs(span.duration),
    });
  }

  const roots: SpanNode[] = [];
  for (const node of nodeMap.values()) {
    if (node.span.parentSpanId && nodeMap.has(node.span.parentSpanId)) {
      nodeMap.get(node.span.parentSpanId)!.children.push(node);
    } else {
      roots.push(node);
    }
  }

  function setDepth(node: SpanNode, depth: number) {
    node.depth = depth;
    node.children.sort((a, b) => a.offsetMs - b.offsetMs);
    for (const child of node.children) setDepth(child, depth + 1);
  }
  roots.sort((a, b) => a.offsetMs - b.offsetMs);
  for (const root of roots) setDepth(root, 0);

  return roots;
}

function flattenTree(nodes: SpanNode[]): SpanNode[] {
  const result: SpanNode[] = [];
  function walk(node: SpanNode) {
    result.push(node);
    for (const child of node.children) walk(child);
  }
  for (const n of nodes) walk(n);
  return result;
}

// ──────────────────────────────────────────
// Reducer (avoids set-state-in-effect lint)
// ──────────────────────────────────────────

type TraceState = {
  trace: TraceData | null;
  loading: boolean;
  error: string | null;
};

type TraceAction =
  | { type: "success"; trace: TraceData }
  | { type: "error"; message: string }
  | { type: "not_found" };

function traceReducer(_state: TraceState, action: TraceAction): TraceState {
  switch (action.type) {
    case "success":
      return { trace: action.trace, loading: false, error: null };
    case "error":
      return { trace: null, loading: false, error: action.message };
    case "not_found":
      return { trace: null, loading: false, error: "Trace not found" };
  }
}

// ──────────────────────────────────────────
// Hook
// ──────────────────────────────────────────

/**
 * Hook for fetching a single trace with its span tree.
 * Uses useReducer to dispatch state transitions from async callbacks.
 */
export function useTrace(traceId: string) {
  const [state, dispatch] = useReducer(traceReducer, {
    trace: null,
    loading: true,
    error: null,
  });
  const [selectedSpanId, setSelectedSpanId] = useState<string | null>(null);

  useEffect(() => {
    let cancelled = false;

    traceClient
      .getTrace({ traceId })
      .then((res) => {
        if (cancelled) return;
        if (!res.trace) {
          dispatch({ type: "not_found" });
          return;
        }
        const t = res.trace;
        const traceStartMs = tsToMs(t.startTime);
        const tree = buildTree(t.spans, traceStartMs);
        const flatSpans = flattenTree(tree);

        dispatch({
          type: "success",
          trace: {
            traceId: t.traceId,
            rootSpanName: t.rootSpanName || "unknown",
            totalDurationMs: durationToMs(t.duration),
            totalTokens: Number(t.totalTokens) || 0,
            totalCostUsd: t.totalCostUsd || 0,
            spanCount: t.spanCount || t.spans.length,
            environment: t.environment || "—",
            startTime: t.startTime
              ? new Date(traceStartMs).toLocaleString()
              : "—",
            tree,
            flatSpans,
          },
        });
      })
      .catch((err) => {
        if (!cancelled) dispatch({ type: "error", message: err.message });
      });

    return () => { cancelled = true; };
  }, [traceId]);

  const selectedNode = state.trace?.flatSpans.find(
    (n) => n.span.spanId === selectedSpanId
  );

  const toggleSpan = (spanId: string) =>
    setSelectedSpanId((prev) => (prev === spanId ? null : spanId));

  return {
    trace: state.trace,
    loading: state.loading,
    error: state.error,
    selectedSpanId,
    selectedNode,
    toggleSpan,
  };
}

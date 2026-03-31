"use client";

import { useEffect, useState } from "react";
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
  /** Offset from trace start in ms */
  offsetMs: number;
  /** Duration in ms */
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
      const parent = nodeMap.get(node.span.parentSpanId)!;
      parent.children.push(node);
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
// Hook
// ──────────────────────────────────────────

/**
 * Hook for fetching a single trace with its span tree.
 * Encapsulates the GetTrace RPC, tree building, and flattening.
 */
export function useTrace(traceId: string) {
  const [trace, setTrace] = useState<TraceData | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [selectedSpanId, setSelectedSpanId] = useState<string | null>(null);

  useEffect(() => {
    setLoading(true);
    setError(null);

    traceClient
      .getTrace({ traceId })
      .then((res) => {
        if (!res.trace) {
          setError("Trace not found");
          return;
        }
        const t = res.trace;
        const traceStartMs = tsToMs(t.startTime);
        const tree = buildTree(t.spans, traceStartMs);
        const flatSpans = flattenTree(tree);

        setTrace({
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
        });
      })
      .catch((err) => setError(err.message))
      .finally(() => setLoading(false));
  }, [traceId]);

  const selectedNode = trace?.flatSpans.find(
    (n) => n.span.spanId === selectedSpanId
  );

  const toggleSpan = (spanId: string) =>
    setSelectedSpanId((prev) => (prev === spanId ? null : spanId));

  return {
    trace,
    loading,
    error,
    selectedSpanId,
    selectedNode,
    toggleSpan,
  };
}

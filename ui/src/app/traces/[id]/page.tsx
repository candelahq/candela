"use client";

import { useEffect, useState } from "react";
import { useParams, useRouter } from "next/navigation";
import { createClient } from "@connectrpc/connect";
import { transport } from "@/lib/connect";
import { TraceService } from "@/gen/v1/trace_service_pb";
import type { Span } from "@/gen/types/trace_pb";
import { SpanKind, SpanStatus } from "@/gen/types/trace_pb";

// ──────────────────────────────────────────
// Types
// ──────────────────────────────────────────

interface SpanNode {
  span: Span;
  children: SpanNode[];
  depth: number;
  /** Offset from trace start in ms */
  offsetMs: number;
  /** Duration in ms */
  durationMs: number;
}

interface TraceData {
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

function tsToMs(ts?: { seconds: bigint; nanos: number }): number {
  if (!ts) return 0;
  return Number(ts.seconds) * 1000 + (ts.nanos ?? 0) / 1e6;
}

function durationToMs(d?: { seconds: bigint; nanos: number }): number {
  if (!d) return 0;
  return Number(d.seconds) * 1000 + (d.nanos ?? 0) / 1e6;
}

function kindLabel(kind: SpanKind): string {
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

function kindColor(kind: SpanKind): string {
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

  // Create nodes
  for (const span of spans) {
    nodeMap.set(span.spanId, {
      span,
      children: [],
      depth: 0,
      offsetMs: tsToMs(span.startTime) - traceStartMs,
      durationMs: durationToMs(span.duration),
    });
  }

  // Build tree
  const roots: SpanNode[] = [];
  for (const node of nodeMap.values()) {
    if (node.span.parentSpanId && nodeMap.has(node.span.parentSpanId)) {
      const parent = nodeMap.get(node.span.parentSpanId)!;
      parent.children.push(node);
    } else {
      roots.push(node);
    }
  }

  // Set depths and sort children by start time
  function setDepth(node: SpanNode, depth: number) {
    node.depth = depth;
    node.children.sort((a, b) => a.offsetMs - b.offsetMs);
    for (const child of node.children) {
      setDepth(child, depth + 1);
    }
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
// Components
// ──────────────────────────────────────────

function SpanRow({
  node,
  totalDurationMs,
  selected,
  onClick,
}: {
  node: SpanNode;
  totalDurationMs: number;
  selected: boolean;
  onClick: () => void;
}) {
  const leftPct = totalDurationMs > 0 ? (node.offsetMs / totalDurationMs) * 100 : 0;
  const widthPct = totalDurationMs > 0 ? Math.max((node.durationMs / totalDurationMs) * 100, 0.5) : 100;
  const color = kindColor(node.span.kind);
  const isError = node.span.status === SpanStatus.ERROR;

  return (
    <div
      className={`waterfall-row ${selected ? "waterfall-row-selected" : ""}`}
      onClick={onClick}
      role="button"
      tabIndex={0}
    >
      {/* Left: span label with tree indentation */}
      <div className="waterfall-label" style={{ paddingLeft: node.depth * 20 + 8 }}>
        <span className="waterfall-connector" />
        <span
          className="waterfall-kind"
          style={{ color, borderColor: color }}
        >
          {kindLabel(node.span.kind)}
        </span>
        <span className="waterfall-name" title={node.span.name}>
          {node.span.name}
        </span>
        {isError && <span className="badge badge-error" style={{ marginLeft: 6, fontSize: 10 }}>ERR</span>}
      </div>

      {/* Right: waterfall bar */}
      <div className="waterfall-bar-container">
        <div
          className="waterfall-bar"
          style={{
            left: `${leftPct}%`,
            width: `${widthPct}%`,
            backgroundColor: isError ? "var(--error)" : color,
          }}
        />
        <span
          className="waterfall-duration"
          style={{ left: `${Math.min(leftPct + widthPct + 1, 95)}%` }}
        >
          {node.durationMs.toFixed(0)}ms
        </span>
      </div>
    </div>
  );
}

function SpanDetail({ node }: { node: SpanNode }) {
  const { span } = node;
  const genAi = span.genAi;
  const tool = span.tool;

  return (
    <div className="span-detail animate-in">
      <div className="span-detail-header">
        <h3>{span.name}</h3>
        <span
          className="waterfall-kind"
          style={{
            color: kindColor(span.kind),
            borderColor: kindColor(span.kind),
          }}
        >
          {kindLabel(span.kind)}
        </span>
      </div>

      {/* Metadata grid */}
      <div className="span-meta-grid">
        <div className="span-meta-item">
          <span className="span-meta-label">Span ID</span>
          <span className="span-meta-value mono">{span.spanId}</span>
        </div>
        <div className="span-meta-item">
          <span className="span-meta-label">Duration</span>
          <span className="span-meta-value">{node.durationMs.toFixed(1)}ms</span>
        </div>
        <div className="span-meta-item">
          <span className="span-meta-label">Status</span>
          <span className="span-meta-value">
            <span className={`badge ${span.status === SpanStatus.ERROR ? "badge-error" : "badge-success"}`}>
              {span.status === SpanStatus.ERROR ? "Error" : "OK"}
            </span>
          </span>
        </div>
        {span.statusMessage && (
          <div className="span-meta-item" style={{ gridColumn: "1 / -1" }}>
            <span className="span-meta-label">Error Message</span>
            <span className="span-meta-value" style={{ color: "var(--error)" }}>{span.statusMessage}</span>
          </div>
        )}
        {span.serviceName && (
          <div className="span-meta-item">
            <span className="span-meta-label">Service</span>
            <span className="span-meta-value">{span.serviceName}</span>
          </div>
        )}
      </div>

      {/* GenAI attributes */}
      {genAi && (genAi.model || Number(genAi.totalTokens) > 0) && (
        <div className="span-section">
          <h4>🤖 GenAI</h4>
          <div className="span-meta-grid">
            {genAi.model && (
              <div className="span-meta-item">
                <span className="span-meta-label">Model</span>
                <span className="span-meta-value mono">{genAi.model}</span>
              </div>
            )}
            {genAi.provider && (
              <div className="span-meta-item">
                <span className="span-meta-label">Provider</span>
                <span className="span-meta-value">{genAi.provider}</span>
              </div>
            )}
            {Number(genAi.inputTokens) > 0 && (
              <div className="span-meta-item">
                <span className="span-meta-label">Input Tokens</span>
                <span className="span-meta-value">{Number(genAi.inputTokens).toLocaleString()}</span>
              </div>
            )}
            {Number(genAi.outputTokens) > 0 && (
              <div className="span-meta-item">
                <span className="span-meta-label">Output Tokens</span>
                <span className="span-meta-value">{Number(genAi.outputTokens).toLocaleString()}</span>
              </div>
            )}
            {genAi.costUsd > 0 && (
              <div className="span-meta-item">
                <span className="span-meta-label">Cost</span>
                <span className="span-meta-value">${genAi.costUsd.toFixed(6)}</span>
              </div>
            )}
            {genAi.temperature > 0 && (
              <div className="span-meta-item">
                <span className="span-meta-label">Temperature</span>
                <span className="span-meta-value">{genAi.temperature}</span>
              </div>
            )}
          </div>

          {/* Prompt / Completion */}
          {genAi.inputContent && (
            <div className="span-content-block">
              <div className="span-content-label">📥 Prompt</div>
              <pre className="span-content-pre">{genAi.inputContent}</pre>
            </div>
          )}
          {genAi.outputContent && (
            <div className="span-content-block">
              <div className="span-content-label">📤 Completion</div>
              <pre className="span-content-pre">{genAi.outputContent}</pre>
            </div>
          )}
        </div>
      )}

      {/* Tool attributes */}
      {tool && tool.toolName && (
        <div className="span-section">
          <h4>🔧 Tool Call</h4>
          <div className="span-meta-grid">
            <div className="span-meta-item">
              <span className="span-meta-label">Tool</span>
              <span className="span-meta-value mono">{tool.toolName}</span>
            </div>
          </div>
          {tool.toolInput && (
            <div className="span-content-block">
              <div className="span-content-label">Input</div>
              <pre className="span-content-pre">{tool.toolInput}</pre>
            </div>
          )}
          {tool.toolOutput && (
            <div className="span-content-block">
              <div className="span-content-label">Output</div>
              <pre className="span-content-pre">{tool.toolOutput}</pre>
            </div>
          )}
        </div>
      )}

      {/* Custom attributes */}
      {span.attributes.length > 0 && (
        <div className="span-section">
          <h4>📋 Attributes</h4>
          <table className="span-attr-table">
            <tbody>
              {span.attributes.map((attr, i) => (
                <tr key={i}>
                  <td className="span-attr-key mono">{attr.key}</td>
                  <td className="span-attr-val">
                    {attr.value.case
                      ? String(attr.value.value)
                      : "—"}
                  </td>
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      )}
    </div>
  );
}

// ──────────────────────────────────────────
// Page
// ──────────────────────────────────────────

export default function TraceDetailPage() {
  const params = useParams();
  const router = useRouter();
  const traceId = params.id as string;

  const [trace, setTrace] = useState<TraceData | null>(null);
  const [loading, setLoading] = useState(true);
  const [error, setError] = useState<string | null>(null);
  const [selectedSpanId, setSelectedSpanId] = useState<string | null>(null);

  useEffect(() => {
    const client = createClient(TraceService, transport);
    client
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

  return (
    <>
      <header className="main-header">
        <div style={{ display: "flex", alignItems: "center", gap: 12 }}>
          <button
            className="btn"
            onClick={() => router.push("/traces")}
            style={{ padding: "6px 12px" }}
          >
            ← Back
          </button>
          <h1 style={{ margin: 0 }}>
            {loading ? "Loading..." : trace?.rootSpanName ?? "Trace"}
          </h1>
        </div>
        {trace && (
          <div className="mono" style={{ fontSize: 12, color: "var(--text-muted)" }}>
            {trace.traceId}
          </div>
        )}
      </header>

      <div className="main-body">
        {error && (
          <div
            className="card animate-in"
            style={{
              borderColor: "var(--error)",
              marginBottom: 24,
              background: "rgba(248,113,113,0.05)",
            }}
          >
            <div className="card-title" style={{ color: "var(--error)" }}>
              {error.includes("fetch") ? "Backend Unavailable" : "Error"}
            </div>
            <div style={{ fontSize: 13, color: "var(--text-secondary)" }}>
              {error}
            </div>
          </div>
        )}

        {trace && (
          <>
            {/* Summary cards */}
            <div className="trace-summary-bar animate-in">
              <div className="trace-summary-item">
                <span className="trace-summary-label">Duration</span>
                <span className="trace-summary-value">
                  {trace.totalDurationMs.toFixed(0)}ms
                </span>
              </div>
              <div className="trace-summary-item">
                <span className="trace-summary-label">Spans</span>
                <span className="trace-summary-value">{trace.spanCount}</span>
              </div>
              <div className="trace-summary-item">
                <span className="trace-summary-label">Tokens</span>
                <span className="trace-summary-value">
                  {trace.totalTokens.toLocaleString()}
                </span>
              </div>
              <div className="trace-summary-item">
                <span className="trace-summary-label">Cost</span>
                <span className="trace-summary-value">
                  ${trace.totalCostUsd.toFixed(4)}
                </span>
              </div>
              <div className="trace-summary-item">
                <span className="trace-summary-label">Environment</span>
                <span className="badge badge-info">{trace.environment}</span>
              </div>
              <div className="trace-summary-item">
                <span className="trace-summary-label">Started</span>
                <span className="trace-summary-value" style={{ fontSize: 12 }}>
                  {trace.startTime}
                </span>
              </div>
            </div>

            {/* Waterfall + Detail split */}
            <div className="waterfall-split">
              {/* Waterfall panel */}
              <div className="waterfall-panel animate-in">
                <div className="waterfall-header">
                  <span className="table-title">Span Waterfall</span>
                  <span style={{ fontSize: 12, color: "var(--text-muted)" }}>
                    {trace.totalDurationMs.toFixed(0)}ms total
                  </span>
                </div>
                {/* Time ruler */}
                <div className="waterfall-ruler">
                  <div className="waterfall-ruler-label" style={{ left: 0 }}>0ms</div>
                  <div className="waterfall-ruler-label" style={{ left: "25%" }}>
                    {(trace.totalDurationMs * 0.25).toFixed(0)}ms
                  </div>
                  <div className="waterfall-ruler-label" style={{ left: "50%" }}>
                    {(trace.totalDurationMs * 0.5).toFixed(0)}ms
                  </div>
                  <div className="waterfall-ruler-label" style={{ left: "75%" }}>
                    {(trace.totalDurationMs * 0.75).toFixed(0)}ms
                  </div>
                  <div className="waterfall-ruler-label" style={{ right: 0 }}>
                    {trace.totalDurationMs.toFixed(0)}ms
                  </div>
                </div>
                {/* Span rows */}
                <div className="waterfall-body">
                  {trace.flatSpans.map((node) => (
                    <SpanRow
                      key={node.span.spanId}
                      node={node}
                      totalDurationMs={trace.totalDurationMs}
                      selected={node.span.spanId === selectedSpanId}
                      onClick={() =>
                        setSelectedSpanId(
                          node.span.spanId === selectedSpanId
                            ? null
                            : node.span.spanId
                        )
                      }
                    />
                  ))}
                </div>
              </div>

              {/* Detail panel */}
              {selectedNode && (
                <div className="detail-panel">
                  <SpanDetail node={selectedNode} />
                </div>
              )}
            </div>
          </>
        )}
      </div>
    </>
  );
}

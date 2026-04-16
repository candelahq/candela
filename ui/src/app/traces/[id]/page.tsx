"use client";

import { useState } from "react";

import { useParams, useRouter } from "next/navigation";
import { useTrace, kindLabel, kindColor } from "@/hooks/useTrace";
import type { SpanNode } from "@/hooks/useTrace";
import { ErrorBanner } from "@/components/ErrorBanner";
import { SpanStatus } from "@/gen/types/trace_pb";


function ExpandablePre({ content, label }: { content: string; label: string }) {
  const [expanded, setExpanded] = useState(false);

  // Try to pretty-print JSON
  let formatted = content;
  try {
    const parsed = JSON.parse(content);
    formatted = JSON.stringify(parsed, null, 2);
  } catch {
    // not JSON, use as-is
  }

  const needsExpand = formatted.length > 500 || formatted.split("\n").length > 15;

  return (
    <div className="span-content-block">
      <div className="span-content-label">{label}</div>
      <pre className={`span-content-pre ${expanded ? "expanded" : ""}`}>
        {formatted}
      </pre>
      {needsExpand && (
        <button
          className="span-content-toggle"
          onClick={() => setExpanded(!expanded)}
        >
          {expanded ? "▲ Collapse" : "▼ Show full content"}
        </button>
      )}
    </div>
  );
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
      <div className="waterfall-label" style={{ paddingLeft: node.depth * 20 + 8 }}>
        <span className="waterfall-connector" />
        <span className="waterfall-kind" style={{ color, borderColor: color }}>
          {kindLabel(node.span.kind)}
        </span>
        <span className="waterfall-name" title={node.span.name}>
          {node.span.name}
        </span>
        {isError && <span className="badge badge-error" style={{ marginLeft: 6, fontSize: 10 }}>ERR</span>}
      </div>

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

          {genAi.inputContent && (
            <ExpandablePre content={genAi.inputContent} label="📥 Prompt" />
          )}
          {genAi.outputContent && (
            <ExpandablePre content={genAi.outputContent} label="📤 Completion" />
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
            <ExpandablePre content={tool.toolInput} label="🔧 Input" />
          )}
          {tool.toolOutput && (
            <ExpandablePre content={tool.toolOutput} label="📋 Output" />
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

  const { trace, loading, error, selectedSpanId, selectedNode, toggleSpan } =
    useTrace(traceId);

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
          <ErrorBanner title={error.includes("fetch") ? "Backend Unavailable" : "Error"}>
            {error}
          </ErrorBanner>
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
                      onClick={() => toggleSpan(node.span.spanId)}
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

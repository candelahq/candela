"use client";

import { useMemo, useCallback } from "react";
import type { SpanNode } from "@/hooks/useTrace";
import { kindLabel, kindColor } from "@/hooks/useTrace";
import { SpanStatus } from "@/gen/candela/types/trace_pb";

// ──────────────────────────────────────────
// Layout constants
// ──────────────────────────────────────────

const NODE_W = 260;
const NODE_H = 72;
const H_GAP = 40; // horizontal gap between siblings
const V_GAP = 80; // vertical gap between levels

// Node rendering constants
const BADGE_CHAR_WIDTH = 7.5;
const BADGE_PADDING = 10;
const BADGE_HEIGHT = 18;
const BADGE_Y = 10;
const BADGE_X = 14;
const ERR_BADGE_WIDTH = 28;
const ERR_BADGE_OFFSET = 38;
const SPAN_NAME_MAX_LEN = 30;
const SPAN_NAME_TRUNC_LEN = 28;

// ──────────────────────────────────────────
// Layout types
// ──────────────────────────────────────────

interface LayoutNode {
  node: SpanNode;
  x: number;
  y: number;
  width: number;
  children: LayoutNode[];
}

// ──────────────────────────────────────────
// Tree layout algorithm
// ──────────────────────────────────────────

/** Compute the subtree width for a node (sum of children widths or self) */
function subtreeWidth(node: SpanNode): number {
  if (node.children.length === 0) return NODE_W;
  const childWidths = node.children.map(subtreeWidth);
  return childWidths.reduce((a, b) => a + b, 0) + (node.children.length - 1) * H_GAP;
}

/** Recursively lay out nodes in a top-down tree */
function layoutTree(
  nodes: SpanNode[],
  startX: number,
  startY: number
): LayoutNode[] {
  const result: LayoutNode[] = [];
  let cursor = startX;

  for (const node of nodes) {
    const w = subtreeWidth(node);
    const x = cursor + w / 2 - NODE_W / 2;
    const y = startY;

    const children = layoutTree(node.children, cursor, y + NODE_H + V_GAP);
    result.push({ node, x, y, width: w, children });

    cursor += w + H_GAP;
  }

  return result;
}

/** Flatten layout tree for rendering */
function flattenLayout(nodes: LayoutNode[]): LayoutNode[] {
  const result: LayoutNode[] = [];
  function walk(n: LayoutNode) {
    result.push(n);
    n.children.forEach(walk);
  }
  nodes.forEach(walk);
  return result;
}

/** Collect edges from layout tree */
function collectEdges(
  nodes: LayoutNode[]
): { from: LayoutNode; to: LayoutNode }[] {
  const edges: { from: LayoutNode; to: LayoutNode }[] = [];
  function walk(n: LayoutNode) {
    for (const child of n.children) {
      edges.push({ from: n, to: child });
      walk(child);
    }
  }
  nodes.forEach(walk);
  return edges;
}

// ──────────────────────────────────────────
// SVG edge component
// ──────────────────────────────────────────

function DAGEdge({
  from,
  to,
}: {
  from: LayoutNode;
  to: LayoutNode;
}) {
  const x1 = from.x + NODE_W / 2;
  const y1 = from.y + NODE_H;
  const x2 = to.x + NODE_W / 2;
  const y2 = to.y;

  const midY = (y1 + y2) / 2;

  const d = `M ${x1} ${y1} C ${x1} ${midY}, ${x2} ${midY}, ${x2} ${y2}`;

  return (
    <path
      d={d}
      className="dag-edge"
      stroke="var(--border)"
      strokeWidth={1.5}
      fill="none"
      markerEnd="url(#dag-arrow)"
    />
  );
}

// ──────────────────────────────────────────
// DAG node component
// ──────────────────────────────────────────

function DAGNode({
  layout,
  selected,
  onClick,
}: {
  layout: LayoutNode;
  selected: boolean;
  onClick: () => void;
}) {
  const { node } = layout;
  const color = kindColor(node.span.kind);
  const isError = node.span.status === SpanStatus.ERROR;
  const label = kindLabel(node.span.kind);
  const costUsd = node.subtreeCostUsd;
  const tokens = node.subtreeTokens;

  return (
    <g
      transform={`translate(${layout.x}, ${layout.y})`}
      className={`dag-node-group ${selected ? "dag-node-selected" : ""}`}
      onClick={onClick}
      style={{ cursor: "pointer" }}
    >
      {/* Glow effect for selected */}
      {selected && (
        <rect
          x={-4}
          y={-4}
          width={NODE_W + 8}
          height={NODE_H + 8}
          rx={16}
          ry={16}
          className="dag-node-glow"
          fill="none"
          stroke="var(--accent)"
          strokeWidth={2}
          opacity={0.6}
        />
      )}

      {/* Main card */}
      <rect
        width={NODE_W}
        height={NODE_H}
        rx={12}
        ry={12}
        className="dag-node-bg"
        fill="var(--bg-elevated)"
        stroke={isError ? "var(--error)" : selected ? "var(--accent)" : "var(--border)"}
        strokeWidth={isError ? 1.5 : 1}
      />

      {/* Left color accent bar */}
      <rect
        x={0}
        y={0}
        width={4}
        height={NODE_H}
        rx={2}
        fill={color}
        opacity={0.8}
      />

      {/* Kind badge */}
      <rect
        x={BADGE_X}
        y={BADGE_Y}
        width={label.length * BADGE_CHAR_WIDTH + BADGE_PADDING}
        height={BADGE_HEIGHT}
        rx={4}
        fill={color}
        opacity={0.15}
      />
      <text
        x={BADGE_X + (label.length * BADGE_CHAR_WIDTH + BADGE_PADDING) / 2}
        y={22}
        textAnchor="middle"
        className="dag-node-kind"
        fill={color}
        fontSize={10}
        fontWeight={600}
      >
        {label.toUpperCase()}
      </text>

      {/* Error badge */}
      {isError && (
        <>
          <rect
            x={NODE_W - ERR_BADGE_OFFSET}
            y={BADGE_Y}
            width={ERR_BADGE_WIDTH}
            height={BADGE_HEIGHT}
            rx={4}
            fill="var(--error)"
            opacity={0.15}
          />
          <text
            x={NODE_W - ERR_BADGE_OFFSET + ERR_BADGE_WIDTH / 2}
            y={22}
            textAnchor="middle"
            fill="var(--error)"
            fontSize={9}
            fontWeight={600}
          >
            ERR
          </text>
        </>
      )}

      {/* Span name */}
      <text
        x={14}
        y={44}
        className="dag-node-name"
        fill="var(--text-primary)"
        fontSize={12}
        fontWeight={500}
      >
        {node.span.name.length > SPAN_NAME_MAX_LEN
          ? node.span.name.slice(0, SPAN_NAME_TRUNC_LEN) + "…"
          : node.span.name}
      </text>

      {/* Bottom metrics row */}
      <text
        x={14}
        y={62}
        className="dag-node-metric"
        fill="var(--text-muted)"
        fontSize={10}
        fontFamily="'SF Mono', monospace"
      >
        {node.durationMs.toFixed(0)}ms
        {tokens > 0 ? ` · ${tokens.toLocaleString()} tok` : ""}
        {costUsd > 0 ? ` · $${costUsd.toFixed(4)}` : ""}
      </text>
    </g>
  );
}

// ──────────────────────────────────────────
// Main AgentDAG component
// ──────────────────────────────────────────

export function AgentDAG({
  tree,
  selectedSpanId,
  onSelectSpan,
}: {
  tree: SpanNode[];
  selectedSpanId: string | null;
  onSelectSpan: (spanId: string) => void;
}) {
  const layout = useMemo(() => layoutTree(tree, 40, 40), [tree]);
  const allNodes = useMemo(() => flattenLayout(layout), [layout]);
  const edges = useMemo(() => collectEdges(layout), [layout]);

  // Compute SVG viewBox dimensions
  const { svgW, svgH } = useMemo(() => {
    let maxX = 0;
    let maxY = 0;
    for (const n of allNodes) {
      maxX = Math.max(maxX, n.x + NODE_W);
      maxY = Math.max(maxY, n.y + NODE_H);
    }
    return { svgW: maxX + 80, svgH: maxY + 80 };
  }, [allNodes]);

  const handleClick = useCallback(
    (spanId: string) => () => onSelectSpan(spanId),
    [onSelectSpan]
  );

  if (tree.length === 0) {
    return (
      <div className="empty-state">
        <div className="empty-state-icon">🌳</div>
        <div className="empty-state-title">No spans to visualize</div>
      </div>
    );
  }

  return (
    <div className="dag-container">
      <svg
        width={svgW}
        height={svgH}
        viewBox={`0 0 ${svgW} ${svgH}`}
        className="dag-svg"
      >
        <defs>
          {/* Arrow marker for edges */}
          <marker
            id="dag-arrow"
            viewBox="0 0 10 10"
            refX={10}
            refY={5}
            markerWidth={6}
            markerHeight={6}
            orient="auto-start-reverse"
          >
            <path d="M 0 0 L 10 5 L 0 10 z" fill="var(--text-muted)" opacity={0.5} />
          </marker>
        </defs>

        {/* Edges */}
        <g className="dag-edges">
          {edges.map((edge) => (
            <DAGEdge key={`${edge.from.node.span.spanId}-${edge.to.node.span.spanId}`} from={edge.from} to={edge.to} />
          ))}
        </g>

        {/* Nodes */}
        <g className="dag-nodes">
          {allNodes.map((ln) => (
            <DAGNode
              key={ln.node.span.spanId}
              layout={ln}
              selected={ln.node.span.spanId === selectedSpanId}
              onClick={handleClick(ln.node.span.spanId)}
            />
          ))}
        </g>
      </svg>
    </div>
  );
}

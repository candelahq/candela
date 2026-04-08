"use client";

import { useRef, useState, useCallback, useMemo } from "react";

// ──────────────────────────────────────────
// Types
// ──────────────────────────────────────────

export interface DataPoint {
  label: string; // Display label (e.g. "Apr 8", "14:00")
  value: number;
}

export interface AreaChartProps {
  data: DataPoint[];
  height?: number;
  color?: string; // CSS color for the line/fill
  formatValue?: (value: number) => string;
  emptyMessage?: string;
}

// ──────────────────────────────────────────
// Helpers
// ──────────────────────────────────────────

function buildPath(
  points: { x: number; y: number }[],
  smoothing = 0.2
): string {
  if (points.length < 2) return "";

  const line = (a: { x: number; y: number }, b: { x: number; y: number }) => ({
    length: Math.sqrt((b.x - a.x) ** 2 + (b.y - a.y) ** 2),
    angle: Math.atan2(b.y - a.y, b.x - a.x),
  });

  const controlPoint = (
    current: { x: number; y: number },
    previous: { x: number; y: number } | undefined,
    next: { x: number; y: number } | undefined,
    reverse = false
  ) => {
    const p = previous || current;
    const n = next || current;
    const l = line(p, n);
    const angle = l.angle + (reverse ? Math.PI : 0);
    const length = l.length * smoothing;
    return {
      x: current.x + Math.cos(angle) * length,
      y: current.y + Math.sin(angle) * length,
    };
  };

  let d = `M ${points[0].x},${points[0].y}`;

  for (let i = 1; i < points.length; i++) {
    const cp1 = controlPoint(points[i - 1], points[i - 2], points[i]);
    const cp2 = controlPoint(points[i], points[i - 1], points[i + 1], true);
    d += ` C ${cp1.x},${cp1.y} ${cp2.x},${cp2.y} ${points[i].x},${points[i].y}`;
  }

  return d;
}

// ──────────────────────────────────────────
// Component
// ──────────────────────────────────────────

const PADDING = { top: 8, right: 12, bottom: 28, left: 48 };

export function AreaChart({
  data,
  height = 180,
  color = "var(--accent)",
  formatValue = (v) => v.toLocaleString(),
  emptyMessage = "No data",
}: AreaChartProps) {
  const svgRef = useRef<SVGSVGElement>(null);
  const [tooltip, setTooltip] = useState<{
    x: number;
    y: number;
    point: DataPoint;
  } | null>(null);
  const [containerWidth, setContainerWidth] = useState(0);

  // Measure container width
  const containerRef = useCallback((node: HTMLDivElement | null) => {
    if (!node) return;
    const observer = new ResizeObserver((entries) => {
      for (const entry of entries) {
        setContainerWidth(entry.contentRect.width);
      }
    });
    observer.observe(node);
    return () => observer.disconnect();
  }, []);

  // Compute chart geometry
  const chartWidth = containerWidth - PADDING.left - PADDING.right;
  const chartHeight = height - PADDING.top - PADDING.bottom;

  const { points, yTicks, linePath, areaPath } = useMemo(() => {
    if (data.length === 0 || chartWidth <= 0 || chartHeight <= 0) {
      return { points: [], yTicks: [], linePath: "", areaPath: "" };
    }

    const maxVal = Math.max(...data.map((d) => d.value), 1);
    const minVal = 0;
    const range = maxVal - minVal;

    const pts = data.map((d, i) => ({
      x: PADDING.left + (i / Math.max(data.length - 1, 1)) * chartWidth,
      y:
        PADDING.top +
        chartHeight -
        ((d.value - minVal) / range) * chartHeight,
      data: d,
    }));

    // Y-axis ticks (4 ticks)
    const tickCount = 4;
    const ticks = Array.from({ length: tickCount + 1 }, (_, i) => {
      const val = minVal + (range / tickCount) * i;
      const y =
        PADDING.top + chartHeight - ((val - minVal) / range) * chartHeight;
      return { value: val, y };
    });

    const lp = buildPath(pts);
    // Area path = line path + close to bottom
    const ap = lp
      ? `${lp} L ${pts[pts.length - 1].x},${PADDING.top + chartHeight} L ${pts[0].x},${PADDING.top + chartHeight} Z`
      : "";

    return { points: pts, yTicks: ticks, linePath: lp, areaPath: ap };
  }, [data, chartWidth, chartHeight]);

  const handleMouseMove = useCallback(
    (e: React.MouseEvent<SVGSVGElement>) => {
      if (!svgRef.current || points.length === 0) return;
      const rect = svgRef.current.getBoundingClientRect();
      const mouseX = e.clientX - rect.left;

      // Find nearest point
      let nearest = points[0];
      let minDist = Infinity;
      for (const pt of points) {
        const dist = Math.abs(pt.x - mouseX);
        if (dist < minDist) {
          minDist = dist;
          nearest = pt;
        }
      }

      setTooltip({ x: nearest.x, y: nearest.y, point: nearest.data });
    },
    [points]
  );

  if (data.length === 0) {
    return (
      <div className="chart-empty" style={{ height }}>
        <span>{emptyMessage}</span>
      </div>
    );
  }

  // Determine which x-axis labels to show (avoid overlap)
  const labelStep = Math.max(1, Math.floor(data.length / 6));

  return (
    <div ref={containerRef} className="chart-container">
      <svg
        ref={svgRef}
        width="100%"
        height={height}
        onMouseMove={handleMouseMove}
        onMouseLeave={() => setTooltip(null)}
        className="chart-svg"
      >
        <defs>
          <linearGradient id={`grad-${color.replace(/[^a-z0-9]/gi, "")}`} x1="0" y1="0" x2="0" y2="1">
            <stop offset="0%" stopColor={color} stopOpacity={0.3} />
            <stop offset="100%" stopColor={color} stopOpacity={0.02} />
          </linearGradient>
        </defs>

        {/* Grid lines */}
        {yTicks.map((tick, i) => (
          <line
            key={i}
            x1={PADDING.left}
            y1={tick.y}
            x2={PADDING.left + chartWidth}
            y2={tick.y}
            className="chart-grid-line"
          />
        ))}

        {/* Y-axis labels */}
        {yTicks.map((tick, i) => (
          <text
            key={`y-${i}`}
            x={PADDING.left - 8}
            y={tick.y + 3}
            className="chart-axis-label"
            textAnchor="end"
          >
            {formatValue(tick.value)}
          </text>
        ))}

        {/* X-axis labels */}
        {points
          .filter((_, i) => i % labelStep === 0 || i === points.length - 1)
          .map((pt, i) => (
            <text
              key={`x-${i}`}
              x={pt.x}
              y={height - 4}
              className="chart-axis-label"
              textAnchor="middle"
            >
              {pt.data.label}
            </text>
          ))}

        {/* Area fill */}
        {areaPath && (
          <path
            d={areaPath}
            fill={`url(#grad-${color.replace(/[^a-z0-9]/gi, "")})`}
          />
        )}

        {/* Line */}
        {linePath && (
          <path
            d={linePath}
            fill="none"
            stroke={color}
            strokeWidth={2}
            strokeLinejoin="round"
            strokeLinecap="round"
          />
        )}

        {/* Hover indicator */}
        {tooltip && (
          <>
            <line
              x1={tooltip.x}
              y1={PADDING.top}
              x2={tooltip.x}
              y2={PADDING.top + chartHeight}
              className="chart-hover-line"
            />
            <circle
              cx={tooltip.x}
              cy={tooltip.y}
              r={4}
              fill={color}
              stroke="var(--bg-primary)"
              strokeWidth={2}
            />
          </>
        )}
      </svg>

      {/* Tooltip */}
      {tooltip && (
        <div
          className="chart-tooltip"
          style={{
            left: Math.min(tooltip.x, containerWidth - 120),
            top: tooltip.y - 40,
          }}
        >
          <div className="chart-tooltip-value">{formatValue(tooltip.point.value)}</div>
          <div className="chart-tooltip-label">{tooltip.point.label}</div>
        </div>
      )}
    </div>
  );
}

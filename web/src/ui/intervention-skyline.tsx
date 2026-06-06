import { useMemo } from "react";

import type { ActivitySummary, Intervention } from "../api/activity";
import { formatClockTime } from "./format";

type InterventionSkylineProps = {
  summary: ActivitySummary;
  selectedId?: string;
  onSelect: (id: string) => void;
};

// Plot geometry (SVG user units). The component scales to its container via the
// viewBox, so these are just an internal coordinate system.
const VIEW_W = 1416;
const VIEW_H = 410;
const PLOT_LEFT = 16;
const PLOT_RIGHT = 1400;
const BASELINE_Y = 300;
const MARKER_MAX = 250; // px above baseline for a severity-1.0 intervention
const MARKER_MIN_TOP = 44; // never let a marker punch through the top labels
const VOL_MAX_DEPTH = 84; // px below baseline for peak request volume
const AXIS_LABEL_Y = 338;

// Whole-hour gridline positions within the window.
function hourTicks(start: number, end: number): number[] {
  const ticks: number[] = [];
  const first = new Date(start);
  first.setMinutes(0, 0, 0);
  if (first.getTime() < start) {
    first.setHours(first.getHours() + 1);
  }
  for (let t = first.getTime(); t <= end; t += 60 * 60 * 1000) {
    ticks.push(t);
  }
  return ticks;
}

export function InterventionSkyline({ summary, selectedId, onSelect }: InterventionSkylineProps) {
  const { rangeStartUnixMilli: start, rangeEndUnixMilli: end } = summary;

  const xFor = useMemo(() => {
    const span = Math.max(1, end - start);
    return (time: number) => PLOT_LEFT + ((time - start) / span) * (PLOT_RIGHT - PLOT_LEFT);
  }, [start, end]);

  const volumePath = useMemo(() => {
    if (summary.volume.length === 0) {
      return "";
    }
    const maxVolume = Math.max(...summary.volume.map((point) => point.volume), 0.0001);
    const segments = summary.volume.map((point) => {
      const x = xFor(point.timeUnixMilli);
      const y = BASELINE_Y + (point.volume / maxVolume) * VOL_MAX_DEPTH;
      return `L ${x.toFixed(1)},${y.toFixed(1)}`;
    });
    return `M ${PLOT_LEFT},${BASELINE_Y} ${segments.join(" ")} L ${PLOT_RIGHT},${BASELINE_Y} Z`;
  }, [summary.volume, xFor]);

  const ticks = useMemo(() => hourTicks(start, end), [start, end]);

  return (
    <svg
      className="activity-skyline__svg"
      width="100%"
      viewBox={`0 0 ${VIEW_W} ${VIEW_H}`}
      preserveAspectRatio="xMidYMid meet"
      role="group"
      aria-label="Intervention skyline"
    >
      {/* Baseline */}
      <line x1={PLOT_LEFT} y1={BASELINE_Y} x2={PLOT_RIGHT} y2={BASELINE_Y} className="activity-skyline__baseline" />

      {/* Request volume area, hanging below the baseline */}
      <path d={volumePath} className="activity-skyline__volume" />

      {/* Hour gridlines + labels */}
      {ticks.map((tick) => {
        const x = xFor(tick);
        return (
          <g key={tick}>
            <line x1={x} y1={28} x2={x} y2={BASELINE_Y} className="activity-skyline__grid" />
            <text x={x} y={AXIS_LABEL_Y} className="activity-skyline__tick" textAnchor="middle">
              {formatClockTime(tick)}
            </text>
          </g>
        );
      })}

      {/* Intervention markers */}
      {summary.interventions.map((item) => (
        <Marker
          key={item.id}
          item={item}
          x={xFor(item.timestampUnixMilli)}
          selected={item.id === selectedId}
          onSelect={onSelect}
        />
      ))}
    </svg>
  );
}

type MarkerProps = {
  item: Intervention;
  x: number;
  selected: boolean;
  onSelect: (id: string) => void;
};

function Marker({ item, x, selected, onSelect }: MarkerProps) {
  const topY = Math.max(MARKER_MIN_TOP, BASELINE_Y - item.severity * MARKER_MAX);
  const color = `var(--c-${item.category})`;
  const showLabel = Boolean(item.label) || selected;

  return (
    <g
      className={selected ? "activity-skyline__marker activity-skyline__marker--selected" : "activity-skyline__marker"}
      onClick={() => onSelect(item.id)}
      role="button"
      tabIndex={0}
      onKeyDown={(event) => {
        if (event.key === "Enter" || event.key === " ") {
          event.preventDefault();
          onSelect(item.id);
        }
      }}
      aria-label={`${item.title} (${item.category})`}
    >
      {/* Wide transparent hit target */}
      <rect x={x - 12} y={topY - 18} width={24} height={BASELINE_Y - topY + 18} fill="transparent" />
      <line x1={x} y1={BASELINE_Y} x2={x} y2={topY} stroke={color} className="activity-skyline__stem" />
      <circle cx={x} cy={topY} r={11} fill={color} className="activity-skyline__halo" />
      <circle cx={x} cy={topY} r={5} fill={color} />
      <circle cx={x} cy={topY} r={5} fill="none" className="activity-skyline__dot-ring" />
      {showLabel ? (
        <text x={x} y={topY - 16} fill={color} className="activity-skyline__label" textAnchor="middle">
          {item.label ?? item.title}
        </text>
      ) : null}
    </g>
  );
}

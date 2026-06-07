import { type PointerEvent as ReactPointerEvent, useEffect, useMemo, useRef, useState } from "react";

import { type ActivitySummary, type Intervention, categoryLabels } from "../api/activity";
import { CategoryIcon } from "./category-icon";
import { formatClockTime } from "./format";

type InterventionSkylineProps = {
  summary: ActivitySummary;
  pinnedId?: string;
  onPin: (id: string | null) => void;
  onZoom: (startUnixMilli: number, endUnixMilli: number) => void;
  onZoomOut: () => void;
};

// Minimum horizontal drag (in viewBox units) that counts as a zoom rather than a click.
const MIN_DRAG_VBX = 8;

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

function markerTopY(severity: number): number {
  return Math.max(MARKER_MIN_TOP, BASELINE_Y - severity * MARKER_MAX);
}

// viewBoxXToTime inverts the time→x plot mapping: a viewBox x-coordinate within the
// plot maps back to a timestamp in [start, end]. Exported for unit testing the zoom.
export function viewBoxXToTime(start: number, end: number, vbX: number): number {
  const clamped = Math.min(PLOT_RIGHT, Math.max(PLOT_LEFT, vbX));
  return start + ((clamped - PLOT_LEFT) / (PLOT_RIGHT - PLOT_LEFT)) * (end - start);
}

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

export function InterventionSkyline({ summary, pinnedId, onPin, onZoom, onZoomOut }: InterventionSkylineProps) {
  const { rangeStartUnixMilli: start, rangeEndUnixMilli: end } = summary;
  const [hoveredId, setHoveredId] = useState<string>();
  const svgRef = useRef<SVGSVGElement>(null);
  // Drag selection in viewBox x-units; null when not dragging.
  const [drag, setDrag] = useState<{ from: number; to: number } | null>(null);

  // Pinned selection wins; otherwise the currently hovered/focused marker shows.
  const activeId = pinnedId ?? hoveredId;

  // Dismiss a pinned popover with Escape.
  useEffect(() => {
    if (!pinnedId) {
      return;
    }
    const onKey = (event: KeyboardEvent) => {
      if (event.key === "Escape") {
        onPin(null);
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, [pinnedId, onPin]);

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
  const active = useMemo(
    () => summary.interventions.find((item) => item.id === activeId),
    [summary.interventions, activeId],
  );

  // Convert a pointer's client X into a clamped viewBox x-coordinate. The SVG fills
  // the container width, so the horizontal ratio maps directly onto the viewBox.
  const clientToVbX = (clientX: number): number => {
    const rect = svgRef.current?.getBoundingClientRect();
    if (!rect || rect.width === 0) {
      return PLOT_LEFT;
    }
    const vbX = ((clientX - rect.left) / rect.width) * VIEW_W;
    return Math.min(PLOT_RIGHT, Math.max(PLOT_LEFT, vbX));
  };

  // Invert xFor: viewBox x → timestamp.
  const timeForVbX = (vbX: number): number => viewBoxXToTime(start, end, vbX);

  const handlePointerDown = (event: ReactPointerEvent<SVGRectElement>) => {
    event.preventDefault();
    event.currentTarget.setPointerCapture(event.pointerId);
    const vbX = clientToVbX(event.clientX);
    setDrag({ from: vbX, to: vbX });
  };

  const handlePointerMove = (event: ReactPointerEvent<SVGRectElement>) => {
    if (!drag) {
      return;
    }
    setDrag({ from: drag.from, to: clientToVbX(event.clientX) });
  };

  const handlePointerUp = (event: ReactPointerEvent<SVGRectElement>) => {
    if (!drag) {
      return;
    }
    event.currentTarget.releasePointerCapture(event.pointerId);
    const { from, to } = drag;
    setDrag(null);
    if (Math.abs(to - from) < MIN_DRAG_VBX) {
      return; // treat as a click, not a zoom
    }
    const t0 = timeForVbX(Math.min(from, to));
    const t1 = timeForVbX(Math.max(from, to));
    if (t1 - t0 >= 1000) {
      onZoom(Math.round(t0), Math.round(t1));
    }
  };

  const selectLeft = drag ? Math.min(drag.from, drag.to) : 0;
  const selectWidth = drag ? Math.abs(drag.to - drag.from) : 0;

  return (
    <div className="activity-skyline" onMouseLeave={() => setHoveredId(undefined)}>
      <svg
        ref={svgRef}
        className="activity-skyline__svg"
        width="100%"
        viewBox={`0 0 ${VIEW_W} ${VIEW_H}`}
        preserveAspectRatio="xMidYMid meet"
        role="group"
        aria-label="Intervention skyline"
        onDoubleClick={onZoomOut}
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

        {/* Drag surface for zoom selection (below markers so they stay clickable) */}
        <rect
          className="activity-skyline__surface"
          x={PLOT_LEFT}
          y={28}
          width={PLOT_RIGHT - PLOT_LEFT}
          height={BASELINE_Y - 28}
          fill="transparent"
          onPointerDown={handlePointerDown}
          onPointerMove={handlePointerMove}
          onPointerUp={handlePointerUp}
        />

        {/* Active drag selection */}
        {drag && selectWidth > 0 ? (
          <rect className="activity-skyline__select" x={selectLeft} y={28} width={selectWidth} height={BASELINE_Y - 28} />
        ) : null}

        {/* Intervention markers */}
        {summary.interventions.map((item) => (
          <Marker
            key={item.id}
            item={item}
            x={xFor(item.timestampUnixMilli)}
            active={item.id === activeId}
            onHover={setHoveredId}
            onPin={onPin}
            pinned={item.id === pinnedId}
          />
        ))}
      </svg>

      {active ? <InterventionPopover intervention={active} x={xFor(active.timestampUnixMilli)} pinned={active.id === pinnedId} onClose={() => onPin(null)} /> : null}

      {summary.interventions.length === 0 ? (
        <p className="activity-skyline__watching">
          Centian is watching — routine traffic will sit here, interventions will rise above. Nothing's
          needed your attention yet.
        </p>
      ) : null}
    </div>
  );
}

type MarkerProps = {
  item: Intervention;
  x: number;
  active: boolean;
  pinned: boolean;
  onHover: (id: string | undefined) => void;
  onPin: (id: string | null) => void;
};

function Marker({ item, x, active, pinned, onHover, onPin }: MarkerProps) {
  const topY = markerTopY(item.severity);
  const color = `var(--c-${item.category})`;

  return (
    <g
      className={active ? "activity-skyline__marker activity-skyline__marker--active" : "activity-skyline__marker"}
      onMouseEnter={() => onHover(item.id)}
      onFocus={() => onHover(item.id)}
      onBlur={() => onHover(undefined)}
      onClick={() => onPin(pinned ? null : item.id)}
      role="button"
      tabIndex={0}
      onKeyDown={(event) => {
        if (event.key === "Enter" || event.key === " ") {
          event.preventDefault();
          onPin(pinned ? null : item.id);
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
      {item.label ? (
        <text x={x} y={topY - 16} fill={color} className="activity-skyline__label" textAnchor="middle">
          {item.label}
        </text>
      ) : null}
    </g>
  );
}

type PopoverProps = {
  intervention: Intervention;
  x: number; // viewBox x of the marker
  pinned: boolean;
  onClose: () => void;
};

// Floating detail card anchored to the active marker. Positioned with percentage
// coordinates derived from the viewBox, which map exactly because the SVG fills the
// container width and its height scales proportionally.
function InterventionPopover({ intervention, x, pinned, onClose }: PopoverProps) {
  const topY = markerTopY(intervention.severity);
  const leftPct = (x / VIEW_W) * 100;
  const topPct = (topY / VIEW_H) * 100;
  const nearRight = leftPct > 62;
  const nearLeft = leftPct < 18;
  const placeBelow = topY < 150;
  const translateX = nearRight ? "-88%" : nearLeft ? "-12%" : "-50%";
  const translateY = placeBelow ? "18px" : "calc(-100% - 16px)";

  return (
    <div
      className={`activity-skyline__popover activity-skyline__popover--${intervention.category}`}
      style={{ left: `${leftPct}%`, top: `${topPct}%`, transform: `translate(${translateX}, ${translateY})` }}
      role="dialog"
      aria-label={intervention.title}
    >
      <div className="activity-skyline__popover-head">
        <span className={`activity__pill activity__pill--${intervention.category}`}>
          <CategoryIcon category={intervention.category} />
          {categoryLabels[intervention.category]}
        </span>
        <span className="activity-skyline__popover-time">{formatClockTime(intervention.timestampUnixMilli)}</span>
        {pinned ? (
          <button type="button" className="activity-skyline__popover-close" onClick={onClose} aria-label="Dismiss">
            ×
          </button>
        ) : null}
      </div>
      <div className="activity-skyline__popover-title">{intervention.title}</div>
      <div className="activity-skyline__popover-summary">{intervention.summary}</div>
      {intervention.ruleId || intervention.ruleExplanation ? (
        <div className="activity-skyline__popover-rule">
          {intervention.ruleId ? <div className="activity-skyline__popover-rule-id">{intervention.ruleId}</div> : null}
          {intervention.ruleExplanation ? (
            <div className="activity-skyline__popover-rule-text">{intervention.ruleExplanation}</div>
          ) : null}
        </div>
      ) : null}
      <div className="activity-skyline__popover-footer">
        <span className="activity-skyline__popover-tool">{intervention.toolName}</span>
        <span className="activity-skyline__popover-sep">·</span>
        <span className="activity-skyline__popover-target">
          {intervention.gateway}/{intervention.serverName}
        </span>
      </div>
    </div>
  );
}

import type { TaskRunEvent } from "../api/task-runs";
import {
  formatLatency,
  formatTraceTimestamp,
  getExchangeLatency,
  getExchangeServerName,
  getTimelineAnchorEvent,
  getTimelineItemAlertLabel,
  getTimelineItemSubtitle,
  getTimelineItemTitle,
  getTimelineItemTone,
  type TimelineGroup,
  type TimelineItem,
} from "./task-run-detail-page";

// ── CSS animations ──────────────────────────────────────────────────────
const SCI_FI_STYLES = `
  @import url('https://fonts.googleapis.com/css2?family=Share+Tech+Mono&display=swap');

  .ctv * { box-sizing: border-box; }

  @keyframes breathe {
    0%, 100% { transform: scale(1);    opacity: 0.2; }
    50%       { transform: scale(1.65); opacity: 0.06; }
  }

  .sci-node { cursor: pointer; transition: transform 0.18s cubic-bezier(.34,1.56,.64,1); }
  .sci-node:hover { transform: scale(1.08); }
  .sci-node:hover .sci-outer  { animation: breathe 1.1s ease-in-out infinite !important; opacity: 0.38 !important; }
  .sci-node:hover .sci-tag    { border-color: rgba(255,255,255,0.14) !important; background: rgba(255,255,255,0.04) !important; }
  .sci-node:hover .sci-conn   { opacity: 0.55 !important; width: 24px !important; }
  .sci-node:hover .sci-ts     { color: #5a7a9a !important; }
  .sci-node--selected .sci-tag { border-color: rgba(255,255,255,0.2) !important; background: rgba(255,255,255,0.06) !important; }

  .grid-bg {
    background-image:
      radial-gradient(circle, rgba(30,40,80,0.35) 1px, transparent 1px);
    background-size: 28px 28px;
  }
`;

// ── Color tokens ────────────────────────────────────────────────────────
type ColorToken = {
  color: string;
  bg: string;
  glow: string;
  dim: string;
};

const KNOWN_COLORS: Record<string, ColorToken> = {
  centian: { color: "#a78bfa", bg: "rgba(167,139,250,0.1)", glow: "rgba(167,139,250,0.6)", dim: "#3b2e6e" },
  shell: { color: "#fbbf24", bg: "rgba(251,191,36,0.1)", glow: "rgba(251,191,36,0.6)", dim: "#6b4f10" },
  filesystem: { color: "#34d399", bg: "rgba(52,211,153,0.1)", glow: "rgba(52,211,153,0.6)", dim: "#0e4a35" },
  error: { color: "#f87171", bg: "rgba(248,113,113,0.12)", glow: "rgba(248,113,113,0.7)", dim: "#5c1e1e" },
};

const FALLBACK_PALETTE = [
  "#a78bfa", "#fbbf24", "#34d399", "#60a5fa",
  "#fb7185", "#22d3ee", "#f97316", "#4ade80",
];

function getColorToken(serverName: string): ColorToken {
  const known = KNOWN_COLORS[serverName];
  if (known) return known;

  let hash = 0;
  for (let i = 0; i < serverName.length; i++) {
    hash = (hash * 31 + serverName.charCodeAt(i)) >>> 0;
  }
  const hex = FALLBACK_PALETTE[hash % FALLBACK_PALETTE.length];
  return {
    color: hex,
    bg: `${hex}1a`,
    glow: `${hex}99`,
    dim: `${hex}44`,
  };
}

function getItemColorToken(item: TimelineItem): ColorToken {
  const tone = getTimelineItemTone(item);
  if (tone === "failed") return KNOWN_COLORS.error;

  if (item.kind === "task") return KNOWN_COLORS.centian;
  const serverName = getExchangeServerName(item.exchange);
  return getColorToken(serverName);
}

// ── Shape ───────────────────────────────────────────────────────────────
function NodeShape({ server, size, color }: { server: string; size: number; color: string }) {
  if (server === "centian" || server === "task") {
    return (
      <div style={{ width: size, height: size, clipPath: "polygon(50% 0%,100% 25%,100% 75%,50% 100%,0% 75%,0% 25%)", background: color }} />
    );
  }
  if (server === "filesystem") {
    return (
      <div style={{ width: size * 0.76, height: size * 0.76, background: color, transform: "rotate(45deg)", borderRadius: 2 }} />
    );
  }
  return <div style={{ width: size, height: size, borderRadius: "50%", background: color }} />;
}

// ── Derive group status from its items ──────────────────────────────────
function deriveGroupStatus(group: TimelineGroup): string {
  const items = group.items;
  for (let i = items.length - 1; i >= 0; i--) {
    const item = items[i];
    if (item.kind === "task") {
      const tone = getTimelineItemTone(item);
      if (tone === "failed") return "failed";
      if (tone === "completed") return "passed";
      if (tone === "active") return "active";
    }
  }

  const hasFailed = items.some((item) => getTimelineItemTone(item) === "failed");
  if (hasFailed) return "failed";

  return "info";
}

// ── Sector divider ──────────────────────────────────────────────────────
function SciFiSectorDivider({
  group,
  groupIndex,
  onToggle,
  collapsed,
}: {
  group: TimelineGroup;
  groupIndex: number;
  onToggle: () => void;
  collapsed: boolean;
}) {
  const status = deriveGroupStatus(group);
  const statusColor = status === "passed" ? "#34d399" : status === "failed" ? "#f87171" : "#a78bfa";
  const stepColors = ["#a78bfa", "#34d399", "#fbbf24", "#60a5fa", "#fb7185", "#22d3ee"];
  const accentColor = stepColors[groupIndex % stepColors.length];

  return (
    <button
      type="button"
      onClick={onToggle}
      style={{
        position: "relative", margin: "10px 0 4px", padding: "0 0 0 120px",
        display: "block", width: "100%", background: "none", border: "none",
        cursor: "pointer", textAlign: "left",
      }}
    >
      {/* Full-width horizontal rule */}
      <div style={{ position: "relative", height: 1, background: `linear-gradient(to right, transparent, ${accentColor}44, ${accentColor}22, transparent)`, marginBottom: 8 }}>
        <div style={{ position: "absolute", inset: 0, background: `linear-gradient(to right, transparent, ${accentColor}, transparent)`, opacity: 0.15 }} />
      </div>

      <div style={{ display: "flex", alignItems: "center", gap: 12 }}>
        {/* Corner bracket */}
        <div style={{
          width: 10, height: 10, flexShrink: 0,
          borderTop: `1px solid ${accentColor}`,
          borderLeft: `1px solid ${accentColor}`,
          opacity: 0.7,
        }} />

        <span style={{ fontFamily: "'Share Tech Mono', 'Courier New', monospace", fontSize: 11, color: accentColor, letterSpacing: "0.2em", textTransform: "uppercase", opacity: 0.8 }}>
          SECTOR {String(groupIndex + 1).padStart(2, "0")}
        </span>
        <span style={{ fontFamily: "'Share Tech Mono', 'Courier New', monospace", fontSize: 12, color: "#6a8ab0", letterSpacing: "0.08em" }}>
          {group.label}
        </span>
        <span style={{ fontFamily: "'Share Tech Mono', 'Courier New', monospace", fontSize: 10, color: "#3d4a6a", opacity: 0.7 }}>
          {group.items.length} events
        </span>
        <div style={{ flex: 1, height: 1, background: `linear-gradient(to right, ${accentColor}20, transparent)` }} />
        <span style={{
          fontFamily: "'Share Tech Mono', 'Courier New', monospace", fontSize: 10,
          color: statusColor, letterSpacing: "0.15em", textTransform: "uppercase",
          padding: "2px 8px", border: `1px solid ${statusColor}40`,
          borderRadius: 2, background: `${statusColor}0d`,
        }}>
          {collapsed ? "+" : "−"} {status}
        </span>

        <div style={{
          width: 10, height: 10, flexShrink: 0,
          borderTop: `1px solid ${accentColor}`,
          borderRight: `1px solid ${accentColor}`,
          opacity: 0.7,
        }} />
      </div>
    </button>
  );
}

// ── Event node ──────────────────────────────────────────────────────────
function SciFiEventNode({
  item,
  onSelect,
  selected,
}: {
  item: TimelineItem;
  onSelect: (id: string) => void;
  selected: boolean;
}) {
  const serverName = item.kind === "task" ? "centian" : getExchangeServerName(item.exchange);
  const vc = getItemColorToken(item);
  const anchorEvent = getTimelineAnchorEvent(item);
  const title = getTimelineItemTitle(item);
  const subtitle = getTimelineItemSubtitle(item);
  const tone = getTimelineItemTone(item);
  const alertLabel = getTimelineItemAlertLabel(item);
  const exchangeLatency = item.kind === "exchange" ? getExchangeLatency(item.exchange) : undefined;

  // Outer ring shape mirrors inner shape
  const outerStyle = serverName === "centian"
    ? { clipPath: "polygon(50% 0%,100% 25%,100% 75%,50% 100%,0% 75%,0% 25%)" }
    : serverName === "filesystem"
    ? { transform: "rotate(45deg)", borderRadius: 3 }
    : { borderRadius: "50%" };

  return (
    <button
      type="button"
      className={`sci-node ${selected ? "sci-node--selected" : ""}`}
      aria-label={`Show event details for ${title}`}
      aria-pressed={selected}
      onClick={() => onSelect(item.id)}
      style={{ display: "flex", alignItems: "center", minHeight: 50, position: "relative", background: "none", border: "none", cursor: "pointer", width: "100%", padding: 0, textAlign: "left" }}
    >
      {/* Timestamp */}
      <div className="sci-ts" style={{
        width: 80, textAlign: "right", paddingRight: 0, flexShrink: 0,
        fontFamily: "'Share Tech Mono', 'Courier New', monospace",
        fontSize: 11, color: "#3a5070", letterSpacing: "0.02em",
        transition: "color 0.2s",
      }}>
        {formatTraceTimestamp(anchorEvent.createdAtUnixMilli)}
      </div>

      {/* Timeline column with bubble */}
      <div style={{ width: 44, display: "flex", alignItems: "center", justifyContent: "center", flexShrink: 0, position: "relative", zIndex: 2 }}>
        {/* Outer halo */}
        <div className="sci-outer" style={{
          position: "absolute",
          width: 40, height: 40,
          background: vc.bg,
          ...outerStyle,
          opacity: 0.15,
          transition: "opacity 0.2s",
        }} />
        {/* Middle ring */}
        <div style={{
          position: "absolute",
          width: 24, height: 24,
          border: `1px solid ${vc.color}55`,
          boxShadow: `0 0 10px ${vc.glow}, inset 0 0 8px ${vc.bg}`,
          ...outerStyle,
        }} />
        {/* Core shape */}
        <NodeShape server={serverName} size={12} color={vc.color} />
        {/* Progress pip */}
        <div style={{
          position: "absolute", bottom: -2, right: 0,
          width: 4, height: 4, borderRadius: "50%",
          background: "#1a2540",
          boxShadow: "0 0 0 1px #0d1525",
        }} />
      </div>

      {/* Connector + label tag */}
      <div style={{ flex: 1, display: "flex", alignItems: "center", paddingLeft: 0 }}>
        {/* Connector line */}
        <div className="sci-conn" style={{
          width: 16, height: 1, flexShrink: 0,
          background: `linear-gradient(to right, ${vc.color}, ${vc.color}44)`,
          opacity: 0.2, transition: "all 0.2s",
        }} />

        {/* Label pill */}
        <div className="sci-tag" style={{
          border: `1px solid ${vc.color}18`,
          borderLeft: `2px solid ${vc.color}99`,
          borderRadius: "0 4px 4px 0",
          padding: "6px 14px 6px 12px",
          background: "transparent",
          transition: "all 0.18s",
          flex: 1,
          maxWidth: 520,
        }}>
          <div style={{ display: "flex", alignItems: "center", gap: 8, flexWrap: "nowrap" }}>
            {/* Server dot */}
            <div style={{
              width: 5, height: 5, borderRadius: "50%", flexShrink: 0,
              background: vc.color,
              boxShadow: `0 0 6px ${vc.glow}`,
            }} />
            {/* Server name */}
            <span style={{
              fontFamily: "'Share Tech Mono', 'Courier New', monospace",
              fontSize: 10, color: vc.color, letterSpacing: "0.12em",
              textTransform: "uppercase", opacity: 0.8, flexShrink: 0,
            }}>
              {item.kind === "task" ? "task" : serverName}
            </span>
            {/* Tool name */}
            <span
              data-testid="timeline-event-title"
              style={{
                fontFamily: "'Share Tech Mono', 'Courier New', monospace",
                fontSize: 13, color: "#b0c8e8", fontWeight: "normal",
                overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap",
              }}
            >
              {item.kind === "exchange" ? `${serverName} - ${title}` : title}
            </span>
            {/* Error badge */}
            {tone === "failed" && alertLabel && (
              <span style={{ fontSize: 9, color: "#f87171", background: "#f871710d", border: "1px solid #f8717133", padding: "1px 6px", borderRadius: 2, letterSpacing: "0.12em", flexShrink: 0, textTransform: "uppercase" }}>
                {alertLabel}
              </span>
            )}
            {/* Duration */}
            {exchangeLatency != null && exchangeLatency > 0 && (
              <span style={{ fontFamily: "monospace", fontSize: 10, color: "#3a5070", marginLeft: "auto", flexShrink: 0, paddingRight: 2 }}>
                {formatLatency(exchangeLatency)}
              </span>
            )}
          </div>
          {/* Subtitle */}
          {subtitle ? (
            <div style={{
              fontFamily: "'Share Tech Mono', 'Courier New', monospace",
              fontSize: 11, color: "#4a6a8e", marginTop: 3,
              overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap",
              maxWidth: 460,
            }}>
              {subtitle}
            </div>
          ) : null}
        </div>
      </div>
    </button>
  );
}

// ── Main container ──────────────────────────────────────────────────────
export function SciFiTimeline({
  groups,
  collapsedGroups,
  onToggleGroup,
  onSelectItem,
  selectedItemId,
  events,
}: {
  groups: TimelineGroup[];
  collapsedGroups: Record<string, boolean>;
  onToggleGroup: (key: string) => void;
  onSelectItem: (id: string) => void;
  selectedItemId: string;
  events: TaskRunEvent[];
}) {
  const hasCompleted = events.length > 0 && events.some(
    (e) => e.source === "task" && (e.eventType === "task_completed" || (e.payloadJson as { status?: string } | null)?.status === "completed")
  );

  return (
    <div className="ctv" style={{
      background: "#020210",
      color: "#8ba4c8",
      fontFamily: "'Share Tech Mono','Courier New',monospace",
      position: "relative",
      overflow: "hidden",
      flex: 1,
      minHeight: 0,
    }}>
      <style dangerouslySetInnerHTML={{ __html: SCI_FI_STYLES }} />

      {/* Dot grid background */}
      <div className="grid-bg" style={{ position: "absolute", inset: 0, pointerEvents: "none", zIndex: 0 }} />

      {/* Radial depth vignette */}
      <div style={{
        position: "absolute", inset: 0, pointerEvents: "none", zIndex: 0,
        background: "radial-gradient(ellipse at 50% 30%, transparent 40%, rgba(1,1,16,0.7) 100%)",
      }} />

      {/* Content */}
      <div style={{ position: "relative", zIndex: 1, padding: "24px 24px 80px", overflowY: "auto", height: "100%" }}>

        {/* Server legend */}
        <div style={{ display: "flex", gap: 24, marginBottom: 16, paddingLeft: 120 }}>
          {Object.entries({ centian: "hexagon", shell: "circle", filesystem: "diamond" }).map(([srv]) => (
            <div key={srv} style={{ display: "flex", alignItems: "center", gap: 6, fontSize: 10, color: KNOWN_COLORS[srv].color, opacity: 0.7, letterSpacing: "0.1em", fontFamily: "'Share Tech Mono', 'Courier New', monospace" }}>
              <NodeShape server={srv} size={10} color={KNOWN_COLORS[srv].color} />
              <span>{srv}</span>
            </div>
          ))}
        </div>

        {/* ── Timeline ── */}
        <div style={{ position: "relative" }}>
          {/* Vertical timeline line */}
          <div style={{
            position: "absolute",
            left: 119, top: 0, bottom: 0, width: 1,
            background: "linear-gradient(to bottom, transparent, #1e2a5a44 5%, #1e2a5a44 95%, transparent)",
            zIndex: 1,
          }} />
          {/* Glow line */}
          <div style={{
            position: "absolute",
            left: 119, top: 0, bottom: 0, width: 1,
            background: "linear-gradient(to bottom, transparent, #a78bfa18 10%, #a78bfa10 90%, transparent)",
            boxShadow: "0 0 8px rgba(167,139,250,0.08)",
            zIndex: 1,
          }} />
          {/* Groups */}
          {groups.map((group, groupIndex) => (
            <section key={group.key} aria-label={group.label}>
              <SciFiSectorDivider
                group={group}
                groupIndex={groupIndex}
                onToggle={() => onToggleGroup(group.key)}
                collapsed={!!collapsedGroups[group.key]}
              />
              {!collapsedGroups[group.key] && group.items.map((item) => (
                <SciFiEventNode
                  key={item.id}
                  item={item}
                  onSelect={onSelectItem}
                  selected={selectedItemId === item.id}
                />
              ))}
            </section>
          ))}

          {/* End cap */}
          {hasCompleted && (
            <div style={{ display: "flex", alignItems: "center", paddingLeft: 76, marginTop: 8, gap: 12 }}>
              <div style={{ width: 44, display: "flex", justifyContent: "center" }}>
                <div style={{ width: 8, height: 8, background: "#34d399", clipPath: "polygon(50% 0%,100% 50%,50% 100%,0% 50%)", boxShadow: "0 0 10px rgba(52,211,153,0.6)" }} />
              </div>
              <span style={{ fontSize: 9, color: "#34d399", letterSpacing: "0.25em", opacity: 0.6 }}>SESSION COMPLETE</span>
            </div>
          )}
        </div>
      </div>
    </div>
  );
}

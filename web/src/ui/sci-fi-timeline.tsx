import type { TaskRunEvent } from "../api/task-runs";
import { getServerColorToken, KNOWN_SERVER_COLORS, type ColorToken } from "./server-colors";
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
  GovernanceCategoryIcon,
  getGovernanceCategoryTone,
  getEventAnnotations,
} from "./task-run-detail-page";

// Injected stylesheet for the self-contained sci-fi timeline treatment.
const SCI_FI_STYLES = `
  .ctv * { box-sizing: border-box; }

  @keyframes breathe {
    0%, 100% { transform: scale(1);    opacity: 0.2; }
    50%       { transform: scale(1.65); opacity: 0.06; }
  }

  .sci-node { cursor: pointer; transition: transform 0.18s cubic-bezier(.34,1.56,.64,1); transform-origin: 102px center; }
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

  .sci-sector-status {
    display: inline-flex;
    align-items: center;
    gap: 6px;
  }

  .sci-sector-governance-icons {
    display: inline-flex;
    align-items: center;
    gap: 4px;
    margin-left: 2px;
  }

  .sci-sector-governance-icon {
    display: inline-flex;
    width: 18px;
    height: 18px;
    align-items: center;
    justify-content: center;
    border: 1px solid currentColor;
    border-radius: 999px;
  }

  .sci-sector-governance-icon .task-run-detail__governance-category-icon {
    width: 14px;
    height: 14px;
    color: currentColor;
  }

  .sci-sector-governance-icon .task-run-detail__governance-category-icon svg {
    width: 12px;
    height: 12px;
  }

  .sci-node-governance {
    display: inline-grid;
    gap: 4px;
    align-self: center;
    flex: 0 1 360px;
    margin-left: 10px;
    min-width: 0;
    max-width: min(360px, 30vw);
  }

  .sci-node-governance-row {
    display: grid;
    grid-template-columns: 20px 92px minmax(0, 1fr);
    align-items: center;
    gap: 7px;
    min-width: 0;
    color: #d6e2ff;
    font-family: "IBM Plex Mono", "SFMono-Regular", Menlo, Consolas, monospace;
    font-size: 12px;
    line-height: 1.25;
  }

  .sci-node-governance-icon {
    display: inline-flex;
    width: 18px;
    height: 18px;
    flex: 0 0 auto;
    align-items: center;
    justify-content: center;
    border: 1px solid currentColor;
    border-radius: 999px;
  }

  .sci-node-governance-icon .task-run-detail__governance-category-icon {
    width: 14px;
    height: 14px;
    color: currentColor;
  }

  .sci-node-governance-icon .task-run-detail__governance-category-icon svg {
    width: 12px;
    height: 12px;
  }

  .sci-node-governance-category {
    flex: 0 0 auto;
    font-weight: 700;
  }

  .sci-node-governance-text {
    display: none;
    min-width: 0;
    max-width: 100%;
    padding: 6px 8px;
    border: 1px solid rgba(207, 222, 238, 0.14);
    border-radius: 5px;
    background: rgba(4, 7, 18, 0.82);
    box-shadow: 0 12px 28px rgba(0, 0, 0, 0.28);
    color: #f2f6ff;
    font-family: system-ui, -apple-system, BlinkMacSystemFont, "Segoe UI", sans-serif;
    font-size: 12px;
    font-weight: 500;
    line-height: 1.3;
    overflow-wrap: anywhere;
    white-space: normal;
  }

  .sci-node:hover .sci-node-governance-text,
  .sci-node:focus-visible .sci-node-governance-text {
    display: block;
  }

  @media (max-width: 980px) {
    .sci-node-governance {
      flex-basis: 280px;
      max-width: min(280px, 28vw);
    }

    .sci-node-governance-text {
      font-size: 11px;
    }
  }

  @media (max-width: 760px) {
    .sci-node-governance-text {
      display: none !important;
    }
  }
`;

// Chooses the node color from its tone first, then from the owning server.
function getItemColorToken(item: TimelineItem): ColorToken {
  const tone = getTimelineItemTone(item);
  if (tone === "failed") return KNOWN_SERVER_COLORS.error;

  if (item.kind === "task") return KNOWN_SERVER_COLORS.centian;
  const serverName = getExchangeServerName(item.exchange);
  return getServerColorToken(serverName);
}

// Server summary row shown above the timeline.
type MCPServerLegendEntry = {
  name: string;
  eventCount: number;
};

// Counts exchange traffic per downstream server for the compact legend.
function getMCPServerLegendEntries(groups: TimelineGroup[], events: TaskRunEvent[]): MCPServerLegendEntry[] {
  const serverCounts = new Map<string, number>();

  for (const group of groups) {
    for (const item of group.items) {
      if (item.kind !== "exchange") {
        continue;
      }

      const name = getExchangeServerName(item.exchange).trim();
      if (!name || name === "centian") {
        continue;
      }

      serverCounts.set(name, (serverCounts.get(name) ?? 0) + 1);
    }
  }

  let centianRequestCount = 0;
  // Centian task events are collapsed elsewhere, so count its action requests from the raw event stream.
  for (const event of events) {
    if (
      event.source === "action" &&
      event.serverName === "centian" &&
      (event.messageType === "request" ||
        event.direction === "request" ||
        event.direction === "[CLIENT -> SERVER]")
    ) {
      centianRequestCount += 1;
    }
  }

  if (centianRequestCount > 0) {
    serverCounts.set("centian", centianRequestCount);
  }

  return [...serverCounts.entries()].map(([name, eventCount]) => ({ name, eventCount }));
}

// Renders the server-specific node silhouette used throughout the timeline.
function NodeShape({ server, size, color }: { server: string; size: number; color: string }) {
  // TODO: in the future it would be best if we can find some kind of
  // function that creates the same shape for a server name every time
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

// Summarizes a phase group into a single status badge for the sector header.
function deriveGroupStatus(group: TimelineGroup, governanceEventItemIDs: ReadonlySet<string>): string {
  const items = group.items;
  const hasGovernanceEvent = items.some((item) => governanceEventItemIDs.has(item.id));
  for (let i = items.length - 1; i >= 0; i--) {
    const item = items[i];
    if (item.kind === "task") {
      const tone = getTimelineItemTone(item);
      if (tone === "failed") return "failed";
      if (tone === "completed") return hasGovernanceEvent ? "saved" : "passed";
      if (tone === "active") return "active";
    }
  }

  const hasFailed = items.some((item) => getTimelineItemTone(item) === "failed");
  if (hasFailed) return "failed";

  return "info";
}

type GovernanceSeverity = "low" | "medium" | "high";

type GroupGovernanceSignal = {
  category: string;
  severity: GovernanceSeverity;
};

type TimelineGovernanceEvent = {
  id: string;
  itemId: string;
  action: string;
  category: string;
  event: string;
  reason: string;
  severity: GovernanceSeverity;
};

function getGroupGovernanceSignals(group: TimelineGroup): GroupGovernanceSignal[] {
  const signals: GroupGovernanceSignal[] = [];
  const seen = new Set<string>();

  for (const item of group.items) {
    for (const annotation of getTimelineItemGovernanceAnnotations(item)) {
      if (annotation.type !== "governance_events") {
        continue;
      }
      const category = annotation.category?.trim();
      const severity = normalizeGovernanceSeverity(annotation.severity);
      if (!category || !severity || !getGovernanceCategoryTone(category)) {
        continue;
      }
      const key = `${category.toLowerCase()}:${severity}`;
      if (seen.has(key)) {
        continue;
      }
      seen.add(key);
      signals.push({ category, severity });
    }
  }

  return signals.sort((left, right) => severityOrder(left.severity) - severityOrder(right.severity));
}

function getTimelineItemGovernanceAnnotations(item: TimelineItem) {
  if (item.kind === "task") {
    return [
      ...getEventAnnotations(item.task),
      ...getEventAnnotations(item.correlatedExchange?.request),
      ...getEventAnnotations(item.correlatedExchange?.response),
    ];
  }

  return [
    ...getEventAnnotations(item.exchange.request),
    ...getEventAnnotations(item.exchange.response),
  ];
}

function normalizeGovernanceSeverity(severity?: string): GovernanceSeverity | undefined {
  const normalized = severity?.trim().toLowerCase();
  if (normalized === "critical" || normalized === "high") {
    return "high";
  }
  if (normalized === "medium" || normalized === "low") {
    return normalized;
  }
  return undefined;
}

function severityOrder(severity: GovernanceSeverity): number {
  switch (severity) {
    case "high":
      return 0;
    case "medium":
      return 1;
    case "low":
      return 2;
  }
}

function getTimelineGovernanceEventsByItemID(events: readonly TimelineGovernanceEvent[]): Map<string, TimelineGovernanceEvent[]> {
  const byItemID = new Map<string, TimelineGovernanceEvent[]>();
  for (const event of events) {
    if (!getGovernanceCategoryTone(event.category)) {
      continue;
    }
    const itemEvents = byItemID.get(event.itemId) ?? [];
    itemEvents.push(event);
    byItemID.set(event.itemId, itemEvents);
  }
  return byItemID;
}

function formatGovernanceCategory(category: string): string {
  const normalized = category.trim();
  if (!normalized) {
    return "Governance";
  }
  return normalized.charAt(0).toUpperCase() + normalized.slice(1).toLowerCase();
}

function formatTimelineGovernanceText(event: TimelineGovernanceEvent): string {
  return `${event.action} ${event.event} - ${event.reason}`;
}

function getGroupStatusLabel(status: string): string {
  if (status === "saved") {
    return "Saved";
  }
  return status;
}

function getGroupStatusColor(status: string): string {
  switch (status) {
    case "passed":
      return "#34d399";
    case "saved":
      return "#a78bfa";
    case "failed":
      return "#f87171";
    case "active":
      return "#60a5fa";
    default:
      return "#f8c171";
  }
}

// Renders the collapsible phase divider between groups of timeline items.
function SciFiSectorDivider({
  group,
  groupIndex,
  onToggle,
  collapsed,
  governanceEventItemIDs,
}: {
  group: TimelineGroup;
  groupIndex: number;
  onToggle: () => void;
  collapsed: boolean;
  governanceEventItemIDs: ReadonlySet<string>;
}) {
  const status = deriveGroupStatus(group, governanceEventItemIDs);
  const statusColor = getGroupStatusColor(status);
  const governanceSignals = getGroupGovernanceSignals(group);
  const dividerColor = "#242b3a";
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
      <div style={{ position: "relative", height: 1, background: `linear-gradient(to right, transparent, ${dividerColor}, ${dividerColor}99, transparent)`, marginBottom: 8 }}>
        <div style={{ position: "absolute", inset: 0, background: `linear-gradient(to right, transparent, ${dividerColor}, transparent)`, opacity: 0.2 }} />
      </div>

      <div style={{ display: "flex", alignItems: "center", gap: 12 }}>
        {/* Corner bracket */}
        <div style={{
          width: 10, height: 10, flexShrink: 0,
          borderTop: `1px solid ${dividerColor}`,
          borderLeft: `1px solid ${dividerColor}`,
          opacity: 0.7,
        }} />

        <span style={{ fontFamily: "'IBM Plex Mono', 'SFMono-Regular', Menlo, Consolas, monospace", fontSize: 12, color: "#5a6472", letterSpacing: "0.08em" }}>
          {String(groupIndex + 1).padStart(2, "0")} {group.label}
        </span>
        <span style={{ fontFamily: "'IBM Plex Mono', 'SFMono-Regular', Menlo, Consolas, monospace", fontSize: 12, color: "#3d4a6a", opacity: 0.7 }}>
          {group.items.length} events
        </span>
        <div style={{ flex: 1, height: 1, background: `linear-gradient(to right, ${dividerColor}, transparent)` }} />
        <span style={{
          fontFamily: "'IBM Plex Mono', 'SFMono-Regular', Menlo, Consolas, monospace", fontSize: 10,
          color: statusColor, letterSpacing: "0.15em", textTransform: "uppercase",
          padding: "2px 8px", border: `1px solid ${statusColor}40`,
          borderRadius: 2, background: `${statusColor}0d`,
        }}>
          <span className="sci-sector-status">
            {collapsed ? "+" : "−"} {getGroupStatusLabel(status)}
            {governanceSignals.length > 0 ? (
              <span className="sci-sector-governance-icons" aria-label={`${governanceSignals.length} governance event category icons`}>
                {governanceSignals.map((signal) => (
                  <span
                    key={`${signal.category}:${signal.severity}`}
                    className={`sci-sector-governance-icon task-run-detail__governance-category--${getGovernanceCategoryTone(signal.category)}`}
                    title={`${signal.category} · ${signal.severity}`}
                  >
                    <GovernanceCategoryIcon category={signal.category} decorative />
                  </span>
                ))}
              </span>
            ) : null}
          </span>
        </span>

        <div style={{
          width: 10, height: 10, flexShrink: 0,
          borderTop: `1px solid ${dividerColor}`,
          borderRight: `1px solid ${dividerColor}`,
          opacity: 0.7,
        }} />
      </div>
    </button>
  );
}

// Renders a single clickable task or exchange row on the timeline.
function SciFiEventNode({
  item,
  onSelect,
  selected,
  governanceEvents,
}: {
  item: TimelineItem;
  onSelect: (id: string) => void;
  selected: boolean;
  governanceEvents: readonly TimelineGovernanceEvent[];
}) {
  const serverName = item.kind === "task" ? "centian" : getExchangeServerName(item.exchange);
  const vc = getItemColorToken(item);
  const anchorEvent = getTimelineAnchorEvent(item);
  const title = getTimelineItemTitle(item);
  const subtitle = getTimelineItemSubtitle(item);
  const tone = getTimelineItemTone(item);
  const alertLabel = getTimelineItemAlertLabel(item);
  const exchangeLatency = item.kind === "exchange" ? getExchangeLatency(item.exchange) : undefined;

  // Mirror the inner node shape so the outer halo feels like one component.
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
        fontFamily: "'IBM Plex Mono', 'SFMono-Regular', Menlo, Consolas, monospace",
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
          flex: "0 0 520px",
          width: 520,
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
              fontFamily: "'IBM Plex Mono', 'SFMono-Regular', Menlo, Consolas, monospace",
              fontSize: 10, color: vc.color, letterSpacing: "0.12em",
              textTransform: "uppercase", opacity: 0.8, flexShrink: 0,
            }}>
              {item.kind === "task" ? "task" : serverName}
            </span>
            {/* Tool name */}
            <span
              data-testid="timeline-event-title"
              style={{
                fontFamily: "'IBM Plex Mono', 'SFMono-Regular', Menlo, Consolas, monospace",
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
              fontFamily: "'IBM Plex Mono', 'SFMono-Regular', Menlo, Consolas, monospace",
              fontSize: 11, color: "#4a6a8e", marginTop: 3,
              overflow: "hidden", textOverflow: "ellipsis", whiteSpace: "nowrap",
              maxWidth: 460,
            }}>
              {subtitle}
            </div>
          ) : null}
        </div>

        {governanceEvents.length > 0 ? (
          <div className="sci-node-governance" aria-label={`${governanceEvents.length} governance event annotations`}>
            {governanceEvents.map((event) => {
              const tone = getGovernanceCategoryTone(event.category);
              return (
                <span
                  key={event.id}
                  className="sci-node-governance-row"
                  aria-label={`Timeline governance: ${formatGovernanceCategory(event.category)}: ${formatTimelineGovernanceText(event)}`}
                >
                  <span
                    className={`sci-node-governance-icon task-run-detail__governance-category--${tone}`}
                    title={`${formatGovernanceCategory(event.category)} · ${event.severity}`}
                  >
                    <GovernanceCategoryIcon category={event.category} decorative />
                  </span>
                  <span className={`sci-node-governance-category task-run-detail__governance-category-label task-run-detail__governance-category--${tone}`}>
                    {formatGovernanceCategory(event.category)}
                  </span>
                  <span className="sci-node-governance-text">{formatTimelineGovernanceText(event)}</span>
                </span>
              );
            })}
          </div>
        ) : null}
      </div>
    </button>
  );
}

// Hosts the sci-fi timeline with legend, grouped rows, and the completion marker.
export function SciFiTimeline({
  groups,
  collapsedGroups,
  onToggleGroup,
  onSelectItem,
  selectedItemId,
  events,
  governanceEventItemIDs,
  governanceEvents,
}: {
  groups: TimelineGroup[];
  collapsedGroups: Record<string, boolean>;
  onToggleGroup: (key: string) => void;
  onSelectItem: (id: string) => void;
  selectedItemId: string;
  events: TaskRunEvent[];
  governanceEventItemIDs: ReadonlySet<string>;
  governanceEvents: readonly TimelineGovernanceEvent[];
}) {
  const serverLegendEntries = getMCPServerLegendEntries(groups, events);
  const governanceEventsByItemID = getTimelineGovernanceEventsByItemID(governanceEvents);
  // Show the end-cap only once the run has emitted a completed task event/status.
  const hasCompleted = events.length > 0 && events.some(
    (e) => e.source === "task" && (e.eventType === "task_completed" || (e.payloadJson as { status?: string } | null)?.status === "completed")
  );

  return (
    <div className="ctv" style={{
      background: "#020210",
      color: "#8ba4c8",
      fontFamily: "'IBM Plex Mono', 'SFMono-Regular', Menlo, Consolas, monospace",
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
        {serverLegendEntries.length > 0 && (
          <div style={{ display: "flex", gap: 24, marginBottom: 16, paddingLeft: 120 }}>
            {serverLegendEntries.map(({ name, eventCount }) => (
              <div key={name} style={{ display: "flex", alignItems: "center", gap: 6, fontSize: 14, color: getServerColorToken(name).color, opacity: 0.7, letterSpacing: "0.1em", fontFamily: "'IBM Plex Mono', 'SFMono-Regular', Menlo, Consolas, monospace" }}>
                <NodeShape server={name} size={10} color={getServerColorToken(name).color} />
                <span>{name}</span>
                <span >({eventCount})</span>
                {/* {style={{ color: "#536785", opacity: 0.85 }}} */}
              </div>
            ))}
          </div>
        )}

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
                governanceEventItemIDs={governanceEventItemIDs}
              />
              {!collapsedGroups[group.key] && group.items.map((item) => (
                <SciFiEventNode
                  key={item.id}
                  item={item}
                  onSelect={onSelectItem}
                  selected={selectedItemId === item.id}
                  governanceEvents={governanceEventsByItemID.get(item.id) ?? []}
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

import { type CSSProperties, useEffect, useMemo, useRef, useState } from "react";
import { Eye, ListChecks, ScrollText, SearchCheck, ShieldCheck, TriangleAlert, type LucideIcon } from "lucide-react";
import { Link, useLocation, useParams } from "react-router-dom";

import { ApiError, fetchTaskRunDetail, fetchTaskRunEvents, normalizeProjectSlug, type ProcessorAnnotation, type TaskRunDetailMetadata, type TaskRunEvent, type TaskRunSnapshot } from "../api/task-runs";
import { ApiAuthCard } from "./api-auth-card";
import { formatTimestamp, formatDuration, formatTemplateLabel, humanizeIdentifier, humanizePhase } from "./format";
import { SciFiTimeline } from "./sci-fi-timeline";
import { type TaskRunUIStatus } from "./task-run-status";

// Tracks the fetch state for the detail page.
type LoadState = "loading" | "ready" | "invalid" | "not-found" | "error" | "unauthorized";

// Represents one rendered phase section in the timeline.
export type TimelineGroup = {
  key: string;
  phasePath: string;
  label: string;
  items: TimelineItem[];
};

// Pairs request and response action events into a single exchange row.
export type TimelineExchange = {
  requestId?: string;
  request?: TaskRunEvent;
  response?: TaskRunEvent;
};

// Normalizes task lifecycle events and MCP exchanges into a single timeline model.
export type TimelineItem =
  | {
      id: string;
      kind: "task";
      task: TaskRunEvent;
      correlatedExchange?: TimelineExchange;
    }
  | {
      id: string;
      kind: "exchange";
      exchange: TimelineExchange;
    };

// Loads a single run, groups its events into timeline sections, and drives the inspector UI.
export function TaskRunDetailPage() {
  const { projectSlug: rawProjectSlug, runID } = useParams();
  const projectSlug = normalizeProjectSlug(rawProjectSlug);
  const location = useLocation();
  const [events, setEvents] = useState<TaskRunEvent[]>([]);
  const [detailMetadata, setDetailMetadata] = useState<TaskRunDetailMetadata | null>(null);
  const [loadState, setLoadState] = useState<LoadState>("loading");
  const [collapsedGroups, setCollapsedGroups] = useState<Record<string, boolean>>({});
  const [selectedItemID, setSelectedItemID] = useState<string>("");
  const [detailsWidth, setDetailsWidth] = useState(() => getDefaultDetailsWidth());
  const [draggingResize, setDraggingResize] = useState(false);
  const [authHeaderName, setAuthHeaderName] = useState<string>();
  const [now, setNow] = useState(() => Date.now());
  const [reloadToken, setReloadToken] = useState(0);
  const previousExpandedWidthRef = useRef(getDefaultDetailsWidth());
  const detailStatus = useMemo(() => deriveTaskRunDetailStatus(events), [events]);
  const benchmarkLink = detailMetadata?.benchmarkLinks?.[0];
  const detailSubtitle = useMemo(
    () => deriveTaskRunSubtitle(events, detailMetadata?.snapshot),
    [detailMetadata?.snapshot, events],
  );

  useEffect(() => {
    if (!runID) {
      setLoadState("invalid");
      setEvents([]);
      setDetailMetadata(null);
      return;
    }

    const controller = new AbortController();
    setLoadState("loading");
    setCollapsedGroups({});
    setSelectedItemID("");

    // Reset view state when the route changes and ignore responses from aborted requests.
    void Promise.all([
      fetchTaskRunEvents(projectSlug, runID, controller.signal),
      fetchTaskRunDetail(projectSlug, runID, controller.signal),
    ])
      .then(([eventResult, detailResult]) => {
        setEvents(eventResult);
        setDetailMetadata(detailResult);
        setLoadState("ready");
      })
      .catch((error: unknown) => {
        if ((error as Error)?.name === "AbortError") {
          return;
        }
        if (error instanceof ApiError) {
          if (error.status === 401) {
            setAuthHeaderName(error.authHeaderName);
            setLoadState("unauthorized");
            return;
          }
          if (error.status === 400) {
            setLoadState("invalid");
            return;
          }
          if (error.status === 404) {
            setLoadState("not-found");
            return;
          }
        }
        setLoadState("error");
      });

    return () => controller.abort();
  }, [projectSlug, reloadToken, runID]);

  useEffect(() => {
    if (!runID || loadState !== "ready" || detailStatus !== "active") {
      return;
    }

    let inFlight = false;
    let controller: AbortController | null = null;

    const poll = () => {
      if (inFlight) {
        return;
      }
      if (document.visibilityState === "hidden") {
        return;
      }

      inFlight = true;
      controller = new AbortController();

      void fetchTaskRunEvents(projectSlug, runID, controller.signal)
        .then((result) => {
          setEvents(result);
        })
        .catch((error: unknown) => {
          if ((error as Error)?.name === "AbortError") {
            return;
          }
          if (error instanceof ApiError && error.status === 401) {
            setAuthHeaderName(error.authHeaderName);
            setLoadState("unauthorized");
          }
        })
        .finally(() => {
          inFlight = false;
          controller = null;
        });
    };

    const timer = window.setInterval(poll, 1000);
    return () => {
      window.clearInterval(timer);
      controller?.abort();
    };
  }, [detailStatus, loadState, projectSlug, runID]);

  const timelineItems = useMemo(() => buildTimelineItems(events), [events]);
  const groupedEvents = useMemo(() => groupEventsByPhase(timelineItems), [timelineItems]);
  const flatTimelineItems = useMemo(
    () => groupedEvents.flatMap((group) => group.items),
    [groupedEvents],
  );
  const governanceEvents = useMemo(
    () => deriveGovernanceEvents(flatTimelineItems),
    [flatTimelineItems],
  );
  const governanceEventItemIDs = useMemo(
    () => new Set(governanceEvents.map((event) => event.itemId)),
    [governanceEvents],
  );
  const selectedItem = flatTimelineItems.find((item) => item.id === selectedItemID);
  const inspectorVisible = selectedItemID !== "" && selectedItem != null;

  const runStats = useMemo(() => {
    const startedAt = events[0]?.createdAtUnixMilli;
    const lastSeenAt = events.length > 0 ? events[events.length - 1].createdAtUnixMilli : undefined;

    let processEventCount = 0;
    let mcpEventCount = 0;
    // Aggregate lightweight summary stats directly from the flattened render model.
    for (const item of flatTimelineItems) {
      if (item.kind === "task") {
        processEventCount++;
        continue;
      }

      if (isProcessExchange(item.exchange)) {
        processEventCount++;
      } else {
        mcpEventCount++;
      }
    }

    return {
      startedAt,
      durationEndedAt: detailStatus === "active" ? undefined : lastSeenAt,
      lastSeenAt,
      processEventCount,
      mcpEventCount,
    };
  }, [detailStatus, events, flatTimelineItems]);
  const templateLabel = useMemo(() => getTaskTemplateLabel(detailMetadata), [detailMetadata]);

  useEffect(() => {
    if (detailStatus !== "active") {
      return;
    }

    const timer = window.setInterval(() => {
      setNow(Date.now());
    }, 1000);

    return () => window.clearInterval(timer);
  }, [detailStatus]);

  useEffect(() => {
    // Preserve existing collapse choices while seeding new groups as expanded.
    setCollapsedGroups((current) => {
      const next = { ...current };
      for (const group of groupedEvents) {
        if (!(group.key in next)) {
          next[group.key] = false;
        }
      }
      return next;
    });
  }, [groupedEvents]);

  useEffect(() => {
    if (!draggingResize || !inspectorVisible) {
      return;
    }

    // Resize is driven from the viewport edge so the panel width feels anchored to the right side.
    function handleMouseMove(event: MouseEvent) {
      const nextWidth = clampDetailsWidth(window.innerWidth - event.clientX - 24);
      previousExpandedWidthRef.current = nextWidth;
      setDetailsWidth(nextWidth);
    }

    function handleMouseUp() {
      setDraggingResize(false);
    }

    window.addEventListener("mousemove", handleMouseMove);
    window.addEventListener("mouseup", handleMouseUp);

    return () => {
      window.removeEventListener("mousemove", handleMouseMove);
      window.removeEventListener("mouseup", handleMouseUp);
    };
  }, [draggingResize, inspectorVisible]);

  if (loadState === "loading") {
    return (
      <div className="state-card state-card--detail" data-testid="task-run-detail-loading">
        <p className="state-card__eyebrow">Syncing</p>
        <h2>Loading task timeline…</h2>
        <p>Pulling task and action events for this run.</p>
      </div>
    );
  }

  if (loadState === "invalid") {
    return (
      <DetailStateCard
        eyebrow="Signal Fault"
        title="Invalid task run id"
        body="The requested run id does not match the expected Centian format."
      />
    );
  }

  if (loadState === "not-found") {
    return (
      <DetailStateCard
        eyebrow="Dead Channel"
        title="Task run not found"
        body="No persisted event timeline exists for this task run."
      />
    );
  }

  if (loadState === "error") {
    return (
      <DetailStateCard
        eyebrow="Link Loss"
        title="Task timeline unavailable"
        body="The event stream could not be loaded right now."
      />
    );
  }

  if (loadState === "unauthorized") {
    return (
      <ApiAuthCard
        eyebrow="Access Required"
        title="Task timeline is protected"
        body="Enter a Centian API key to load this persisted run timeline."
        authHeaderName={authHeaderName}
        onSaved={() => setReloadToken((value) => value + 1)}
        showBackLink
      />
    );
  }

  return (
    <div className="task-run-detail">
      <header className="task-run-detail__header">
        <div className="task-run-detail__nav-actions">
          <Link className="back-link task-run-detail__back-link" aria-label="Back to task runs" to={`/${projectSlug}/tasks${location.search || ""}`}>
            <span aria-hidden="true">←</span>
            <span>All Agent Tasks</span>
          </Link>
          {benchmarkLink ? (
            <Link
              className="back-link task-run-detail__benchmark-link"
              to={`/benchmarks/${benchmarkLink.suiteId}/runs/${benchmarkLink.benchmarkRunId}`}
            >
              See Benchmark
            </Link>
          ) : null}
        </div>
        <div className="task-run-detail__title-group">
          <h1 className="task-run-detail__title">Agent Task Details</h1>
        </div>
      </header>

      {detailSubtitle ? <TaskDescriptionRow text={detailSubtitle.text} /> : null}
      <RunMetadataBar stats={runStats} status={detailStatus} now={now} templateLabel={templateLabel} />
      <GovernanceEventsPanel events={governanceEvents} />

      <div className="task-run-detail__workspace">
        <SciFiTimeline
          groups={groupedEvents}
          collapsedGroups={collapsedGroups}
          onToggleGroup={(key) =>
            setCollapsedGroups((current) => ({
              ...current,
              [key]: !current[key],
            }))
          }
          onSelectItem={setSelectedItemID}
          selectedItemId={selectedItemID}
          events={events}
          governanceEventItemIDs={governanceEventItemIDs}
          governanceEvents={governanceEvents}
        />

        {inspectorVisible ? (
          <aside
            className="task-detail-panel"
            style={
              {
                "--task-detail-panel-width": `${detailsWidth}px`,
              } as CSSProperties
            }
          >
            <button
              type="button"
              className="task-detail-panel__resize-handle"
              aria-label="Resize detail panel"
              onMouseDown={(event) => {
                event.preventDefault();
                setDraggingResize(true);
              }}
            />
            <div className="task-detail-panel__surface">
              <div className="task-detail-panel__header">
                <button
                  type="button"
                  className="task-detail-panel__collapse"
                  aria-label="Hide detail panel"
                  onClick={() => {
                    previousExpandedWidthRef.current = detailsWidth;
                    setSelectedItemID("");
                  }}
                >
                  ×
                </button>

                <div className="task-detail-panel__header-copy">
                  <p className="state-card__eyebrow">Inspector</p>
                  <h3>{selectedItem ? getTimelineItemTitle(selectedItem) : "No event selected"}</h3>
                </div>
              </div>

              <div className="task-detail-panel__body">
                {selectedItem ? <DetailInspector item={selectedItem} /> : null}
              </div>
            </div>
          </aside>
        ) : null}
      </div>
    </div>
  );
}

// Shows the selected timeline item along with its payloads and derived metadata.
function DetailInspector({ item }: { item: TimelineItem }) {
  const tone = getTimelineItemTone(item);
  const statusLabel = getTimelineItemStatusLabel(item);
  const statusTone = getInspectorStatusTone(statusLabel, tone);
  const anchorEvent = getTimelineAnchorEvent(item);
  const subtitle = getTimelineItemSubtitle(item);
  const metaLabel = item.kind === "task" ? "task" : getExchangeServerLabel(item.exchange);
  const exchangeLatency = item.kind === "exchange" ? getExchangeLatency(item.exchange) : undefined;

  return (
    <div className="task-detail-panel__content">
      <div className="task-detail-panel__summary">
        <div>
          <p className="task-detail-panel__kicker">{metaLabel}</p>
          <h4 className="task-detail-panel__title">{getTimelineItemTitle(item)}</h4>
          {subtitle ? <p className="task-detail-panel__subtitle">{subtitle}</p> : null}
        </div>
        <div className="task-detail-panel__badges">
          {exchangeLatency != null ? (
            <span className="timeline-event__metric">{formatLatency(exchangeLatency)}</span>
          ) : null}
          <span className={`timeline-event__status timeline-event__status--${statusTone}`}>{statusLabel}</span>
        </div>
      </div>

      <div className="task-detail-panel__meta">
        <span>{formatTimestamp(anchorEvent.createdAtUnixMilli)}</span>
        <span>{item.kind === "task" ? "task lifecycle" : "mcp exchange"}</span>
      </div>

      {item.kind === "task" ? (
        <>
          <PayloadSection label="Task Event" event={item.task} statusLabel={statusLabel} />
          {item.correlatedExchange ? (
            <ExchangeDetails exchange={item.correlatedExchange} prefixLabel="Centian" />
          ) : null}
        </>
      ) : (
        <ExchangeDetails exchange={item.exchange} />
      )}
    </div>
  );
}

function getInspectorStatusTone(
  statusLabel: string,
  fallbackTone: "neutral" | "active" | "completed" | "failed",
): "neutral" | "active" | "completed" | "failed" {
  switch (statusLabel) {
    case "success":
      return "completed";
    case "error":
      return "failed";
    case "timed_out":
      return "failed";
    default:
      return fallbackTone;
  }
}

// Renders a labeled JSON payload block for either a task event or an exchange message.
function PayloadSection({
  label,
  event,
  statusLabel,
}: {
  label: string;
  event: TaskRunEvent;
  statusLabel?: string;
}) {
  return (
    <section className="timeline-event__payload-section">
      <div className="timeline-event__details-meta">
        <span>{label}</span>
        <span>{formatTimestamp(event.createdAtUnixMilli)}</span>
        {statusLabel ? <span>{statusLabel}</span> : null}
      </div>
      <p className="timeline-event__payload-label">{label}</p>
      <pre className="timeline-event__payload">{formatPayload(event.payloadJson)}</pre>
    </section>
  );
}

// Displays the request/response halves of an MCP exchange using the shared payload renderer.
function ExchangeDetails({
  exchange,
  prefixLabel,
}: {
  exchange: TimelineExchange;
  prefixLabel?: string;
}) {
  const requestStatus = exchange.response == null ? "pending" : "sent";
  const responseStatus = getExchangeStatusLabel(exchange);

  return (
    <div className="task-detail-panel__exchange">
      {exchange.request ? (
        <PayloadSection
          label={`${prefixLabel ? `${prefixLabel} ` : ""}Request`}
          event={exchange.request}
          statusLabel={requestStatus}
        />
      ) : null}
      {exchange.response ? (
        <PayloadSection
          label={`${prefixLabel ? `${prefixLabel} ` : ""}Response`}
          event={exchange.response}
          statusLabel={responseStatus}
        />
      ) : null}
    </div>
  );
}

// Keeps the inspector width within a usable range on small and large screens.
function clampDetailsWidth(value: number): number {
  return Math.min(1080, Math.max(320, Math.round(value)));
}

// Chooses the initial inspector width from the viewport when running in the browser.
function getDefaultDetailsWidth(): number {
  if (typeof window === "undefined") {
    return 640;
  }

  return clampDetailsWidth(window.innerWidth * 0.4);
}

// Reusable empty/error state card for detail page fetch failures.
function DetailStateCard({
  eyebrow,
  title,
  body,
}: {
  eyebrow: string;
  title: string;
  body: string;
}) {
  return (
    <div className="state-card state-card--detail">
      <p className="state-card__eyebrow">{eyebrow}</p>
      <h2>{title}</h2>
      <p>{body}</p>
      <Link className="back-link" to="/tasks">
        Back to task runs
      </Link>
    </div>
  );
}

// Summary numbers shown above the timeline.
type RunStats = {
  startedAt: number | undefined;
  durationEndedAt: number | undefined;
  lastSeenAt: number | undefined;
  processEventCount: number;
  mcpEventCount: number;
};

type TaskRunDetailSubtitle = {
  text: string;
};

type GovernanceEventDescription = {
  id: string;
  itemId: string;
  action: string;
  category: string;
  event: string;
  reason: string;
  severity: GovernanceSeverity;
};

type GovernanceSeverity = "low" | "medium" | "high";

const STALE_TASK_RUN_TIMEOUT_MS = 15 * 60 * 1000;

function TaskDescriptionRow({ text }: { text: string }) {
  return (
    <div className="task-run-detail__task-row" aria-label="Task detail summary">
      <span className="task-run-detail__task-row-label">Task</span>
      <p>{text}</p>
    </div>
  );
}

// Displays the headline metrics for the selected run.
function RunMetadataBar({
  stats,
  status,
  now,
  templateLabel,
}: {
  stats: RunStats;
  status: TaskRunUIStatus;
  now: number;
  templateLabel?: string;
}) {
  // Shared inline styles keep the compact metadata row visually consistent.
  const valueStyle: CSSProperties = {
    fontSize: 13,
    color: "#c8daf0",
    fontFamily: "'JetBrains Mono', 'Fira Code', monospace",
  };
  const durationValueStyle: CSSProperties = { ...valueStyle, color: undefined };
  const durationMetric = getDurationMetric(stats, status, now);

  return (
    <div
      style={{
        display: "flex",
        flexWrap: "wrap",
        gap: 24,
        padding: "8px 20px",
        background: "rgba(10, 14, 30, 0.38)",
        borderBottom: "1px solid rgba(100,140,200,0.1)",
        fontFamily: "system-ui, -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif",
        alignItems: "flex-start",
      }}
    >
      {templateLabel ? (
        <div className="task-run-detail__metric task-run-detail__template-metric" aria-label={`Template: ${templateLabel}`}>
          <MetricLabel label="Template" />
          <span className="task-run-detail__template-value" style={valueStyle} title={templateLabel}>
            {templateLabel}
          </span>
        </div>
      ) : null}

      {stats.startedAt != null && (
        <div className="task-run-detail__metric">
          <MetricLabel label="Started" />
          <span style={valueStyle}>{formatTimestamp(stats.startedAt)}</span>
        </div>
      )}

      {stats.startedAt != null && (
        <div className="task-run-detail__metric">
          <MetricLabel label="Duration" />
          <span
            className={`task-run-detail__duration-value task-run-detail__duration-value--${durationMetric.tone}`}
            style={durationValueStyle}
          >
            {durationMetric.label}
          </span>
        </div>
      )}

      <SplitMetric
        label="Agent Actions"
        labelDetail="(Process/MCP)"
        processCount={stats.processEventCount}
        mcpCount={stats.mcpEventCount}
        valueStyle={valueStyle}
      />
    </div>
  );
}

function GovernanceEventsPanel({ events }: { events: GovernanceEventDescription[] }) {
  const [expanded, setExpanded] = useState(true);
  const eventCount = events.length;
  const panelID = "task-run-governance-events";
  const titleSignals = getGovernanceTitleSignals(events);

  return (
    <section className="task-run-detail__governance" aria-label={`Governance Events: ${eventCount}`}>
      <button
        type="button"
        className="task-run-detail__governance-toggle"
        aria-expanded={expanded}
        aria-controls={panelID}
        onClick={() => setExpanded((current) => !current)}
      >
        <span className="task-run-detail__governance-title">
          Governance Events: <span className="task-run-detail__governance-title-count">{eventCount}</span>
          {titleSignals.length > 0 ? (
            <span className="task-run-detail__governance-title-signals" aria-hidden="true">
              <span className="task-run-detail__governance-title-paren">(</span>
              {titleSignals.map((signal) => (
                <span
                  key={signal.category}
                  className={`task-run-detail__governance-title-signal task-run-detail__governance-category--${getGovernanceCategoryTone(signal.category)}`}
                  title={`${signal.category} · ${signal.severity}`}
                >
                  <GovernanceCategoryIcon category={signal.category} decorative />
                </span>
              ))}
              <span className="task-run-detail__governance-title-paren">)</span>
            </span>
          ) : null}
        </span>
        <span className="task-run-detail__governance-action">
          {expanded ? "Hide" : "Show"}
        </span>
      </button>

      {expanded ? (
        <div id={panelID} className="task-run-detail__governance-body">
          {events.length > 0 ? (
            <ul className="task-run-detail__governance-list">
              {events.map((event) => (
                <li
                  key={event.id}
                  className={`task-run-detail__governance-item task-run-detail__governance-item--${event.severity}`}
                  aria-label={`${formatGovernanceCategory(event.category)}: ${event.action} ${event.event} - ${event.reason}`}
                >
                  <GovernanceCategoryIcon category={event.category} />
                  <span className={`task-run-detail__governance-category-label task-run-detail__governance-category--${getGovernanceCategoryTone(event.category)}`}>
                    {formatGovernanceCategory(event.category)}
                  </span>
                  <span className="task-run-detail__governance-separator">:</span>
                  <span className="task-run-detail__governance-event-action">
                    {event.action}
                  </span>
                  <code>{event.event}</code>
                  <span className="task-run-detail__governance-separator">-</span>
                  <span>{event.reason}</span>
                </li>
              ))}
            </ul>
          ) : (
            <p className="task-run-detail__governance-empty">No governance events recorded.</p>
          )}
        </div>
      ) : null}
    </section>
  );
}

type GovernanceTitleSignal = {
  category: string;
  severity: GovernanceSeverity;
};

function getGovernanceTitleSignals(events: GovernanceEventDescription[]): GovernanceTitleSignal[] {
  const byCategory = new Map<string, GovernanceTitleSignal>();
  for (const event of events) {
    const category = event.category.trim();
    if (!category || !getGovernanceCategoryIcon(category)) {
      continue;
    }
    const key = category.toLowerCase();
    const current = byCategory.get(key);
    if (!current || governanceSeverityRank(event.severity) < governanceSeverityRank(current.severity)) {
      byCategory.set(key, { category, severity: event.severity });
    }
  }
  return [...byCategory.values()].sort((left, right) => governanceSeverityRank(left.severity) - governanceSeverityRank(right.severity));
}

function governanceSeverityRank(severity: GovernanceSeverity): number {
  switch (severity) {
    case "high":
      return 0;
    case "medium":
      return 1;
    case "low":
      return 2;
  }
}

export function GovernanceCategoryIcon({ category, decorative = false }: { category: string; decorative?: boolean }) {
  const Icon = getGovernanceCategoryIcon(category);
  const tone = getGovernanceCategoryTone(category);
  if (!Icon) {
    return null;
  }
  return (
    <span
      className={`task-run-detail__governance-category-icon task-run-detail__governance-category--${tone}`}
      title={decorative ? undefined : `Category: ${category}`}
    >
      <Icon aria-hidden="true" />
    </span>
  );
}

function formatGovernanceCategory(category: string): string {
  const normalized = category.trim();
  if (!normalized) {
    return "Governance";
  }
  return normalized.charAt(0).toUpperCase() + normalized.slice(1).toLowerCase();
}

function SplitMetric({
  label,
  labelDetail,
  processCount,
  mcpCount,
  valueStyle,
}: {
  label: string;
  labelDetail: string;
  processCount: number;
  mcpCount: number;
  valueStyle: CSSProperties;
}) {
  const accessibleLabel = `${label} ${labelDetail}`;
  return (
    <div className="task-run-detail__metric" aria-label={`${accessibleLabel}: Process ${processCount}, MCP ${mcpCount}`}>
      <MetricLabel label={label} labelDetail={labelDetail} />
      <span className="task-run-detail__split-value" style={{ ...valueStyle, color: undefined }}>
        <span className="task-run-detail__split-process">{processCount}</span>
        <span className="task-run-detail__split-separator">/</span>
        <span className="task-run-detail__split-mcp">{mcpCount}</span>
      </span>
    </div>
  );
}

function MetricLabel({ label, labelDetail }: { label: string; labelDetail?: string }) {
  return (
    <span className="task-run-detail__metric-label" aria-hidden="true">
      <span>{label}</span>
      {labelDetail ? <span className="task-run-detail__metric-label-detail">{labelDetail}</span> : null}
    </span>
  );
}

function getDurationMetric(
  stats: RunStats,
  status: TaskRunUIStatus,
  now: number,
): { label: string; tone: "active" | "success" | "failed" } {
  if (status === "timed_out" || isStaleActiveRun(stats, status, now)) {
    return { label: "timeout", tone: "failed" };
  }

  const label =
    stats.startedAt != null ? formatDuration(stats.startedAt, stats.durationEndedAt, now) : "";

  if (status === "success") {
    return { label, tone: "success" };
  }
  if (status === "failed") {
    return { label, tone: "failed" };
  }
  return { label, tone: "active" };
}

function isStaleActiveRun(stats: RunStats, status: TaskRunUIStatus, now: number): boolean {
  return status === "active" && stats.lastSeenAt != null && now - stats.lastSeenAt >= STALE_TASK_RUN_TIMEOUT_MS;
}

function getTaskTemplateLabel(detailMetadata: TaskRunDetailMetadata | null): string | undefined {
  const snapshot = detailMetadata?.snapshot;
  if (snapshot) {
    return formatTemplateLabel(
      snapshot.templateId,
      snapshot.templateName || snapshot.selectedTemplate?.task?.name,
    );
  }

  const summary = detailMetadata?.summary;
  if (summary) {
    return formatTemplateLabel(summary.templateId, summary.templateName);
  }

  return undefined;
}

function deriveTaskRunSubtitle(events: TaskRunEvent[], snapshot?: TaskRunSnapshot): TaskRunDetailSubtitle | undefined {
  const taskDescription =
    findLatestTaskPayloadString(events, "task_registered", [
      ["taskDescription"],
      ["task_description"],
      ["prompt"],
    ]) ?? snapshot?.taskDescription?.trim();
  return taskDescription ? { text: taskDescription } : undefined;
}

function findLatestTaskPayloadString(events: TaskRunEvent[], eventType: string, paths: string[][]): string | undefined {
  for (let index = events.length - 1; index >= 0; index -= 1) {
    const event = events[index];
    if (event.source !== "task" || event.eventType !== eventType) {
      continue;
    }
    const value = readPayloadString(event.payloadJson, paths);
    if (value) {
      return value;
    }
  }
  return undefined;
}

function readPayloadString(payload: unknown, paths: string[][]): string | undefined {
  for (const path of paths) {
    const value = readPayloadPath(payload, path);
    if (typeof value === "string" && value.trim() !== "") {
      return value.trim();
    }
  }
  return undefined;
}

function readPayloadPath(payload: unknown, path: string[]): unknown {
  let current = payload;
  for (const segment of path) {
    if (current == null || typeof current !== "object" || Array.isArray(current)) {
      return undefined;
    }
    current = (current as Record<string, unknown>)[segment];
  }
  return current;
}

function deriveGovernanceEvents(items: TimelineItem[]): GovernanceEventDescription[] {
  const descriptions: GovernanceEventDescription[] = [];

  const addDescription = (
    id: string,
    itemId: string,
    action: string,
    category: string,
    event: string,
    reason: string,
    severity: GovernanceSeverity,
  ) => {
    descriptions.push({ id, itemId, action, category, event, reason, severity });
  };

  for (const item of items) {
    if (item.kind === "task") {
      addAnnotationGovernanceDescriptions(item.id, getProcessActionLabel(item.task), [item.task], addDescription);
      if (item.correlatedExchange) {
        addAnnotationGovernanceDescriptions(
          item.id,
          getGovernanceToolLabel(item.correlatedExchange),
          [item.correlatedExchange.request, item.correlatedExchange.response],
          addDescription,
        );
      }
      continue;
    }

    addAnnotationGovernanceDescriptions(
      item.id,
      getGovernanceToolLabel(item.exchange),
      [item.exchange.request, item.exchange.response],
      addDescription,
    );
  }

  const actionOrder: Record<string, number> = {
    Redacted: 0,
    Removed: 0,
    Modified: 0,
    Escalated: 0,
    Blocked: 1,
    Stopped: 2,
  };
  return descriptions.sort((left, right) => (actionOrder[left.action] ?? 3) - (actionOrder[right.action] ?? 3));
}

function addAnnotationGovernanceDescriptions(
  itemId: string,
  eventLabel: string,
  events: Array<TaskRunEvent | undefined>,
  addDescription: (
    id: string,
    itemId: string,
    action: string,
    category: string,
    event: string,
    reason: string,
    severity: GovernanceSeverity,
  ) => void,
) {
  let index = 0;
  for (const event of events) {
    for (const annotation of getEventAnnotations(event)) {
      if (annotation.type !== "governance_events") {
        continue;
      }
      const action = getProcessorGovernanceAction(annotation.action);
      const category = annotation.category?.trim();
      const severity = normalizeGovernanceSeverity(annotation.severity);
      if (!action || !category || !severity) {
        continue;
      }
      addDescription(
        `${itemId}:${event?.id ?? "event"}:${index}`,
        itemId,
        action,
        category,
        eventLabel,
        getProcessorGovernanceReason(annotation),
        severity,
      );
      index += 1;
    }
  }
}

export function getEventAnnotations(event: TaskRunEvent | undefined): ProcessorAnnotation[] {
  if (!event) {
    return [];
  }

  const payloadAnnotations = readPayloadAnnotations(event.payloadJson);
  if (!event.annotations || event.annotations.length === 0) {
    return payloadAnnotations;
  }
  return [...event.annotations, ...payloadAnnotations];
}

function readPayloadAnnotations(payload: unknown): ProcessorAnnotation[] {
  const annotations = readPayloadPath(payload, ["annotations"]);
  if (!Array.isArray(annotations)) {
    return [];
  }
  return annotations.filter((annotation): annotation is ProcessorAnnotation => (
    annotation != null &&
    typeof annotation === "object" &&
    !Array.isArray(annotation)
  ));
}

function getProcessorGovernanceAction(action?: string): string | undefined {
  const normalized = action?.trim();
  if (!normalized) {
    return undefined;
  }
  return normalized.charAt(0).toUpperCase() + normalized.slice(1).toLowerCase();
}

function getProcessorGovernanceReason(annotation: NonNullable<TaskRunEvent["annotations"]>[number]): string {
  const message = annotation.message?.trim();
  if (message) {
    return message;
  }

  switch (annotation.action?.trim().toLowerCase()) {
    case "blocked":
      return "processor blocked content";
    case "stopped":
      return "process action stopped";
    case "redacted":
      return "processor redacted content";
    case "removed":
      return "processor removed content";
    case "escalated":
      return "processor escalated event";
    default:
      return "processor modified content";
  }
}

function getProcessActionLabel(event: TaskRunEvent): string {
  switch (event.eventType) {
    case "task_registered":
      return "Register Task";
    case "onboarding_completed":
      return "Complete Onboarding";
    case "planning_completed":
      return "Complete Planning";
    case "step_started":
    case "step_completed":
      return getTaskStepDisplayName(event).replace(/^Execution\s*\/\s*/i, "") || "Execution Step";
    case "task_completed":
      return "Complete Task";
    case "task_failed":
      return "Fail Task";
    case "task_timed_out":
      return "Timeout Task";
    case "restarted":
      return "Restart Task";
    case "resumed":
      return "Resume Task";
    default:
      return humanizeIdentifier(event.eventType ?? "Process Action");
  }
}

function getProcessActionLabelForToolName(toolName?: string): string {
  switch (toolName) {
    case "centian.task_register":
      return "Register Task";
    case "centian.task_complete_onboarding":
      return "Complete Onboarding";
    case "centian.task_complete_planning":
      return "Complete Planning";
    case "centian.task_start_step":
      return "Start Execution";
    case "centian.task_complete_step":
      return "Complete Execution";
    case "centian.task_resume":
      return "Resume Task";
    case "centian.task_restart":
      return "Restart Task";
    case "centian.task_fail":
      return "Fail Task";
    default:
      return humanizeIdentifier(toolName ?? "Process Action");
  }
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

export function getGovernanceCategoryTone(category: string): string | undefined {
  switch (category.trim().toLowerCase()) {
    case "security":
    case "policy":
    case "quality":
    case "observability":
    case "compliance":
    case "risk":
      return category.trim().toLowerCase();
    default:
      return undefined;
  }
}

function getGovernanceCategoryIcon(category: string): LucideIcon | undefined {
  switch (category.trim().toLowerCase()) {
    case "security":
      return ShieldCheck;
    case "policy":
      return ScrollText;
    case "observability":
      return Eye;
    case "compliance":
      return ListChecks;
    case "risk":
      return TriangleAlert;
    case "quality":
      return SearchCheck;
    default:
      return undefined;
  }
}

function getGovernanceToolLabel(exchange: TimelineExchange): string {
  const event = exchange.request ?? exchange.response;
  const originalToolName = event?.originalToolName?.trim();
  const parsedOriginalToolName = originalToolName ? parseNamespacedToolName(originalToolName) : undefined;
  if (parsedOriginalToolName) {
    return `${parsedOriginalToolName.server} - ${parsedOriginalToolName.tool}`;
  }

  const toolName = event?.toolName?.trim();
  if (toolName?.startsWith("centian.task_")) {
    return getProcessActionLabelForToolName(toolName);
  }

  const serverName = getExchangeServerName(exchange);
  if (toolName) {
    return `${serverName} - ${toolName}`;
  }
  return getExchangeTitle(exchange);
}

function parseNamespacedToolName(value: string): { server: string; tool: string } | undefined {
  const match = value.match(/^([A-Za-z0-9-]+)_{2,3}(.+)$/);
  if (!match) {
    return undefined;
  }
  return { server: match[1], tool: match[2] };
}

function isProcessExchange(exchange: TimelineExchange): boolean {
  return getExchangeServerName(exchange) === "centian";
}

// Converts the raw event stream into timeline rows, merging request/response pairs where possible.
function buildTimelineItems(events: TaskRunEvent[]): TimelineItem[] {
  const rawItems: TimelineItem[] = [];
  const pendingExchanges = new Map<string, TimelineExchange>();

  for (const event of events) {
    if (event.source === "task") {
      rawItems.push({
        id: event.id,
        kind: "task",
        task: event,
      });
      continue;
    }

    const exchange = createSingletonExchange(event);
    if (!hasRenderableExchangeEvent(exchange)) {
      continue;
    }
    const requestId = getExchangeRequestID(event);
    if (!requestId) {
      rawItems.push({
        id: event.id,
        kind: "exchange",
        exchange,
      });
      continue;
    }

    if (isRequestAction(event)) {
      rawItems.push({
        id: event.id,
        kind: "exchange",
        exchange,
      });
      pendingExchanges.set(requestId, exchange);
      continue;
    }

    if (isResponseAction(event)) {
      const pending = pendingExchanges.get(requestId);
      if (pending && pending.response == null) {
        pending.response = event;
        continue;
      }
    }

    rawItems.push({
      id: event.id,
      kind: "exchange",
      exchange,
    });
  }

  const centianExchangesByRequestID = new Map<string, TimelineExchange>();
  const centianFallbackExchangesByTaskKey = new Map<string, TimelineExchange[]>();
  for (const item of rawItems) {
    if (
      item.kind === "exchange" &&
      isCollapsibleCentianExchange(item.exchange)
    ) {
      if (item.exchange.requestId) {
        centianExchangesByRequestID.set(item.exchange.requestId, item.exchange);
      }
      const fallbackKey = getCentianTaskFallbackKey(item.exchange);
      if (fallbackKey) {
        const existing = centianFallbackExchangesByTaskKey.get(fallbackKey) ?? [];
        existing.push(item.exchange);
        centianFallbackExchangesByTaskKey.set(fallbackKey, existing);
      }
    }
  }

  const hiddenExchangeRequestIDs = new Set<string>();
  const hiddenExchangeEventIDs = new Set<string>();
  const correlatedExchangeByTaskID = new Map<string, TimelineExchange>();
  for (const item of rawItems) {
    if (item.kind !== "task") {
      continue;
    }
    const correlatedExchange =
      item.task.relatedActionRequestId != null
        ? centianExchangesByRequestID.get(item.task.relatedActionRequestId)
        : getFallbackCentianTaskExchange(item.task, centianFallbackExchangesByTaskKey);
    if (!correlatedExchange) {
      continue;
    }
    correlatedExchangeByTaskID.set(item.task.id, correlatedExchange);
    if (correlatedExchange.requestId) {
      hiddenExchangeRequestIDs.add(correlatedExchange.requestId);
    }
    if (correlatedExchange.request?.id) {
      hiddenExchangeEventIDs.add(correlatedExchange.request.id);
    }
    if (correlatedExchange.response?.id) {
      hiddenExchangeEventIDs.add(correlatedExchange.response.id);
    }
  }

  const items: TimelineItem[] = [];
  for (const item of rawItems) {
    if (item.kind === "task") {
      // Fold matching Centian exchanges into task rows so the timeline stays compact.
      const correlatedExchange = correlatedExchangeByTaskID.get(item.task.id);
      items.push(
        correlatedExchange
          ? {
              ...item,
              correlatedExchange,
            }
          : item,
      );
      continue;
    }

    if (hiddenExchangeEventIDs.has(item.id)) {
      continue;
    }
    if (
      item.exchange.requestId &&
      hiddenExchangeRequestIDs.has(item.exchange.requestId) &&
      isCollapsibleCentianExchange(item.exchange)
    ) {
      continue;
    }

    items.push(item);
  }

  return items;
}

function getFallbackCentianTaskExchange(
  task: TaskRunEvent,
  fallbackMap: Map<string, TimelineExchange[]>,
): TimelineExchange | undefined {
  const fallbackKey = getCentianTaskFallbackKeyForTask(task);
  if (!fallbackKey) {
    return undefined;
  }
  const candidates = fallbackMap.get(fallbackKey);
  return candidates?.length === 1 ? candidates[0] : undefined;
}

function getCentianTaskFallbackKeyForTask(task: TaskRunEvent): string | undefined {
  if (task.source !== "task" || !task.eventType) {
    return undefined;
  }
  return `${task.createdAtUnixMilli}:${task.eventType}`;
}

function getCentianTaskFallbackKey(exchange: TimelineExchange): string | undefined {
  const taskEventType = getTaskEventTypeForCentianTool(exchange.request?.toolName ?? exchange.response?.toolName);
  const anchorTimestamp = exchange.request?.createdAtUnixMilli ?? exchange.response?.createdAtUnixMilli;
  if (!taskEventType || anchorTimestamp == null) {
    return undefined;
  }
  return `${anchorTimestamp}:${taskEventType}`;
}

function getTaskEventTypeForCentianTool(toolName?: string): string | undefined {
  switch (toolName) {
    case "centian.task_register":
      return "task_registered";
    case "centian.task_complete_onboarding":
      return "onboarding_completed";
    case "centian.task_complete_planning":
      return "planning_completed";
    case "centian.task_start_step":
      return "step_started";
    case "centian.task_complete_step":
      return "step_completed";
    case "centian.task_resume":
      return "resumed";
    case "centian.task_restart":
      return "restarted";
    case "centian.task_fail":
      return "task_failed";
    default:
      return undefined;
  }
}

// Wraps a single action event so it can later be paired with its counterpart.
function createSingletonExchange(event: TaskRunEvent): TimelineExchange {
  const requestId = getExchangeRequestID(event);
  if (isRequestAction(event)) {
    return {
      requestId,
      request: event,
    };
  }

  if (isResponseAction(event)) {
    return {
      requestId,
      response: event,
    };
  }

  return { requestId };
}

function hasRenderableExchangeEvent(
  exchange: TimelineExchange,
): exchange is TimelineExchange &
  ({ request: TaskRunEvent; response?: TaskRunEvent } | { request?: TaskRunEvent; response: TaskRunEvent }) {
  return exchange.request != null || exchange.response != null;
}

// Detects request-direction action events across the variants emitted by the backend.
function isRequestAction(event: TaskRunEvent): boolean {
  if (event.source !== "action") {
    return false;
  }

  return (
    event.messageType === "request" ||
    event.direction === "request" ||
    event.direction === "[CLIENT -> SERVER]"
  );
}

// Detects response-direction action events across legacy and current direction labels.
function isResponseAction(event: TaskRunEvent): boolean {
  if (event.source !== "action") {
    return false;
  }

  return (
    event.messageType === "response" ||
    event.direction === "response" ||
    event.direction === "[SERVER -> CLIENT]" ||
    event.direction === "[CENTIAN -> CLIENT]"
  );
}

// Resolves the request id from either the normalized field or the raw payload body.
function getExchangeRequestID(event: TaskRunEvent): string | undefined {
  if (event.requestId && event.requestId.trim() !== "") {
    return event.requestId;
  }

  const payload = readPayloadObject(event.payloadJson);
  const payloadRequestID = payload?.request_id;
  return typeof payloadRequestID === "string" && payloadRequestID.trim() !== ""
    ? payloadRequestID
    : undefined;
}

// Buckets timeline items into contiguous phase groups for the sectored timeline layout.
function groupEventsByPhase(items: TimelineItem[]): TimelineGroup[] {
  const groups: TimelineGroup[] = [];
  let lastKnownPhase = "";

  for (const item of items) {
    const event = getTimelineAnchorEvent(item);
    const effectivePhase = getGroupingPhase(event, lastKnownPhase);
    lastKnownPhase = effectivePhase;

    const existingGroup = groups.length > 0 ? groups[groups.length - 1] : undefined;
    if (existingGroup == null || existingGroup.phasePath !== effectivePhase) {
      groups.push({
        key: `${effectivePhase}:${item.id}`,
        phasePath: effectivePhase,
        label: humanizePhase(effectivePhase),
        items: [item],
      });
      continue;
    }

    existingGroup.items.push(item);
  }

  return groups;
}

// Picks the event that should drive labels and timestamps for a mixed timeline item.
export function getTimelineAnchorEvent(item: TimelineItem): TaskRunEvent {
  if (item.kind === "task") {
    return item.task;
  }

  if (item.exchange.request) {
    return item.exchange.request;
  }
  if (item.exchange.response) {
    return item.exchange.response;
  }
  throw new Error("timeline exchange is missing both request and response events");
}

// Maps a normalized timeline item to its visual tone.
export function getTimelineItemTone(item: TimelineItem): "neutral" | "active" | "completed" | "failed" {
  if (item.kind === "task") {
    return getEventTone(item.task);
  }

  return getExchangeTone(item.exchange);
}

// Builds the primary label shown for a timeline item.
export function getTimelineItemTitle(item: TimelineItem): string {
  if (item.kind === "task") {
    return getEventTitle(item.task);
  }

  return getExchangeTitle(item.exchange);
}

// Builds the secondary line shown underneath the main timeline label.
export function getTimelineItemSubtitle(item: TimelineItem): string {
  if (item.kind === "task") {
    return getEventSubtitle(item.task);
  }

  return getExchangeSubtitle(item.exchange);
}

// Produces the status badge text used in the inspector.
export function getTimelineItemStatusLabel(item: TimelineItem): string {
  if (item.kind === "task") {
    return getEventStatusLabel(item.task);
  }

  return getExchangeStatusLabel(item.exchange);
}

// Flags failed items with a short alert marker for the compact timeline rows.
export function getTimelineItemAlertLabel(item: TimelineItem): string | undefined {
  const tone = getTimelineItemTone(item);
  if (tone === "failed") {
    return "error";
  }

  return undefined;
}

// Centian request/response pairs can be hidden when they already appear inside a task event.
function isCollapsibleCentianExchange(exchange: TimelineExchange): boolean {
  return getExchangeServerName(exchange) === "centian";
}

// Chooses the phase bucket for an item, carrying the previous phase forward when needed.
function getGroupingPhase(event: TaskRunEvent, lastKnownPhase: string): string {
  if (event.source === "task" && shouldStickToCurrentPhase(event) && event.phasePath) {
    return event.phasePath;
  }

  return event.resultingPhasePath || event.phasePath || lastKnownPhase || "unknown";
}

// Completed lifecycle events should remain grouped under the phase they just finished.
function shouldStickToCurrentPhase(event: TaskRunEvent): boolean {
  if (event.source !== "task") {
    return false;
  }

  const payloadStatus = readPayloadStatus(event.payloadJson);
  return (
    event.eventType === "onboarding_completed" ||
    event.eventType === "planning_completed" ||
    event.eventType === "step_completed" ||
    event.eventType === "task_completed" ||
    event.eventType === "task_timed_out" ||
    event.eventType === "task_failed" ||
    payloadStatus === "completed" ||
    payloadStatus === "timed_out" ||
    payloadStatus === "failed"
  );
}

// Derives the header badge status from the latest task lifecycle event, with action failures as fallback.
function deriveTaskRunDetailStatus(events: TaskRunEvent[]): TaskRunUIStatus {
  for (let index = events.length - 1; index >= 0; index -= 1) {
    const event = events[index];
    if (event.source !== "task") {
      continue;
    }

    const payloadStatus = readPayloadStatus(event.payloadJson);
    if (payloadStatus === "timed_out" || event.eventType === "task_timed_out") {
      return "timed_out";
    }
    if (payloadStatus === "failed" || event.eventType === "task_failed") {
      return "failed";
    }
    if (payloadStatus === "completed") {
      return "success";
    }
    return "active";
  }

  const latestActionFailed = events.some((event) => event.source === "action" && getEventTone(event) === "failed");
  return latestActionFailed ? "failed" : "active";
}

// Reads a best-effort status field from arbitrary payload objects.
function readPayloadStatus(payload: unknown): string | undefined {
  if (payload == null || typeof payload !== "object" || Array.isArray(payload)) {
    return undefined;
  }

  const candidate = (payload as { status?: unknown }).status;
  return typeof candidate === "string" ? candidate : undefined;
}

// Creates the title line for any single raw event.
function getEventTitle(event: TaskRunEvent): string {
  if (event.source === "task") {
    const stepName = getTaskStepDisplayName(event);
    if (event.eventType === "step_started") {
      return stepName ? `Step Started · ${stepName}` : "Step Started";
    }
    if (event.eventType === "step_completed") {
      return stepName ? `Step Completed · ${stepName}` : "Step Completed";
    }
    return humanizeIdentifier(event.eventType ?? "task_event");
  }

  if (event.toolName) {
    return event.toolName;
  }
  return humanizeIdentifier(event.messageType ?? "action_event");
}

// Creates the subtitle line for a raw event using the most relevant context available.
function getEventSubtitle(event: TaskRunEvent): string {
  if (event.source === "task") {
    const phaseLine = formatTaskPhaseLine(event);
    if (phaseLine) {
      return phaseLine;
    }
    return humanizePhase(event.resultingPhasePath ?? event.phasePath ?? "unknown");
  }

  const parts = [event.serverName, event.gateway, event.transport].filter(Boolean);
  if (parts.length > 0) {
    return parts.join(" · ");
  }
  return humanizeIdentifier(event.messageType ?? "unknown");
}

// Uses the tool name as the primary label for an exchange whenever available.
function getExchangeTitle(exchange: TimelineExchange): string {
  const toolName = exchange.request?.toolName ?? exchange.response?.toolName;
  if (toolName) {
    return toolName;
  }

  const messageType = exchange.request?.messageType ?? exchange.response?.messageType;
  return humanizeIdentifier(messageType ?? "mcp_exchange");
}

// Pulls a short preview from the request payload for the compact timeline row.
function getExchangeSubtitle(exchange: TimelineExchange): string {
  const requestPayload = readPayloadObject(exchange.request?.payloadJson);
  const preview = extractPayloadPreview(requestPayload);
  if (preview) {
    return preview;
  }

  return "";
}

// Collapses exchange completion into the same tone vocabulary used by task events.
function getExchangeTone(exchange: TimelineExchange): "neutral" | "active" | "completed" | "failed" {
  const response = exchange.response;
  if (response) {
    if (response.isError === true || response.success === false) {
      return "failed";
    }
    return "completed";
  }

  return "active";
}

// Converts exchange success/error flags into human-readable status text.
function getExchangeStatusLabel(exchange: TimelineExchange): string {
  const response = exchange.response;
  if (!response) {
    return "pending";
  }
  if (response.isError === true || response.success === false) {
    return "failed";
  }
  if (response.success === true) {
    return "success";
  }
  return "completed";
}

// Measures request/response latency when both halves of the exchange exist.
export function getExchangeLatency(exchange: TimelineExchange): number | undefined {
  if (!exchange.request || !exchange.response) {
    return undefined;
  }

  return Math.max(0, exchange.response.createdAtUnixMilli - exchange.request.createdAtUnixMilli);
}

// Formats latency for badges in milliseconds or seconds depending on size.
export function formatLatency(durationMs: number): string {
  if (durationMs < 1000) {
    return `${durationMs}ms`;
  }

  return `${(durationMs / 1000).toFixed(durationMs >= 10_000 ? 0 : 1)}s`;
}

// Renders the denser timestamp format used in the timeline rail.
export function formatTraceTimestamp(timestamp: number): string {
  const date = new Date(timestamp);
  const hours = String(date.getHours()).padStart(2, "0");
  const minutes = String(date.getMinutes()).padStart(2, "0");
  const seconds = String(date.getSeconds()).padStart(2, "0");
  const milliseconds = String(date.getMilliseconds()).padStart(3, "0");
  return `${hours}:${minutes}:${seconds}.${milliseconds}`;
}

// Returns the most specific server name attached to an exchange.
export function getExchangeServerName(exchange: TimelineExchange): string {
  return exchange.request?.serverName ?? exchange.response?.serverName ?? "mcp";
}

// Keeps the exchange label API separate in case display names diverge later.
export function getExchangeServerLabel(exchange: TimelineExchange): string {
  return getExchangeServerName(exchange);
}

// Narrows unknown payloads to plain object records for the preview helpers below.
function readPayloadObject(payload: unknown): Record<string, unknown> | undefined {
  if (payload == null || typeof payload !== "object" || Array.isArray(payload)) {
    return undefined;
  }

  return payload as Record<string, unknown>;
}

// Keeps path-like previews short by showing only the trailing segments.
function summarizePath(path: string): string {
  const segments = path.split("/").filter(Boolean);
  if (segments.length === 0) {
    return path;
  }
  return segments.slice(-2).join("/");
}

// Truncates long preview text while preserving room for the ellipsis.
function truncateText(value: string, maxLength: number): string {
  if (value.length <= maxLength) {
    return value;
  }

  return `${value.slice(0, maxLength - 1)}…`;
}

// Walks a payload tree to find a concise summary string for timeline subtitles.
function extractPayloadPreview(payload: unknown, depth = 0): string {
  if (depth > 3 || payload == null) {
    return "";
  }

  if (typeof payload === "string") {
    const trimmed = payload.trim();
    if (trimmed === "") {
      return "";
    }
    return truncateText(trimmed, 88);
  }

  if (Array.isArray(payload)) {
    const textItems = payload
      .filter((item): item is string => typeof item === "string" && item.trim() !== "")
      .slice(0, 3)
      .map((item) => formatPreviewString(item));
    if (textItems.length > 0) {
      return truncateText(textItems.join(" · "), 88);
    }

    for (const item of payload) {
      const preview = extractPayloadPreview(item, depth + 1);
      if (preview) {
        return preview;
      }
    }

    return "";
  }

  if (typeof payload !== "object") {
    return "";
  }

  const record = payload as Record<string, unknown>;

  // Prefer the fields operators usually care about first, then fall back to deeper inspection.
  const prioritizedKeys = [
    "command",
    "cmd",
    "path",
    "filePath",
    "file_path",
    "targetPath",
    "target_path",
    "directory",
    "cwd",
    "templateId",
    "template_id",
    "taskSummary",
    "project_summary",
    "input",
    "message",
  ];

  for (const key of prioritizedKeys) {
    const value = record[key];
    if (typeof value === "string" && value.trim() !== "") {
      return truncateText(formatPreviewString(value), 88);
    }
  }

  const listKeys = ["paths", "files", "filePaths", "file_paths"];
  for (const key of listKeys) {
    const value = record[key];
    if (!Array.isArray(value)) {
      continue;
    }

    const items = value
      .filter((item): item is string => typeof item === "string" && item.trim() !== "")
      .slice(0, 3)
      .map((item) => formatPreviewString(item));
    if (items.length > 0) {
      return truncateText(items.join(" · "), 88);
    }
  }

  if (typeof record.step === "number") {
    return `step ${record.step}`;
  }

  // Search nested argument-like objects before scanning every remaining property.
  const nestedKeys = [
    "tool_call",
    "arguments",
    "args",
    "params",
    "parameters",
    "requiredInputs",
    "requiredInputNames",
    "request",
    "payload",
  ];
  for (const key of nestedKeys) {
    const preview = extractPayloadPreview(record[key], depth + 1);
    if (preview) {
      return preview;
    }
  }

  for (const value of Object.values(record)) {
    const preview = extractPayloadPreview(value, depth + 1);
    if (preview) {
      return preview;
    }
  }

  return "";
}

// Applies path compaction only when a preview string looks file-system-like.
function formatPreviewString(value: string): string {
  if (value.includes("/")) {
    return summarizePath(value);
  }

  return value;
}

// Tries several payload and phase fields to produce the most useful step name for task events.
function getTaskStepDisplayName(event: TaskRunEvent): string {
  const payload = readPayloadObject(event.payloadJson);
  const payloadCandidates = [
    payload?.stepName,
    payload?.step_name,
    payload?.stepTitle,
    payload?.step_title,
  ];

  for (const candidate of payloadCandidates) {
    if (typeof candidate === "string" && candidate.trim() !== "") {
      return humanizePhase(candidate);
    }
  }

  const phaseCandidates =
    event.eventType === "step_started"
      ? [event.resultingPhasePath, event.phasePath]
      : [event.phasePath, event.resultingPhasePath];

  for (const candidate of phaseCandidates) {
    if (typeof candidate === "string" && candidate.trim() !== "") {
      return humanizePhase(candidate);
    }
  }

  if (typeof payload?.step === "number") {
    return `Step ${payload.step}`;
  }

  return "";
}

// Builds a "from -> to" phase transition label when both sides are known.
function formatTaskPhaseLine(event: TaskRunEvent): string {
  const from = event.phasePath ? humanizePhase(event.phasePath) : "";
  const to = event.resultingPhasePath ? humanizePhase(event.resultingPhasePath) : "";

  if (from && to && from !== to) {
    return `${from} → ${to}`;
  }

  return to || from;
}

// Maps task and action events into the shared tone model used by the UI.
function getEventTone(event: TaskRunEvent): "neutral" | "active" | "completed" | "failed" {
  if (event.source === "task") {
    const payloadStatus = readPayloadStatus(event.payloadJson);
    if (event.eventType === "task_timed_out" || payloadStatus === "timed_out") {
      return "neutral";
    }
    if (event.outcome === "failed" || event.eventType === "task_failed") {
      return "failed";
    }
    if (isCompletedTaskLifecycleEvent(event) || payloadStatus === "completed" || payloadStatus === "succeeded") {
      return "completed";
    }
    return "active";
  }

  if (event.isError === true || event.success === false) {
    return "failed";
  }
  if (event.success === true) {
    return "completed";
  }
  return "neutral";
}

// Generates the compact status label shown next to event payloads.
function getEventStatusLabel(event: TaskRunEvent): string {
  if (event.source === "task") {
    const tone = getEventTone(event);
    const payloadStatus = readPayloadStatus(event.payloadJson);
    if (tone === "failed") {
      return "error";
    }
    if (tone === "completed") {
      return "success";
    }
    if (event.outcome === "succeeded") {
      return "success";
    }
    if (tone === "neutral" && payloadStatus === "timed_out") {
      return "timed_out";
    }
    if (payloadStatus) {
      return normalizeInspectorStatusLabel(payloadStatus);
    }
    return normalizeInspectorStatusLabel(event.outcome ?? "tracked");
  }
  if (event.isError === true || event.success === false) {
    return "error";
  }
  if (event.success === true) {
    return "success";
  }
  return normalizeInspectorStatusLabel(event.direction ?? "event");
}

function isCompletedTaskLifecycleEvent(event: TaskRunEvent): boolean {
  switch (event.eventType) {
    case "onboarding_completed":
    case "planning_completed":
    case "step_completed":
    case "task_completed":
    case "resumed":
    case "restarted":
      return true;
    default:
      return false;
  }
}

function normalizeInspectorStatusLabel(value: string): string {
  switch (value) {
    case "succeeded":
    case "completed":
    case "ok":
      return "success";
    default:
      return value;
  }
}

// Serializes payloads defensively so the inspector can always render something readable.
function formatPayload(payload: unknown): string {
  if (payload == null) {
    return "null";
  }
  if (typeof payload === "string") {
    return payload;
  }

  try {
    return JSON.stringify(payload, null, 2) ?? String(payload);
  } catch {
    return String(payload);
  }
}

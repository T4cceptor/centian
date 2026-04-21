import { type CSSProperties, useEffect, useMemo, useRef, useState } from "react";
import { Link, useLocation, useParams } from "react-router-dom";

import { ApiError, fetchTaskRunDetail, fetchTaskRunEvents, type TaskRunDetailMetadata, type TaskRunEvent } from "../api/task-runs";
import { ApiAuthCard } from "./api-auth-card";
import { formatTimestamp, formatDuration, formatTaskRunId, humanizeIdentifier, humanizePhase } from "./format";
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
  const { runID } = useParams();
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
      fetchTaskRunEvents(runID, controller.signal),
      fetchTaskRunDetail(runID, controller.signal),
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
  }, [reloadToken, runID]);

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

      void fetchTaskRunEvents(runID, controller.signal)
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
  }, [detailStatus, loadState, runID]);

  const timelineItems = useMemo(() => buildTimelineItems(events), [events]);
  const groupedEvents = useMemo(() => groupEventsByPhase(timelineItems), [timelineItems]);
  const flatTimelineItems = useMemo(
    () => groupedEvents.flatMap((group) => group.items),
    [groupedEvents],
  );
  const selectedItem = flatTimelineItems.find((item) => item.id === selectedItemID);
  const inspectorVisible = selectedItemID !== "" && selectedItem != null;

  const runStats = useMemo(() => {
    const startedAt = events[0]?.createdAtUnixMilli;
    const lastSeenAt = events.length > 0 ? events[events.length - 1].createdAtUnixMilli : undefined;

    let errorCount = 0;
    // Aggregate lightweight summary stats directly from the flattened render model.
    for (const item of flatTimelineItems) {
      if (item.kind === "exchange") {
        const resp = item.exchange.response;
        if (resp && (resp.isError === true || resp.success === false)) {
          errorCount++;
        }
      } else if (item.kind === "task") {
        if (item.task.outcome === "failed" || item.task.eventType === "task_failed") {
          errorCount++;
        }
      }
    }

    return {
      startedAt,
      durationEndedAt: detailStatus === "active" ? undefined : lastSeenAt,
      errorCount,
      totalEvents: events.length,
    };
  }, [detailStatus, events, flatTimelineItems]);

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
        <div className="task-run-detail__title-block">
          <p className="state-card__eyebrow">
            <span>Run Detail</span>
            <span style={{ opacity: 0.35, margin: "0 8px" }}>·</span>
            {formatTaskRunId(runID ?? "")}
            <span style={{ opacity: 0.35, margin: "0 8px" }}>·</span>
            <span className={`status-badge status-badge--${detailStatus}`}>{detailStatus}</span>
          </p>
        </div>
        <div className="task-run-detail__header-actions">
          <Link className="back-link" style={{fontFamily:"inter"}} to={`/tasks${location.search || ""}`}>
            Back to task runs
          </Link>
        </div>
      </header>

      <RunMetadataBar stats={runStats} now={now} />
      {detailMetadata?.benchmarkLinks && detailMetadata.benchmarkLinks.length > 0 ? (
        <BenchmarkContextPanel links={detailMetadata.benchmarkLinks} />
      ) : null}

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
  errorCount: number;
  totalEvents: number;
};

function BenchmarkContextPanel({ links }: { links: NonNullable<TaskRunDetailMetadata["benchmarkLinks"]> }) {
  return (
    <section className="task-run-benchmark-context">
      <div className="task-run-benchmark-context__header">
        <div>
          <p className="state-card__eyebrow">Benchmark Context</p>
          <h3>Linked benchmark runs</h3>
        </div>
        <p>{links.length} linked benchmark run{links.length === 1 ? "" : "s"}</p>
      </div>
      <div className="task-run-benchmark-context__grid">
        {links.map((link) => (
          <article key={`${link.benchmarkRunId}:${link.caseId}:${link.attempt}`} className="task-run-benchmark-card">
            <h4>{link.caseName || link.caseId}</h4>
            <p>{link.suiteName || link.suiteId}</p>
            <p>{link.agent}{link.selectedModel ? ` / ${link.selectedModel}` : ""} · {link.templateVariant} · attempt {link.attempt}</p>
            <p>Started {formatTimestamp(link.startedAtUnixMilli)}</p>
            <div className="task-run-benchmark-card__links">
              <Link to={`/benchmarks/${link.suiteId}/runs/${link.benchmarkRunId}`}>Open benchmark run</Link>
              <Link to={`/tasks?benchmarkSuite=${encodeURIComponent(link.suiteId)}`}>Show suite task runs</Link>
            </div>
          </article>
        ))}
      </div>
    </section>
  );
}

// Displays the headline metrics for the selected run.
function RunMetadataBar({ stats, now }: { stats: RunStats; now: number }) {
  // Shared inline styles keep the compact metadata row visually consistent.
  const cellStyle: CSSProperties = {
    display: "flex",
    flexDirection: "column",
    gap: 2,
  };
  const labelStyle: CSSProperties = {
    fontSize: 10,
    fontWeight: 600,
    letterSpacing: "0.05em",
    textTransform: "uppercase",
    color: "#5a7a9a",
  };
  const valueStyle: CSSProperties = {
    fontSize: 13,
    color: "#c8daf0",
    fontFamily: "'JetBrains Mono', 'Fira Code', monospace",
  };

  return (
    <div
      style={{
        display: "flex",
        flexWrap: "wrap",
        gap: 24,
        padding: "10px 20px",
        background: "linear-gradient(135deg, rgba(10,14,30,0.7), rgba(15,22,42,0.5))",
        borderBottom: "1px solid rgba(100,140,200,0.1)",
        fontFamily: "system-ui, -apple-system, BlinkMacSystemFont, 'Segoe UI', sans-serif",
        alignItems: "flex-start",
      }}
    >
      {stats.startedAt != null && (
        <div style={cellStyle}>
          <span style={labelStyle}>Started</span>
          <span style={valueStyle}>{formatTimestamp(stats.startedAt)}</span>
        </div>
      )}

      {stats.startedAt != null && (
        <div style={cellStyle}>
          <span style={labelStyle}>Duration</span>
          <span style={valueStyle}>
            {formatDuration(stats.startedAt, stats.durationEndedAt, now)}
          </span>
        </div>
      )}

      <div style={cellStyle}>
        <span style={labelStyle}>Events</span>
        <span style={valueStyle}>{stats.totalEvents}</span>
      </div>

      {stats.errorCount > 0 && (
        <div style={cellStyle}>
          <span style={labelStyle}>Errors</span>
          <span style={{ ...valueStyle, color: "#fb7185" }}>{stats.errorCount}</span>
        </div>
      )}
    </div>
  );
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

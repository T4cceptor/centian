import { type CSSProperties, useEffect, useMemo, useRef, useState } from "react";
import { Link, useParams } from "react-router-dom";

import { ApiError, fetchTaskRunEvents, type TaskRunEvent } from "../api/task-runs";
import { formatTimestamp, humanizeIdentifier, humanizePhase } from "./format";
import { type TaskRunUIStatus } from "./task-run-status";

type LoadState = "loading" | "ready" | "invalid" | "not-found" | "error";

type TimelineGroup = {
  key: string;
  label: string;
  items: TimelineItem[];
};

type TimelineExchange = {
  requestId?: string;
  request?: TaskRunEvent;
  response?: TaskRunEvent;
};

type TimelineItem =
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

export function TaskRunDetailPage() {
  const { runID } = useParams();
  const [events, setEvents] = useState<TaskRunEvent[]>([]);
  const [loadState, setLoadState] = useState<LoadState>("loading");
  const [collapsedGroups, setCollapsedGroups] = useState<Record<string, boolean>>({});
  const [selectedItemID, setSelectedItemID] = useState<string>("");
  const [detailsWidth, setDetailsWidth] = useState(420);
  const [draggingResize, setDraggingResize] = useState(false);
  const previousExpandedWidthRef = useRef(420);

  useEffect(() => {
    if (!runID) {
      setLoadState("invalid");
      setEvents([]);
      return;
    }

    const controller = new AbortController();
    setLoadState("loading");
    setCollapsedGroups({});
    setSelectedItemID("");

    void fetchTaskRunEvents(runID, controller.signal)
      .then((result) => {
        setEvents(result);
        setLoadState("ready");
      })
      .catch((error: unknown) => {
        if ((error as Error)?.name === "AbortError") {
          return;
        }
        if (error instanceof ApiError) {
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
  }, [runID]);

  const timelineItems = useMemo(() => buildTimelineItems(events), [events]);
  const groupedEvents = useMemo(() => groupEventsByPhase(timelineItems), [timelineItems]);
  const detailStatus = useMemo(() => deriveTaskRunDetailStatus(events), [events]);
  const flatTimelineItems = useMemo(
    () => groupedEvents.flatMap((group) => group.items),
    [groupedEvents],
  );
  const selectedItem = flatTimelineItems.find((item) => item.id === selectedItemID);
  const inspectorVisible = selectedItemID !== "" && selectedItem != null;
  const startedAt = events[0]?.createdAtUnixMilli;
  const lastSeenAt = events.length > 0 ? events[events.length - 1].createdAtUnixMilli : undefined;

  useEffect(() => {
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

  return (
    <div className="task-run-detail">
      <header className="task-run-detail__header">
        <div className="task-run-detail__title-block">
          <p className="state-card__eyebrow">Run Detail</p>
          <h2>Task run timeline</h2>
          <code className="task-run-detail__run-id">{runID}</code>
        </div>
        <div className="task-run-detail__header-actions">
          <span className={`status-badge status-badge--${detailStatus}`}>{detailStatus}</span>
          <Link className="back-link" to="/tasks">
            Back to task runs
          </Link>
        </div>
      </header>

      <div className="task-run-detail__summary">
        <div className="task-run-summary-card">
          <span className="task-run-summary-card__label">Started</span>
          <strong>{startedAt ? formatTimestamp(startedAt) : "Unknown"}</strong>
        </div>
        <div className="task-run-summary-card">
          <span className="task-run-summary-card__label">Last activity</span>
          <strong>{lastSeenAt ? formatTimestamp(lastSeenAt) : "Unknown"}</strong>
        </div>
        <div className="task-run-summary-card">
          <span className="task-run-summary-card__label">Events</span>
          <strong>{events.length}</strong>
        </div>
      </div>

      <div
        className="task-run-detail__workspace"
        style={
          inspectorVisible
            ? ({
                "--task-detail-sidebar-width": `${detailsWidth + 20}px`,
              } as CSSProperties)
            : undefined
        }
      >
        <div className="timeline">
          {groupedEvents.map((group, groupIndex) => (
            <section key={group.key} className="timeline-group" aria-label={group.label}>
              <button
                type="button"
                className="timeline-group__toggle"
                aria-expanded={!collapsedGroups[group.key]}
                onClick={() =>
                  setCollapsedGroups((current) => ({
                    ...current,
                    [group.key]: !current[group.key],
                  }))
                }
              >
                <div className="timeline-group__header">
                  <span className="timeline-group__sector">
                    Sector {String(groupIndex + 1).padStart(2, "0")}
                  </span>
                  <span className="timeline-group__label">{group.label}</span>
                  <span className="timeline-group__count">{group.items.length} events</span>
                  <div className="timeline-group__rule" />
                  <span className="timeline-group__chevron" aria-hidden="true">
                    {collapsedGroups[group.key] ? "+" : "-"}
                  </span>
                </div>
              </button>

              {!collapsedGroups[group.key] ? (
                <div className="timeline-group__events">
                  {group.items.map((item) => {
                    const anchorEvent = getTimelineAnchorEvent(item);
                    const tone = getTimelineItemTone(item);
                    const visual = getTimelineItemVisuals(item, tone);
                    const title = getTimelineItemTitle(item);
                    const subtitle = getTimelineItemSubtitle(item);
                    const statusLabel = getTimelineItemStatusLabel(item);
                    const exchangeLatency =
                      item.kind === "exchange" ? getExchangeLatency(item.exchange) : undefined;
                    const metaLabel =
                      item.kind === "task" ? "task" : getExchangeServerLabel(item.exchange);
                    const selected = selectedItem?.id === item.id;

                    return (
                      <article
                        key={item.id}
                        className={`timeline-event timeline-event--${tone} ${
                          selected ? "timeline-event--selected" : ""
                        }`}
                        style={visual.style}
                      >
                        <div className="timeline-event__timestamp">
                          <time dateTime={new Date(anchorEvent.createdAtUnixMilli).toISOString()}>
                            {formatTimestamp(anchorEvent.createdAtUnixMilli)}
                          </time>
                        </div>

                        <div className="timeline-event__marker-column">
                          <span className="timeline-event__halo" />
                          <span className="timeline-event__ring" />
                          <span
                            className={`timeline-event__icon timeline-event__icon--${visual.shape}`}
                            aria-hidden="true"
                          >
                            <EventGlyph kind={visual.iconKind} />
                          </span>
                        </div>

                        <div className="timeline-event__card">
                          <div className="timeline-event__connector" />
                          <div className="timeline-event__content">
                            <button
                              type="button"
                              className="timeline-event__summary"
                              aria-pressed={selected}
                              aria-label={`Show event details for ${title}`}
                              onClick={() => {
                                setSelectedItemID(item.id);
                              }}
                            >
                              <div className="timeline-event__meta">
                                <span
                                  className={`timeline-source-badge timeline-source-badge--${item.kind === "task" ? "task" : "exchange"}`}
                                >
                                  {metaLabel}
                                </span>
                                {exchangeLatency != null ? (
                                  <span className="timeline-event__metric">
                                    {formatLatency(exchangeLatency)}
                                  </span>
                                ) : null}
                                <span className={`timeline-event__status timeline-event__status--${tone}`}>
                                  {statusLabel}
                                </span>
                              </div>

                              <div className="timeline-event__body">
                                <div>
                                  <h3 className="timeline-event__title" data-testid="timeline-event-title">
                                    {title}
                                  </h3>
                                  {subtitle ? <p className="timeline-event__subtitle">{subtitle}</p> : null}
                                  {item.kind === "task" && item.correlatedExchange ? (
                                    <p className="timeline-event__linked-action">
                                      {getLinkedExchangeLabel(item.correlatedExchange)}
                                    </p>
                                  ) : null}
                              </div>
                              <span className="timeline-event__details-link">
                                {selected && inspectorVisible ? "Inspecting" : "Inspect"}
                              </span>
                            </div>
                          </button>
                          </div>
                        </div>
                      </article>
                    );
                  })}
                </div>
              ) : null}
            </section>
          ))}
        </div>

        {inspectorVisible ? (
          <aside className="task-detail-panel" style={{ width: detailsWidth }}>
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

function DetailInspector({ item }: { item: TimelineItem }) {
  const tone = getTimelineItemTone(item);
  const statusLabel = getTimelineItemStatusLabel(item);
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
          <span className={`timeline-event__status timeline-event__status--${tone}`}>{statusLabel}</span>
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

function clampDetailsWidth(value: number): number {
  return Math.min(680, Math.max(320, Math.round(value)));
}

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
  for (const item of rawItems) {
    if (
      item.kind === "exchange" &&
      item.exchange.requestId &&
      isCollapsibleCentianExchange(item.exchange)
    ) {
      centianExchangesByRequestID.set(item.exchange.requestId, item.exchange);
    }
  }

  const hiddenExchangeRequestIDs = new Set<string>();
  const items: TimelineItem[] = [];
  for (const item of rawItems) {
    if (item.kind === "task") {
      const correlatedExchange =
        item.task.relatedActionRequestId != null
          ? centianExchangesByRequestID.get(item.task.relatedActionRequestId)
          : undefined;
      if (correlatedExchange?.requestId) {
        hiddenExchangeRequestIDs.add(correlatedExchange.requestId);
      }
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

function createSingletonExchange(event: TaskRunEvent): TimelineExchange {
  const requestId = getExchangeRequestID(event);
  if (isRequestAction(event)) {
    return {
      requestId,
      request: event,
    };
  }

  return {
    requestId,
    response: event,
  };
}

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

function groupEventsByPhase(items: TimelineItem[]): TimelineGroup[] {
  const groups: TimelineGroup[] = [];
  let lastKnownPhase = "";

  for (const item of items) {
    const event = getTimelineAnchorEvent(item);
    const effectivePhase = getGroupingPhase(event, lastKnownPhase);
    lastKnownPhase = effectivePhase;

    const existingGroup = groups.length > 0 ? groups[groups.length - 1] : undefined;
    if (existingGroup == null || existingGroup.key !== effectivePhase) {
      groups.push({
        key: effectivePhase,
        label: humanizePhase(effectivePhase),
        items: [item],
      });
      continue;
    }

    existingGroup.items.push(item);
  }

  return groups;
}

function getTimelineAnchorEvent(item: TimelineItem): TaskRunEvent {
  if (item.kind === "task") {
    return item.task;
  }

  return item.exchange.request ?? item.exchange.response ?? item.exchange.request!;
}

function getTimelineItemTone(item: TimelineItem): "neutral" | "active" | "completed" | "failed" {
  if (item.kind === "task") {
    return getEventTone(item.task);
  }

  return getExchangeTone(item.exchange);
}

function getTimelineItemTitle(item: TimelineItem): string {
  if (item.kind === "task") {
    return getEventTitle(item.task);
  }

  return getExchangeTitle(item.exchange);
}

function getTimelineItemSubtitle(item: TimelineItem): string {
  if (item.kind === "task") {
    return getEventSubtitle(item.task);
  }

  return getExchangeSubtitle(item.exchange);
}

function getTimelineItemStatusLabel(item: TimelineItem): string {
  if (item.kind === "task") {
    return getEventStatusLabel(item.task);
  }

  return getExchangeStatusLabel(item.exchange);
}

function getTimelineItemVisuals(
  item: TimelineItem,
  tone: "neutral" | "active" | "completed" | "failed",
): {
  channelLabel: string;
  iconKind: "task" | "filesystem" | "shell" | "server";
  shape: "hex" | "diamond" | "circle";
  style: CSSProperties;
} {
  if (item.kind === "task") {
    return getEventVisuals(item.task, tone);
  }

  const representative = item.exchange.response ?? item.exchange.request;
  return getEventVisuals(representative ?? item.exchange.request!, tone);
}

function isCollapsibleCentianExchange(exchange: TimelineExchange): boolean {
  return getExchangeServerName(exchange) === "centian";
}

function getGroupingPhase(event: TaskRunEvent, lastKnownPhase: string): string {
  if (event.source === "task" && shouldStickToCurrentPhase(event) && event.phasePath) {
    return event.phasePath;
  }

  return event.resultingPhasePath || event.phasePath || lastKnownPhase || "unknown";
}

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
    event.eventType === "task_failed" ||
    payloadStatus === "completed" ||
    payloadStatus === "failed"
  );
}

function deriveTaskRunDetailStatus(events: TaskRunEvent[]): TaskRunUIStatus {
  for (let index = events.length - 1; index >= 0; index -= 1) {
    const event = events[index];
    if (event.source !== "task") {
      continue;
    }

    const payloadStatus = readPayloadStatus(event.payloadJson);
    if (payloadStatus === "failed" || event.eventType === "task_failed" || event.outcome === "failed") {
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

function readPayloadStatus(payload: unknown): string | undefined {
  if (payload == null || typeof payload !== "object" || Array.isArray(payload)) {
    return undefined;
  }

  const candidate = (payload as { status?: unknown }).status;
  return typeof candidate === "string" ? candidate : undefined;
}

function getEventTitle(event: TaskRunEvent): string {
  if (event.source === "task") {
    return humanizeIdentifier(event.eventType ?? "task_event");
  }

  if (event.toolName) {
    return `${humanizeIdentifier(event.direction ?? "action")} · ${event.toolName}`;
  }
  return humanizeIdentifier(event.messageType ?? "action_event");
}

function getEventSubtitle(event: TaskRunEvent): string {
  if (event.source === "task") {
    const parts = [event.nodeKind, event.resultingNodeKind].filter(Boolean);
    if (parts.length > 0) {
      return parts.join(" → ");
    }
    return humanizePhase(event.resultingPhasePath ?? event.phasePath ?? "unknown");
  }

  const parts = [event.serverName, event.gateway, event.transport].filter(Boolean);
  if (parts.length > 0) {
    return parts.join(" · ");
  }
  return humanizeIdentifier(event.messageType ?? "unknown");
}

function getExchangeTitle(exchange: TimelineExchange): string {
  const toolName = exchange.request?.toolName ?? exchange.response?.toolName;
  if (toolName) {
    return toolName;
  }

  const messageType = exchange.request?.messageType ?? exchange.response?.messageType;
  return humanizeIdentifier(messageType ?? "mcp_exchange");
}

function getExchangeSubtitle(exchange: TimelineExchange): string {
  const requestPayload = readPayloadObject(exchange.request?.payloadJson);
  if (requestPayload) {
    if (typeof requestPayload.command === "string" && requestPayload.command.trim() !== "") {
      return truncateText(requestPayload.command, 72);
    }

    if (typeof requestPayload.path === "string" && requestPayload.path.trim() !== "") {
      return summarizePath(requestPayload.path);
    }

    if (Array.isArray(requestPayload.paths) && requestPayload.paths.length > 0) {
      return requestPayload.paths
        .filter((path): path is string => typeof path === "string" && path.trim() !== "")
        .slice(0, 3)
        .map((path) => summarizePath(path))
        .join(" · ");
    }

    if (typeof requestPayload.step === "number") {
      return `Step ${requestPayload.step}`;
    }

    if (typeof requestPayload.templateId === "string" && requestPayload.templateId.trim() !== "") {
      return requestPayload.templateId;
    }
  }

  const gateway = exchange.request?.gateway ?? exchange.response?.gateway;
  const transport = exchange.request?.transport ?? exchange.response?.transport;
  const serverName = getExchangeServerName(exchange);
  if (gateway && gateway !== serverName) {
    return gateway;
  }
  if (transport) {
    return transport.toUpperCase();
  }

  return "";
}

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

function getExchangeLatency(exchange: TimelineExchange): number | undefined {
  if (!exchange.request || !exchange.response) {
    return undefined;
  }

  return Math.max(0, exchange.response.createdAtUnixMilli - exchange.request.createdAtUnixMilli);
}

function formatLatency(durationMs: number): string {
  if (durationMs < 1000) {
    return `${durationMs}ms`;
  }

  return `${(durationMs / 1000).toFixed(durationMs >= 10_000 ? 0 : 1)}s`;
}

function getExchangeServerName(exchange: TimelineExchange): string {
  return exchange.request?.serverName ?? exchange.response?.serverName ?? "mcp";
}

function getExchangeServerLabel(exchange: TimelineExchange): string {
  return getExchangeServerName(exchange);
}

function getLinkedExchangeLabel(exchange: TimelineExchange): string {
  return `Centian MCP · ${getExchangeTitle(exchange)} · ${getExchangeStatusLabel(exchange)}`;
}

function readPayloadObject(payload: unknown): Record<string, unknown> | undefined {
  if (payload == null || typeof payload !== "object" || Array.isArray(payload)) {
    return undefined;
  }

  return payload as Record<string, unknown>;
}

function summarizePath(path: string): string {
  const segments = path.split("/").filter(Boolean);
  if (segments.length === 0) {
    return path;
  }
  return segments.slice(-2).join("/");
}

function truncateText(value: string, maxLength: number): string {
  if (value.length <= maxLength) {
    return value;
  }

  return `${value.slice(0, maxLength - 1)}…`;
}

function getEventTone(event: TaskRunEvent): "neutral" | "active" | "completed" | "failed" {
  if (event.source === "task") {
    if (event.outcome === "failed" || event.eventType === "task_failed") {
      return "failed";
    }
    if (event.eventType === "step_completed" || readPayloadStatus(event.payloadJson) === "completed") {
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

function getEventStatusLabel(event: TaskRunEvent): string {
  if (event.source === "task") {
    return event.outcome ?? "tracked";
  }
  if (event.isError === true || event.success === false) {
    return "error";
  }
  if (event.success === true) {
    return "ok";
  }
  return event.direction ?? "event";
}

function getEventVisuals(
  event: TaskRunEvent,
  tone: "neutral" | "active" | "completed" | "failed",
): {
  channelLabel: string;
  iconKind: "task" | "filesystem" | "shell" | "server";
  shape: "hex" | "diamond" | "circle";
  style: CSSProperties;
} {
  let color = "#8ce6d8";
  let glow = "rgba(140, 230, 216, 0.5)";
  let background = "rgba(140, 230, 216, 0.08)";
  let shape: "hex" | "diamond" | "circle" = "hex";
  let channelLabel = "centian";
  let iconKind: "task" | "filesystem" | "shell" | "server" = "server";

  if (event.source === "task") {
    color = "#a78bfa";
    glow = "rgba(167, 139, 250, 0.55)";
    background = "rgba(167, 139, 250, 0.08)";
    channelLabel = "task";
    iconKind = "task";
    shape = "hex";
  } else if (event.serverName === "filesystem") {
    color = "#34d399";
    glow = "rgba(52, 211, 153, 0.55)";
    background = "rgba(52, 211, 153, 0.08)";
    channelLabel = "filesystem";
    iconKind = "filesystem";
    shape = "diamond";
  } else if (event.serverName === "shell") {
    color = "#fbbf24";
    glow = "rgba(251, 191, 36, 0.55)";
    background = "rgba(251, 191, 36, 0.08)";
    channelLabel = "shell";
    iconKind = "shell";
    shape = "circle";
  } else if (event.serverName) {
    color = "#9fc6ff";
    glow = "rgba(159, 198, 255, 0.52)";
    background = "rgba(159, 198, 255, 0.08)";
    channelLabel = event.serverName;
    shape = "circle";
  }

  if (tone === "failed") {
    color = "#ff8c8c";
    glow = "rgba(255, 140, 140, 0.52)";
    background = "rgba(255, 140, 140, 0.1)";
  }

  return {
    channelLabel,
    iconKind,
    shape,
    style: {
      "--event-color": color,
      "--event-glow": glow,
      "--event-bg": background,
    } as CSSProperties,
  };
}

function EventGlyph({
  kind,
}: {
  kind: "task" | "filesystem" | "shell" | "server";
}) {
  if (kind === "task") {
    return (
      <svg viewBox="0 0 16 16" aria-hidden="true">
        <path d="M4 5.5h8M4 8h8M4 10.5h5" />
      </svg>
    );
  }

  if (kind === "filesystem") {
    return (
      <svg viewBox="0 0 16 16" aria-hidden="true">
        <path d="M2.5 5.5h4l1.2 1.5h5.8v4.5H2.5z" />
        <path d="M2.5 5.5V4h4l1.2 1.5" />
      </svg>
    );
  }

  if (kind === "shell") {
    return (
      <svg viewBox="0 0 16 16" aria-hidden="true">
        <path d="m4 5 2.5 2.5L4 10" />
        <path d="M8 10.5h3.5" />
      </svg>
    );
  }

  return (
    <svg viewBox="0 0 16 16" aria-hidden="true">
      <circle cx="4" cy="8" r="1.3" />
      <circle cx="12" cy="5" r="1.3" />
      <circle cx="12" cy="11" r="1.3" />
      <path d="M5.2 7.4 10.7 5.6M5.2 8.6l5.5 1.8" />
    </svg>
  );
}

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

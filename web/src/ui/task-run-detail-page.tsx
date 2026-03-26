import { type CSSProperties, useEffect, useMemo, useRef, useState } from "react";
import { Link, useParams } from "react-router-dom";

import { ApiError, fetchTaskRunEvents, type TaskRunEvent } from "../api/task-runs";
import { formatTimestamp, formatTaskRunId, humanizeIdentifier, humanizePhase } from "./format";
import { SciFiTimeline } from "./sci-fi-timeline";
import { type TaskRunUIStatus } from "./task-run-status";

type LoadState = "loading" | "ready" | "invalid" | "not-found" | "error";

export type TimelineGroup = {
  key: string;
  label: string;
  items: TimelineItem[];
};

export type TimelineExchange = {
  requestId?: string;
  request?: TaskRunEvent;
  response?: TaskRunEvent;
};

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

export function TaskRunDetailPage() {
  const { runID } = useParams();
  const [events, setEvents] = useState<TaskRunEvent[]>([]);
  const [loadState, setLoadState] = useState<LoadState>("loading");
  const [collapsedGroups, setCollapsedGroups] = useState<Record<string, boolean>>({});
  const [selectedItemID, setSelectedItemID] = useState<string>("");
  const [detailsWidth, setDetailsWidth] = useState(() => getDefaultDetailsWidth());
  const [draggingResize, setDraggingResize] = useState(false);
  const previousExpandedWidthRef = useRef(getDefaultDetailsWidth());

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
          <p className="state-card__eyebrow">
            Run Detail
            <span style={{ opacity: 0.35, margin: "0 8px" }}>·</span>
            {formatTaskRunId(runID ?? "")}
            <span style={{ opacity: 0.35, margin: "0 8px" }}>·</span>
            <span className={`status-badge status-badge--${detailStatus}`}>{detailStatus}</span>
          </p>
        </div>
        <div className="task-run-detail__header-actions">
          <Link className="back-link" style={{fontFamily:"inter"}} to="/tasks">
            Back to task runs
          </Link>
        </div>
      </header>

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
  return Math.min(1080, Math.max(320, Math.round(value)));
}

function getDefaultDetailsWidth(): number {
  if (typeof window === "undefined") {
    return 640;
  }

  return clampDetailsWidth(window.innerWidth * 0.4);
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

export function getTimelineAnchorEvent(item: TimelineItem): TaskRunEvent {
  if (item.kind === "task") {
    return item.task;
  }

  return item.exchange.request ?? item.exchange.response ?? item.exchange.request!;
}

export function getTimelineItemTone(item: TimelineItem): "neutral" | "active" | "completed" | "failed" {
  if (item.kind === "task") {
    return getEventTone(item.task);
  }

  return getExchangeTone(item.exchange);
}

export function getTimelineItemTitle(item: TimelineItem): string {
  if (item.kind === "task") {
    return getEventTitle(item.task);
  }

  return getExchangeTitle(item.exchange);
}


export function getTimelineItemSubtitle(item: TimelineItem): string {
  if (item.kind === "task") {
    return getEventSubtitle(item.task);
  }

  return getExchangeSubtitle(item.exchange);
}

export function getTimelineItemStatusLabel(item: TimelineItem): string {
  if (item.kind === "task") {
    return getEventStatusLabel(item.task);
  }

  return getExchangeStatusLabel(item.exchange);
}

export function getTimelineItemAlertLabel(item: TimelineItem): string | undefined {
  const tone = getTimelineItemTone(item);
  if (tone === "failed") {
    return "error";
  }

  return undefined;
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
  const preview = extractPayloadPreview(requestPayload);
  if (preview) {
    return preview;
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

export function getExchangeLatency(exchange: TimelineExchange): number | undefined {
  if (!exchange.request || !exchange.response) {
    return undefined;
  }

  return Math.max(0, exchange.response.createdAtUnixMilli - exchange.request.createdAtUnixMilli);
}

export function formatLatency(durationMs: number): string {
  if (durationMs < 1000) {
    return `${durationMs}ms`;
  }

  return `${(durationMs / 1000).toFixed(durationMs >= 10_000 ? 0 : 1)}s`;
}

export function formatTraceTimestamp(timestamp: number): string {
  const date = new Date(timestamp);
  const hours = String(date.getHours()).padStart(2, "0");
  const minutes = String(date.getMinutes()).padStart(2, "0");
  const seconds = String(date.getSeconds()).padStart(2, "0");
  const milliseconds = String(date.getMilliseconds()).padStart(3, "0");
  return `${hours}:${minutes}:${seconds}.${milliseconds}`;
}

export function getExchangeServerName(exchange: TimelineExchange): string {
  return exchange.request?.serverName ?? exchange.response?.serverName ?? "mcp";
}

export function getExchangeServerLabel(exchange: TimelineExchange): string {
  return getExchangeServerName(exchange);
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
    "projectSummary",
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

  const nestedKeys = [
    "tool_call",
    "arguments",
    "args",
    "params",
    "parameters",
    "request",
    "payload",
    "draftParameters",
    "draft_parameters",
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

function formatPreviewString(value: string): string {
  if (value.includes("/")) {
    return summarizePath(value);
  }

  return value;
}

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

function formatTaskPhaseLine(event: TaskRunEvent): string {
  const from = event.phasePath ? humanizePhase(event.phasePath) : "";
  const to = event.resultingPhasePath ? humanizePhase(event.resultingPhasePath) : "";

  if (from && to && from !== to) {
    return `${from} → ${to}`;
  }

  return to || from;
}

export function getServerAccentColor(serverName: string): string {
  const palette = [
    "#a78bfa",
    "#fbbf24",
    "#34d399",
    "#60a5fa",
    "#fb7185",
    "#22d3ee",
    "#f97316",
    "#4ade80",
  ];

  let hash = 0;
  for (let index = 0; index < serverName.length; index += 1) {
    hash = (hash * 31 + serverName.charCodeAt(index)) >>> 0;
  }

  return palette[hash % palette.length];
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

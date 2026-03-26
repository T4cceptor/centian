import { type CSSProperties, useEffect, useMemo, useState } from "react";
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

type TimelineItem = {
  id: string;
  primary: TaskRunEvent;
  correlatedAction?: TaskRunEvent;
};

export function TaskRunDetailPage() {
  const { runID } = useParams();
  const [events, setEvents] = useState<TaskRunEvent[]>([]);
  const [loadState, setLoadState] = useState<LoadState>("loading");
  const [collapsedGroups, setCollapsedGroups] = useState<Record<string, boolean>>({});
  const [expandedEvents, setExpandedEvents] = useState<Record<string, boolean>>({});

  useEffect(() => {
    if (!runID) {
      setLoadState("invalid");
      setEvents([]);
      return;
    }

    const controller = new AbortController();
    setLoadState("loading");
    setCollapsedGroups({});
    setExpandedEvents({});

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

  const timelineItems = useMemo(() => mergeCentianTaskActionEvents(events), [events]);
  const groupedEvents = useMemo(() => groupEventsByPhase(timelineItems), [timelineItems]);
  const detailStatus = useMemo(() => deriveTaskRunDetailStatus(events), [events]);
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
                  const event = item.primary;
                  const tone = getEventTone(event);
                  const visual = getEventVisuals(event, tone);
                  const title = getEventTitle(event);
                  const statusLabel = getEventStatusLabel(event);
                  const expanded = expandedEvents[event.id] === true;

                  return (
                    <article
                      key={event.id}
                      className={`timeline-event timeline-event--${tone}`}
                      style={visual.style}
                    >
                      <div className="timeline-event__timestamp">
                        <time dateTime={new Date(event.createdAtUnixMilli).toISOString()}>
                          {formatTimestamp(event.createdAtUnixMilli)}
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
                            aria-expanded={expanded}
                            aria-label={`Toggle event details for ${title}`}
                            onClick={() =>
                              setExpandedEvents((current) => ({
                                ...current,
                                [event.id]: !current[event.id],
                              }))
                            }
                          >
                            <div className="timeline-event__meta">
                              <span className="timeline-event__channel">{visual.channelLabel}</span>
                              <span className={`timeline-source-badge timeline-source-badge--${event.source}`}>
                                {event.source}
                              </span>
                              <span className={`timeline-event__status timeline-event__status--${tone}`}>
                                {statusLabel}
                              </span>
                            </div>

                            <div className="timeline-event__body">
                              <div>
                                <h3 className="timeline-event__title" data-testid="timeline-event-title">
                                  {title}
                                </h3>
                                <p className="timeline-event__subtitle">{getEventSubtitle(event)}</p>
                                {item.correlatedAction ? (
                                  <p className="timeline-event__linked-action">
                                    Centian MCP · {getActionLabel(item.correlatedAction)}
                                  </p>
                                ) : null}
                              </div>
                              <span className="timeline-event__details-link">
                                {expanded ? "Hide" : "JSON"}
                              </span>
                            </div>
                          </button>

                          {expanded ? (
                            <div className="timeline-event__details">
                              <div className="timeline-event__details-meta">
                                <span>{formatTimestamp(event.createdAtUnixMilli)}</span>
                                <span>{statusLabel}</span>
                              </div>
                              <p className="timeline-event__payload-label">Task event</p>
                              <pre className="timeline-event__payload">{formatPayload(event.payloadJson)}</pre>
                              {item.correlatedAction ? (
                                <>
                                  <div className="timeline-event__details-meta">
                                    <span>Centian MCP</span>
                                    <span>{getActionLabel(item.correlatedAction)}</span>
                                  </div>
                                  <p className="timeline-event__payload-label">MCP event</p>
                                  <pre className="timeline-event__payload">
                                    {formatPayload(item.correlatedAction.payloadJson)}
                                  </pre>
                                </>
                              ) : null}
                            </div>
                          ) : null}
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
    </div>
  );
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

function mergeCentianTaskActionEvents(events: TaskRunEvent[]): TimelineItem[] {
  const pairedRequestIDs = new Set(
    events
      .filter((event) => event.source === "task" && typeof event.relatedActionRequestId === "string")
      .map((event) => event.relatedActionRequestId as string),
  );
  const centianActionsByRequestID = new Map<string, TaskRunEvent>();
  for (const event of events) {
    if (isCollapsibleCentianAction(event) && event.requestId) {
      centianActionsByRequestID.set(event.requestId, event);
    }
  }

  const items: TimelineItem[] = [];
  for (const event of events) {
    if (event.source === "task") {
      items.push({
        id: event.id,
        primary: event,
        correlatedAction:
          event.relatedActionRequestId != null
            ? centianActionsByRequestID.get(event.relatedActionRequestId)
            : undefined,
      });
      continue;
    }

    if (isCollapsibleCentianAction(event) && event.requestId && pairedRequestIDs.has(event.requestId)) {
      continue;
    }

    items.push({
      id: event.id,
      primary: event,
    });
  }

  return items;
}

function groupEventsByPhase(items: TimelineItem[]): TimelineGroup[] {
  const groups: TimelineGroup[] = [];
  let lastKnownPhase = "";

  for (const item of items) {
    const event = item.primary;
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

function isCollapsibleCentianAction(event: TaskRunEvent): boolean {
  return event.source === "action" && event.serverName === "centian";
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

function getActionLabel(event: TaskRunEvent): string {
  if (event.toolName) {
    return event.toolName;
  }
  if (event.messageType) {
    return humanizeIdentifier(event.messageType);
  }
  return event.requestId ?? "unknown";
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

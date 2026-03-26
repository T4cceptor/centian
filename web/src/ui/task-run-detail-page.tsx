import { useEffect, useMemo, useState } from "react";
import { Link, useParams } from "react-router-dom";

import { ApiError, fetchTaskRunEvents, type TaskRunEvent } from "../api/task-runs";
import { formatTimestamp, humanizeIdentifier, humanizePhase } from "./format";
import { type TaskRunUIStatus } from "./task-run-status";

type LoadState = "loading" | "ready" | "invalid" | "not-found" | "error";

type TimelineGroup = {
  key: string;
  label: string;
  events: TaskRunEvent[];
};

export function TaskRunDetailPage() {
  const { runID } = useParams();
  const [events, setEvents] = useState<TaskRunEvent[]>([]);
  const [loadState, setLoadState] = useState<LoadState>("loading");
  const [expandedPayloads, setExpandedPayloads] = useState<Record<string, boolean>>({});

  useEffect(() => {
    if (!runID) {
      setLoadState("invalid");
      setEvents([]);
      return;
    }

    const controller = new AbortController();
    setLoadState("loading");
    setExpandedPayloads({});

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

  const groupedEvents = useMemo(() => groupEventsByPhase(events), [events]);
  const detailStatus = useMemo(() => deriveTaskRunDetailStatus(events), [events]);
  const startedAt = events[0]?.createdAtUnixMilli;
  const lastSeenAt = events.length > 0 ? events[events.length - 1].createdAtUnixMilli : undefined;

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
        {groupedEvents.map((group) => (
          <section key={group.key} className="timeline-group" aria-label={group.label}>
            <div className="timeline-group__header">
              <span className="timeline-group__marker" />
              <div className="timeline-group__rule" />
              <span className="timeline-group__label">{group.label}</span>
            </div>

            <div className="timeline-group__events">
              {group.events.map((event) => {
                const expanded = expandedPayloads[event.id] === true;
                const tone = getEventTone(event);
                const payloadText = formatPayload(event.payloadJson);

                return (
                  <article key={event.id} className={`timeline-event timeline-event--${tone}`}>
                    <div className="timeline-event__meta">
                      <span className={`timeline-source-badge timeline-source-badge--${event.source}`}>
                        {event.source}
                      </span>
                      <time dateTime={new Date(event.createdAtUnixMilli).toISOString()}>
                        {formatTimestamp(event.createdAtUnixMilli)}
                      </time>
                    </div>

                    <div className="timeline-event__body">
                      <div>
                        <h3 className="timeline-event__title" data-testid="timeline-event-title">
                          {getEventTitle(event)}
                        </h3>
                        <p className="timeline-event__subtitle">{getEventSubtitle(event)}</p>
                      </div>
                      <span className={`timeline-event__status timeline-event__status--${tone}`}>
                        {getEventStatusLabel(event)}
                      </span>
                    </div>

                    <button
                      type="button"
                      className="timeline-event__toggle"
                      onClick={() =>
                        setExpandedPayloads((current) => ({
                          ...current,
                          [event.id]: !current[event.id],
                        }))
                      }
                    >
                      {expanded ? "Hide payload" : "Show payload"}
                    </button>

                    {expanded ? (
                      <pre className="timeline-event__payload">{payloadText}</pre>
                    ) : null}
                  </article>
                );
              })}
            </div>
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

function groupEventsByPhase(events: TaskRunEvent[]): TimelineGroup[] {
  const groups: TimelineGroup[] = [];
  let lastKnownPhase = "";

  for (const event of events) {
    const effectivePhase =
      event.resultingPhasePath || event.phasePath || lastKnownPhase || "unknown";
    lastKnownPhase = effectivePhase;

    const existingGroup = groups.length > 0 ? groups[groups.length - 1] : undefined;
    if (existingGroup == null || existingGroup.key !== effectivePhase) {
      groups.push({
        key: effectivePhase,
        label: humanizePhase(effectivePhase),
        events: [event],
      });
      continue;
    }

    existingGroup.events.push(event);
  }

  return groups;
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
      return "completed";
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

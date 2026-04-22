import { useEffect, useState } from "react";
import { Link, useSearchParams } from "react-router-dom";

import { type EventListFilters, type EventListItem, fetchEvents } from "../api/events";
import { ApiError } from "../api/task-runs";
import { ApiAuthCard } from "./api-auth-card";
import { formatTimestamp, formatTimestampCompact, humanizePhase } from "./format";

type LoadState = "loading" | "ready" | "error" | "unauthorized";
type FilterFormState = {
  gateway: string;
  server: string;
  tool: string;
  direction: string;
  messageType: string;
  success: string;
  requestId: string;
  sessionId: string;
};

const defaultFilterForm: FilterFormState = {
  gateway: "",
  server: "",
  tool: "",
  direction: "",
  messageType: "",
  success: "",
  requestId: "",
  sessionId: "",
};

export function EventListPage() {
  const [searchParams, setSearchParams] = useSearchParams();
  const [items, setItems] = useState<EventListItem[]>([]);
  const [nextCursor, setNextCursor] = useState<string>();
  const [loadState, setLoadState] = useState<LoadState>("loading");
  const [errorMessage, setErrorMessage] = useState("");
  const [authHeaderName, setAuthHeaderName] = useState<string>();
  const [reloadToken, setReloadToken] = useState(0);
  const [expandedEventID, setExpandedEventID] = useState<string>();
  const [filterForm, setFilterForm] = useState<FilterFormState>(defaultFilterForm);

  const filters = eventFiltersFromSearchParams(searchParams);
  const activeFilters = buildActiveFilterChips(filters);

  useEffect(() => {
    setFilterForm({
      gateway: filters.gateway ?? "",
      server: filters.server ?? "",
      tool: filters.tool ?? "",
      direction: filters.direction ?? "",
      messageType: filters.messageType ?? "",
      success: typeof filters.success === "boolean" ? String(filters.success) : "",
      requestId: filters.requestId ?? "",
      sessionId: filters.sessionId ?? "",
    });
  }, [
    filters.direction,
    filters.gateway,
    filters.messageType,
    filters.requestId,
    filters.server,
    filters.sessionId,
    filters.success,
    filters.tool,
  ]);

  useEffect(() => {
    const controller = new AbortController();
    setLoadState("loading");
    setErrorMessage("");

    void fetchEvents(filters, controller.signal)
      .then((page) => {
        setItems(page.items);
        setNextCursor(page.nextCursor);
        setExpandedEventID(undefined);
        setLoadState("ready");
      })
      .catch((error: unknown) => {
        if ((error as Error)?.name === "AbortError") {
          return;
        }
        if (error instanceof ApiError && error.status === 401) {
          setAuthHeaderName(error.authHeaderName);
          setErrorMessage("Enter a Centian API key to read persisted MCP events.");
          setLoadState("unauthorized");
          return;
        }
        setErrorMessage("Unable to load MCP events right now.");
        setLoadState("error");
      });

    return () => controller.abort();
  }, [filters.cursor, filters.direction, filters.gateway, filters.limit, filters.messageType, filters.requestId, filters.server, filters.sessionId, filters.success, filters.tool, reloadToken]);

  useEffect(() => {
    if (loadState !== "ready" || filters.cursor) {
      return;
    }

    let inFlight = false;
    let controller: AbortController | null = null;

    const poll = () => {
      if (inFlight || document.visibilityState === "hidden") {
        return;
      }

      inFlight = true;
      controller = new AbortController();
      void fetchEvents(filters, controller.signal)
        .then((page) => {
          setItems(page.items);
          setNextCursor(page.nextCursor);
        })
        .catch((error: unknown) => {
          if ((error as Error)?.name === "AbortError") {
            return;
          }
          if (error instanceof ApiError && error.status === 401) {
            setAuthHeaderName(error.authHeaderName);
            setErrorMessage("Enter a Centian API key to read persisted MCP events.");
            setLoadState("unauthorized");
          }
        })
        .finally(() => {
          inFlight = false;
          controller = null;
        });
    };

    const timer = window.setInterval(poll, 2000);
    return () => {
      window.clearInterval(timer);
      controller?.abort();
    };
  }, [filters.cursor, filters.direction, filters.gateway, filters.limit, filters.messageType, filters.requestId, filters.server, filters.sessionId, filters.success, filters.tool, loadState]);

  if (loadState === "loading") {
    return (
      <div className="state-card" data-testid="event-list-loading">
        <p className="state-card__eyebrow">Syncing</p>
        <h2>Loading MCP events…</h2>
        <p>Pulling the latest persisted proxy activity from the Centian API.</p>
      </div>
    );
  }

  if (loadState === "error") {
    return (
      <div className="state-card state-card--error" role="alert">
        <p className="state-card__eyebrow">Link Loss</p>
        <h2>Event feed unavailable</h2>
        <p>{errorMessage}</p>
      </div>
    );
  }

  if (loadState === "unauthorized") {
    return (
      <ApiAuthCard
        eyebrow="Access Required"
        title="Event feed is protected"
        body={errorMessage}
        authHeaderName={authHeaderName}
        onSaved={() => setReloadToken((value) => value + 1)}
      />
    );
  }

  return (
    <div className="event-page">
      <div className="event-toolbar">
        <div>
          <p className="state-card__eyebrow">Global Feed</p>
          <h2>Observed MCP events</h2>
          <p className="event-toolbar__subtitle">Newest first across all proxied traffic.</p>
        </div>
        <div className="event-toolbar__actions">
          <p className="task-run-list__count">{items.length} visible events</p>
          <button
            type="button"
            className="task-run-filter-clear"
            onClick={() => {
              const next = new URLSearchParams(searchParams);
              next.delete("cursor");
              setSearchParams(next);
            }}
          >
            Newest
          </button>
          <button
            type="button"
            className="event-pagination__button"
            disabled={!nextCursor}
            onClick={() => {
              if (!nextCursor) {
                return;
              }
              const next = new URLSearchParams(searchParams);
              next.set("cursor", nextCursor);
              setSearchParams(next);
            }}
          >
            Older
          </button>
        </div>
      </div>

      <form
        className="event-filter-panel"
        onSubmit={(event) => {
          event.preventDefault();
          const next = new URLSearchParams(searchParams);
          setOrDelete(next, "gateway", filterForm.gateway);
          setOrDelete(next, "server", filterForm.server);
          setOrDelete(next, "tool", filterForm.tool);
          setOrDelete(next, "direction", filterForm.direction);
          setOrDelete(next, "messageType", filterForm.messageType);
          setOrDelete(next, "success", filterForm.success);
          setOrDelete(next, "requestId", filterForm.requestId);
          setOrDelete(next, "sessionId", filterForm.sessionId);
          next.delete("cursor");
          setSearchParams(next);
        }}
      >
        <label className="event-filter-field">
          <span>Gateway</span>
          <input
            value={filterForm.gateway}
            onChange={(event) => setFilterForm((current) => ({ ...current, gateway: event.target.value }))}
          />
        </label>
        <label className="event-filter-field">
          <span>Server</span>
          <input
            value={filterForm.server}
            onChange={(event) => setFilterForm((current) => ({ ...current, server: event.target.value }))}
          />
        </label>
        <label className="event-filter-field">
          <span>Tool</span>
          <input
            value={filterForm.tool}
            onChange={(event) => setFilterForm((current) => ({ ...current, tool: event.target.value }))}
          />
        </label>
        <label className="event-filter-field">
          <span>Direction</span>
          <select
            value={filterForm.direction}
            onChange={(event) => setFilterForm((current) => ({ ...current, direction: event.target.value }))}
          >
            <option value="">All directions</option>
            <option value="[CLIENT -> SERVER]">Client to server</option>
            <option value="[SERVER -> CLIENT]">Server to client</option>
            <option value="[CENTIAN -> CLIENT]">Centian to client</option>
          </select>
        </label>
        <label className="event-filter-field">
          <span>Message Type</span>
          <select
            value={filterForm.messageType}
            onChange={(event) => setFilterForm((current) => ({ ...current, messageType: event.target.value }))}
          >
            <option value="">All message types</option>
            <option value="request">Request</option>
            <option value="response">Response</option>
            <option value="system">System</option>
          </select>
        </label>
        <label className="event-filter-field">
          <span>Success</span>
          <select
            value={filterForm.success}
            onChange={(event) => setFilterForm((current) => ({ ...current, success: event.target.value }))}
          >
            <option value="">All outcomes</option>
            <option value="true">Success</option>
            <option value="false">Failure</option>
          </select>
        </label>
        <label className="event-filter-field">
          <span>Request ID</span>
          <input
            value={filterForm.requestId}
            onChange={(event) => setFilterForm((current) => ({ ...current, requestId: event.target.value }))}
          />
        </label>
        <label className="event-filter-field">
          <span>Session ID</span>
          <input
            value={filterForm.sessionId}
            onChange={(event) => setFilterForm((current) => ({ ...current, sessionId: event.target.value }))}
          />
        </label>
        <div className="event-filter-actions">
          <button type="submit" className="event-pagination__button">
            Apply
          </button>
          <button
            type="button"
            className="task-run-filter-clear"
            onClick={() => {
              setFilterForm(defaultFilterForm);
              setSearchParams(new URLSearchParams());
            }}
          >
            Clear
          </button>
        </div>
      </form>

      {activeFilters.length > 0 ? (
        <div className="task-run-filter-bar" aria-label="Active event filters">
          {activeFilters.map((filter) => (
            <span key={filter.key} className="task-run-filter-chip">
              {filter.label}
            </span>
          ))}
        </div>
      ) : null}

      {items.length === 0 ? (
        <div className="state-card">
          <p className="state-card__eyebrow">Quiet Channel</p>
          <h2>{activeFilters.length > 0 ? "No matching events" : "No persisted events yet"}</h2>
          <p>
            {activeFilters.length > 0
              ? "No persisted MCP events match the active filters."
              : "Proxy activity will appear here once the event store records MCP traffic."}
          </p>
        </div>
      ) : (
        <div className="event-list" role="list">
          {items.map((item) => {
            const expanded = expandedEventID === item.id;
            const timestampParts = formatTimestampCompact(item.createdAtUnixMilli);
            const statusClass = item.success ? "status-badge status-badge--success" : "status-badge status-badge--failed";
            const identityLabel = item.requestId ?? item.sessionId ?? "—";

            return (
              <article key={item.id} className="event-card" role="listitem">
                <button
                  type="button"
                  className="event-card__summary"
                  onClick={() => setExpandedEventID(expanded ? undefined : item.id)}
                >
                  <span className="event-card__timestamp" title={formatTimestamp(item.createdAtUnixMilli)}>
                    <span>{timestampParts.date}</span>
                    <span>{timestampParts.time}</span>
                  </span>
                  <span className="event-card__tool">{item.toolName ?? item.originalToolName ?? "Unknown tool"}</span>
                  <span className="event-card__server">{[item.gateway, item.serverName].filter(Boolean).join(" / ") || "—"}</span>
                  <span className="event-card__direction">{item.direction ?? "—"}</span>
                  <span className="event-card__message">{item.messageType ?? "—"}</span>
                  <span className="event-card__status">
                    <span className={statusClass}>{item.success ? "success" : "failed"}</span>
                  </span>
                  <span className="event-card__identity" title={identityLabel}>
                    {identityLabel}
                  </span>
                </button>

                {expanded ? (
                  <div className="event-card__detail">
                    <div className="event-card__meta">
                      <div>
                        <p className="event-card__meta-label">Request ID</p>
                        <p>{item.requestId ?? "—"}</p>
                      </div>
                      <div>
                        <p className="event-card__meta-label">Session ID</p>
                        <p>{item.sessionId ?? "—"}</p>
                      </div>
                      <div>
                        <p className="event-card__meta-label">Endpoint</p>
                        <p>{item.endpoint ?? "—"}</p>
                      </div>
                      <div>
                        <p className="event-card__meta-label">Transport</p>
                        <p>{item.transport ?? "—"}</p>
                      </div>
                      <div>
                        <p className="event-card__meta-label">Original Tool</p>
                        <p>{item.originalToolName && item.originalToolName !== item.toolName ? item.originalToolName : "—"}</p>
                      </div>
                      <div>
                        <p className="event-card__meta-label">Invocation Phase</p>
                        <p>{item.invocationPhasePath ? humanizePhase(item.invocationPhasePath) : "—"}</p>
                      </div>
                    </div>

                    {item.taskRunId ? (
                      <p className="event-card__task-link">
                        Related task: <Link to={`/tasks/${item.taskRunId}`}>{item.taskRunId}</Link>
                      </p>
                    ) : null}

                    <div className="event-card__payload">
                      <p className="event-card__meta-label">Payload</p>
                      <textarea
                        className="event-card__payload-text"
                        readOnly
                        spellCheck={false}
                        aria-label={`Payload for ${item.toolName ?? item.id}`}
                        value={formatPayloadJSON(item.payloadJson)}
                      />
                    </div>
                  </div>
                ) : null}
              </article>
            );
          })}
        </div>
      )}
    </div>
  );
}

function eventFiltersFromSearchParams(searchParams: URLSearchParams): EventListFilters {
  const rawSuccess = searchParams.get("success")?.trim();
  const rawLimit = searchParams.get("limit")?.trim();

  return {
    gateway: searchParams.get("gateway")?.trim() || undefined,
    server: searchParams.get("server")?.trim() || undefined,
    tool: searchParams.get("tool")?.trim() || undefined,
    direction: searchParams.get("direction")?.trim() || undefined,
    messageType: searchParams.get("messageType")?.trim() || undefined,
    success: rawSuccess === "true" ? true : rawSuccess === "false" ? false : undefined,
    requestId: searchParams.get("requestId")?.trim() || undefined,
    sessionId: searchParams.get("sessionId")?.trim() || undefined,
    cursor: searchParams.get("cursor")?.trim() || undefined,
    limit: rawLimit && Number(rawLimit) > 0 ? Number(rawLimit) : undefined,
  };
}

function buildActiveFilterChips(filters: EventListFilters): Array<{ key: string; label: string }> {
  const chips: Array<{ key: string; label: string }> = [];
  if (filters.gateway) chips.push({ key: "gateway", label: `Gateway: ${filters.gateway}` });
  if (filters.server) chips.push({ key: "server", label: `Server: ${filters.server}` });
  if (filters.tool) chips.push({ key: "tool", label: `Tool: ${filters.tool}` });
  if (filters.direction) chips.push({ key: "direction", label: `Direction: ${filters.direction}` });
  if (filters.messageType) chips.push({ key: "messageType", label: `Type: ${filters.messageType}` });
  if (typeof filters.success === "boolean") chips.push({ key: "success", label: `Success: ${String(filters.success)}` });
  if (filters.requestId) chips.push({ key: "requestId", label: `Request: ${filters.requestId}` });
  if (filters.sessionId) chips.push({ key: "sessionId", label: `Session: ${filters.sessionId}` });
  if (filters.cursor) chips.push({ key: "cursor", label: "Older page" });
  return chips;
}

function setOrDelete(params: URLSearchParams, key: string, value: string) {
  const normalized = value.trim();
  if (normalized) {
    params.set(key, normalized);
    return;
  }
  params.delete(key);
}

function formatPayloadJSON(value: unknown): string {
  if (value === undefined) {
    return "No payload recorded.";
  }
  try {
    const formatted = JSON.stringify(value, null, 2);
    return formatted ?? "No payload recorded.";
  } catch {
    return String(value);
  }
}

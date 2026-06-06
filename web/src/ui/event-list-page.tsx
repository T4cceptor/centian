import { useEffect, useMemo, useState } from "react";
import { Eye, ListChecks, ScrollText, SearchCheck, ShieldCheck, TriangleAlert, type LucideIcon } from "lucide-react";
import { Link, useParams, useSearchParams } from "react-router-dom";

import { type EventListFilters, type EventListItem, fetchEvents } from "../api/events";
import { ApiError, normalizeProjectSlug, type ProcessorAnnotation } from "../api/task-runs";
import { ApiAuthCard } from "./api-auth-card";
import { formatTimestamp, formatTimestampCompact, humanizeIdentifier, humanizePhase } from "./format";

type LoadState = "loading" | "ready" | "error" | "unauthorized";
type EventSortColumn = "time" | "tool" | "server" | "direction" | "messageType" | "status" | "governance";
type SortDirection = "asc" | "desc";
type EventSortState = {
  column: EventSortColumn;
  direction: SortDirection;
};
type GovernanceSeverity = "high" | "medium" | "low";
type EventGovernanceDescription = {
  id: string;
  action: string;
  category: string;
  eventLabel: string;
  requestKey: string;
  reason: string;
  severity: GovernanceSeverity;
};
type EventGovernanceCategoryCount = {
  category: string;
  count: number;
  severity: GovernanceSeverity;
};
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

const eventColumns: Array<{ key: EventSortColumn; label: string }> = [
  { key: "time", label: "Time" },
  { key: "tool", label: "Tool" },
  { key: "server", label: "Server" },
  { key: "direction", label: "Direction" },
  { key: "messageType", label: "Type" },
  { key: "status", label: "Status" },
  { key: "governance", label: "Gov. Events" },
];

export function EventListPage() {
  const { projectSlug: rawProjectSlug } = useParams();
  const projectSlug = normalizeProjectSlug(rawProjectSlug);
  const [searchParams, setSearchParams] = useSearchParams();
  const [items, setItems] = useState<EventListItem[]>([]);
  const [nextCursor, setNextCursor] = useState<string>();
  const [loadState, setLoadState] = useState<LoadState>("loading");
  const [errorMessage, setErrorMessage] = useState("");
  const [authHeaderName, setAuthHeaderName] = useState<string>();
  const [reloadToken, setReloadToken] = useState(0);
  const [expandedEventID, setExpandedEventID] = useState<string>();
  const [filterForm, setFilterForm] = useState<FilterFormState>(defaultFilterForm);
  const [filtersExpanded, setFiltersExpanded] = useState(false);
  const [isLoadingMore, setIsLoadingMore] = useState(false);
  const [loadMoreToken, setLoadMoreToken] = useState(0);
  const [loadMoreErrorMessage, setLoadMoreErrorMessage] = useState("");
  const [sortState, setSortState] = useState<EventSortState>({ column: "time", direction: "desc" });

  const filters = eventFiltersFromSearchParams(searchParams);
  const baseFilters = eventBaseFilters(filters);
  const activeFilters = buildActiveFilterChips(baseFilters);
  const governanceEvents = useMemo(() => deriveEventGovernanceEvents(items), [items]);
  const sortedItems = [...items].sort((left, right) => compareEvents(left, right, sortState));

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
    setLoadMoreErrorMessage("");
    setIsLoadingMore(false);

    void fetchEvents(projectSlug, baseFilters, controller.signal)
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
  }, [baseFilters.direction, baseFilters.gateway, baseFilters.limit, baseFilters.messageType, baseFilters.requestId, baseFilters.server, baseFilters.sessionId, baseFilters.success, baseFilters.tool, projectSlug, reloadToken]);

  useEffect(() => {
    if (loadState !== "ready" || !filters.cursor) {
      return;
    }

    const controller = new AbortController();
    setIsLoadingMore(true);
    setLoadMoreErrorMessage("");

    void fetchEvents(projectSlug, filters, controller.signal)
      .then((page) => {
        setItems((current) => appendUniqueEvents(current, page.items));
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
          return;
        }
        setLoadMoreErrorMessage("Unable to load more MCP events right now.");
      })
      .finally(() => {
        setIsLoadingMore(false);
      });

    return () => controller.abort();
  }, [baseFilters.direction, baseFilters.gateway, baseFilters.limit, baseFilters.messageType, baseFilters.requestId, baseFilters.server, baseFilters.sessionId, baseFilters.success, baseFilters.tool, filters.cursor, loadMoreToken, projectSlug, loadState]);

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
      void fetchEvents(projectSlug, baseFilters, controller.signal)
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
  }, [baseFilters.direction, baseFilters.gateway, baseFilters.limit, baseFilters.messageType, baseFilters.requestId, baseFilters.server, baseFilters.sessionId, baseFilters.success, baseFilters.tool, filters.cursor, loadState, projectSlug]);

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
      <details
        className="event-filter-disclosure"
        open={filtersExpanded}
        onToggle={(event) => setFiltersExpanded(event.currentTarget.open)}
      >
        <summary>
          <span className="event-filter-disclosure__title">Filters</span>
          {activeFilters.length > 0 ? (
            <span className="event-filter-disclosure__chips" aria-label="Active event filters">
              {activeFilters.map((filter) => (
                <span key={filter.key} className="task-run-filter-chip">
                  {filter.label}
                </span>
              ))}
            </span>
          ) : null}
        </summary>

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
      </details>

      <EventGovernancePanel events={governanceEvents} />

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
        <>
          <div className="event-list" role="list">
            <div className="event-list__header" role="row">
              {eventColumns.map((column) => {
                const isActive = sortState.column === column.key;
                const ariaSort = isActive ? (sortState.direction === "asc" ? "ascending" : "descending") : "none";
                return (
                  <span key={column.key} role="columnheader" aria-sort={ariaSort}>
                    <button
                      type="button"
                      className={isActive ? "benchmark-sort-button benchmark-sort-button--active" : "benchmark-sort-button"}
                      aria-label={`Sort by ${column.label}${isActive ? ` (${sortState.direction})` : ""}`}
                      onClick={() => {
                        setSortState((current) => {
                          if (current.column === column.key) {
                            return {
                              column: column.key,
                              direction: current.direction === "asc" ? "desc" : "asc",
                            };
                          }
                          return {
                            column: column.key,
                            direction: defaultEventSortDirection(column.key),
                          };
                        });
                      }}
                    >
                      <span>{column.label}</span>
                      <span className="benchmark-sort-button__indicator" aria-hidden="true">
                        {isActive ? (sortState.direction === "asc" ? "▲" : "▼") : "↕"}
                      </span>
                    </button>
                  </span>
                );
              })}
            </div>
            {sortedItems.map((item) => {
              const expanded = expandedEventID === item.id;
              const timestampParts = formatTimestampCompact(item.createdAtUnixMilli);
              const compactTimestamp = `${timestampParts.date} ${timestampParts.time}`;
              const statusClass = item.success ? "status-badge status-badge--success" : "status-badge status-badge--failed";

              return (
                <article key={item.id} className="event-card" role="listitem">
                  <button
                    type="button"
                    className="event-card__summary"
                    onClick={() => setExpandedEventID(expanded ? undefined : item.id)}
                  >
                    <span className="event-card__timestamp" title={formatTimestamp(item.createdAtUnixMilli)}>{compactTimestamp}</span>
                    <span className="event-card__tool">{item.toolName ?? item.originalToolName ?? "Unknown tool"}</span>
                    <span className="event-card__server">{[item.gateway, item.serverName].filter(Boolean).join(" / ") || "—"}</span>
                    <span className="event-card__direction">{item.direction ?? "—"}</span>
                    <span className="event-card__message">{item.messageType ?? "—"}</span>
                    <span className="event-card__status">
                      <span className={statusClass}>{item.success ? "success" : "failed"}</span>
                    </span>
                    <EventCardGovernanceCategories item={item} />
                  </button>

                  {expanded ? (
                    <div className="event-card__detail">
                      <div className="event-card__meta">
                        <div className="event-card__meta-item">
                          <p className="event-card__meta-label">Request ID</p>
                          <p className="event-card__meta-value event-card__meta-value--id">{item.requestId ?? "—"}</p>
                        </div>
                        <div className="event-card__meta-item">
                          <p className="event-card__meta-label">Session ID</p>
                          <div className="event-card__meta-action-row">
                            <p className="event-card__meta-value event-card__meta-value--id">{item.sessionId ?? "—"}</p>
                            {item.sessionId ? (
                              <button
                                type="button"
                                className="event-card__filter-button"
                                onClick={(event) => {
                                  event.stopPropagation();
                                  const next = new URLSearchParams(searchParams);
                                  next.set("sessionId", item.sessionId!);
                                  next.delete("cursor");
                                  setSearchParams(next);
                                }}
                              >
                                Filter session
                              </button>
                            ) : null}
                          </div>
                        </div>
                        <div className="event-card__meta-item">
                          <p className="event-card__meta-label">Endpoint</p>
                          <p className="event-card__meta-value">{item.endpoint ?? "—"}</p>
                        </div>
                        <div className="event-card__meta-item">
                          <p className="event-card__meta-label">Transport</p>
                          <p className="event-card__meta-value">{item.transport ?? "—"}</p>
                        </div>
                        <div className="event-card__meta-item">
                          <p className="event-card__meta-label">Original Tool</p>
                          <p className="event-card__meta-value">{item.originalToolName && item.originalToolName !== item.toolName ? item.originalToolName : "—"}</p>
                        </div>
                        <div className="event-card__meta-item">
                          <p className="event-card__meta-label">Invocation Phase</p>
                          <p className="event-card__meta-value">{item.invocationPhasePath ? humanizePhase(item.invocationPhasePath) : "—"}</p>
                        </div>
                      </div>

                      {item.taskRunId ? (
                        <p className="event-card__task-link">
                          Related task: <Link to={`/${projectSlug}/tasks/${item.taskRunId}`}>{item.taskRunId}</Link>
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

          <div className="event-pagination">
            <p className="task-run-list__count">{items.length} visible events</p>
            <button
              type="button"
              className="event-pagination__button"
              disabled={!nextCursor || isLoadingMore}
              onClick={() => {
                if (!nextCursor) {
                  return;
                }
                const next = new URLSearchParams(searchParams);
                next.set("cursor", nextCursor);
                setSearchParams(next);
                setLoadMoreToken((value) => value + 1);
              }}
            >
              {isLoadingMore ? "Loading..." : "Load more..."}
            </button>
          </div>
          {loadMoreErrorMessage ? (
            <p className="event-pagination__error" role="alert">
              {loadMoreErrorMessage}
            </p>
          ) : null}
        </>
      )}
    </div>
  );
}

function EventCardGovernanceCategories({ item }: { item: EventListItem }) {
  const categories = getEventGovernanceCategories(item);
  return (
    <span className="event-card__governance" aria-label={categories.length > 0 ? `Governance events: ${formatGovernanceCategoryList(categories)}` : "Governance events: none"}>
      {categories.length > 0 ? (
        categories.map((category) => (
          <span
            key={category.category.toLowerCase()}
            className={`event-card__governance-category event-governance__category--${getGovernanceCategoryTone(category.category)}`}
            title={formatGovernanceCategory(category.category)}
          >
            <GovernanceCategoryIcon category={category.category} decorative />
            <span>{formatGovernanceCategory(category.category)}</span>
          </span>
        ))
      ) : (
        <span className="event-card__governance-empty">—</span>
      )}
    </span>
  );
}

function EventGovernancePanel({ events }: { events: EventGovernanceDescription[] }) {
  const [expanded, setExpanded] = useState(true);
  const panelID = "event-governance-events";
  const categoryCounts = useMemo(() => deriveGovernanceCategoryCounts(events), [events]);

  return (
    <section className="event-governance" aria-label={`Governance Events: ${events.length}`}>
      <button
        type="button"
        className="event-governance__header"
        aria-expanded={expanded}
        aria-controls={panelID}
        onClick={() => setExpanded((current) => !current)}
      >
        <span className="event-governance__title">
          Governance Events: <span className="event-governance__count">{events.length}</span>
        </span>
        <span className="event-governance__header-meta">
          {categoryCounts.length > 0 ? (
            <span className="event-governance__category-counts" aria-label="Governance events by category">
              {categoryCounts.map((categoryCount) => (
                <span
                  key={categoryCount.category}
                  className={`event-governance__category-count event-governance__category--${getGovernanceCategoryTone(categoryCount.category)}`}
                  title={`${formatGovernanceCategory(categoryCount.category)}: ${categoryCount.count}`}
                >
                  <GovernanceCategoryIcon category={categoryCount.category} decorative />
                  <span>{formatGovernanceCategory(categoryCount.category)}</span>
                  <span className="event-governance__category-count-value">{categoryCount.count}</span>
                </span>
              ))}
            </span>
          ) : null}
          <span className="event-governance__toggle-action">{expanded ? "Hide" : "Show"}</span>
        </span>
      </button>
      {expanded ? (
        <div id={panelID} className="event-governance__body">
          {events.length > 0 ? (
            <ul className="event-governance__list">
              {events.map((event) => (
                <li
                  key={event.id}
                  className={`event-governance__item event-governance__item--${event.severity}`}
                  aria-label={`${formatGovernanceCategory(event.category)}: ${event.action} ${event.eventLabel} - ${event.reason}`}
                >
                  <GovernanceCategoryIcon category={event.category} />
                  <span className={`event-governance__category event-governance__category--${getGovernanceCategoryTone(event.category)}`}>
                    {formatGovernanceCategory(event.category)}
                  </span>
                  <span className="event-governance__separator">:</span>
                  <span className="event-governance__action">{event.action}</span>
                  <code>{event.eventLabel}</code>
                  <span className="event-governance__separator">-</span>
                  <span>{event.reason}</span>
                </li>
              ))}
            </ul>
          ) : (
            <p className="event-governance__empty">No governance events recorded.</p>
          )}
        </div>
      ) : null}
    </section>
  );
}

function deriveEventGovernanceEvents(items: EventListItem[]): EventGovernanceDescription[] {
  const descriptions: EventGovernanceDescription[] = [];
  const seen = new Set<string>();

  for (const item of items) {
    let annotationIndex = 0;
    for (const annotation of getEventAnnotations(item)) {
      if (annotation.type !== "governance_events") {
        continue;
      }
      const action = getProcessorGovernanceAction(annotation.action);
      const category = annotation.category?.trim();
      const severity = normalizeGovernanceSeverity(annotation.severity);
      if (!action || !category || !severity) {
        continue;
      }

      const eventLabel = getEventGovernanceLabel(item);
      const reason = getProcessorGovernanceReason(annotation);
      const requestKey = getEventGovernanceRequestKey(item);
      const key = `${requestKey}:${action}:${category}:${eventLabel}:${reason}:${severity}`;
      if (seen.has(key)) {
        continue;
      }
      seen.add(key);
      descriptions.push({
        id: `${item.id}:${annotationIndex}`,
        action,
        category,
        eventLabel,
        requestKey,
        reason,
        severity,
      });
      annotationIndex += 1;
    }
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

function deriveGovernanceCategoryCounts(events: EventGovernanceDescription[]): EventGovernanceCategoryCount[] {
  const byCategory = new Map<string, EventGovernanceCategoryCount>();
  for (const event of events) {
    const category = event.category.trim();
    if (!category) {
      continue;
    }
    const key = category.toLowerCase();
    const current = byCategory.get(key);
    if (!current) {
      byCategory.set(key, { category, count: 1, severity: event.severity });
      continue;
    }
    current.count += 1;
    if (governanceSeverityRank(event.severity) < governanceSeverityRank(current.severity)) {
      current.severity = event.severity;
    }
  }
  return [...byCategory.values()].sort((left, right) => governanceSeverityRank(left.severity) - governanceSeverityRank(right.severity));
}

function getEventAnnotations(event: EventListItem): ProcessorAnnotation[] {
  const payloadAnnotations = readPayloadAnnotations(event.payloadJson);
  if (!event.annotations || event.annotations.length === 0) {
    return payloadAnnotations;
  }
  return [...event.annotations, ...payloadAnnotations];
}

function getEventGovernanceCategories(item: EventListItem): EventGovernanceCategoryCount[] {
  return deriveGovernanceCategoryCounts(deriveEventGovernanceEvents([item]));
}

function formatGovernanceCategoryList(categories: EventGovernanceCategoryCount[]): string {
  return categories.map((category) => formatGovernanceCategory(category.category)).join(", ");
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

function getEventGovernanceLabel(item: EventListItem): string {
  const toolName = item.toolName?.trim();
  if (toolName?.startsWith("centian.task_")) {
    return getProcessActionLabelForToolName(toolName);
  }
  return item.originalToolName?.trim() || toolName || humanizeIdentifier(item.messageType ?? "event");
}

function getEventGovernanceRequestKey(item: EventListItem): string {
  return item.requestId?.trim() || item.id;
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

function getProcessorGovernanceAction(action?: string): string | undefined {
  const normalized = action?.trim();
  if (!normalized) {
    return undefined;
  }
  return normalized.charAt(0).toUpperCase() + normalized.slice(1).toLowerCase();
}

function getProcessorGovernanceReason(annotation: ProcessorAnnotation): string {
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

function getGovernanceCategoryTone(category: string): string | undefined {
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

function GovernanceCategoryIcon({ category, decorative = false }: { category: string; decorative?: boolean }) {
  const Icon = getGovernanceCategoryIcon(category);
  const tone = getGovernanceCategoryTone(category);
  if (!Icon) {
    return null;
  }
  return (
    <span className={`event-governance__category-icon event-governance__category--${tone}`} title={decorative ? undefined : `Category: ${category}`}>
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

function eventBaseFilters(filters: EventListFilters): EventListFilters {
  const { cursor: _cursor, ...baseFilters } = filters;
  return baseFilters;
}

function appendUniqueEvents(current: EventListItem[], next: EventListItem[]): EventListItem[] {
  const existingIDs = new Set(current.map((item) => item.id));
  const uniqueNext = next.filter((item) => !existingIDs.has(item.id));
  return [...current, ...uniqueNext];
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

function defaultEventSortDirection(column: EventSortColumn): SortDirection {
  switch (column) {
    case "time":
      return "desc";
    default:
      return "asc";
  }
}

function compareEvents(left: EventListItem, right: EventListItem, sortState: EventSortState): number {
  const directionMultiplier = sortState.direction === "asc" ? 1 : -1;
  const primaryComparison = comparePrimaryEventColumn(left, right, sortState.column) * directionMultiplier;
  if (primaryComparison !== 0) {
    return primaryComparison;
  }

  const timeComparison = compareNumbers(right.createdAtUnixMilli, left.createdAtUnixMilli);
  if (timeComparison !== 0) {
    return timeComparison;
  }

  return right.id.localeCompare(left.id);
}

function comparePrimaryEventColumn(left: EventListItem, right: EventListItem, column: EventSortColumn): number {
  switch (column) {
    case "time":
      return compareNumbers(left.createdAtUnixMilli, right.createdAtUnixMilli);
    case "tool":
      return compareStrings(left.toolName ?? left.originalToolName ?? "", right.toolName ?? right.originalToolName ?? "");
    case "server":
      return compareStrings(serverLabel(left), serverLabel(right));
    case "direction":
      return compareStrings(left.direction ?? "", right.direction ?? "");
    case "messageType":
      return compareStrings(left.messageType ?? "", right.messageType ?? "");
    case "status":
      return compareNumbers(eventStatusRank(left), eventStatusRank(right));
    case "governance":
      return compareStrings(eventGovernanceSortValue(left), eventGovernanceSortValue(right));
  }
}

function compareNumbers(left: number, right: number): number {
  return left - right;
}

function compareStrings(left: string, right: string): number {
  return left.localeCompare(right);
}

function serverLabel(item: EventListItem): string {
  return [item.gateway, item.serverName].filter(Boolean).join(" / ");
}

function eventStatusRank(item: EventListItem): number {
  return item.success ? 1 : 0;
}

function eventGovernanceSortValue(item: EventListItem): string {
  return getEventGovernanceCategories(item)
    .map((category) => category.category.toLowerCase())
    .join(",");
}

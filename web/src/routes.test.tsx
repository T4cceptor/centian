import { act, render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import { afterEach, describe, expect, it, vi } from "vitest";

import { AppRoutes } from "./routes";
import { clearStoredApiAuth } from "./api/api-auth";
import { ErrorBoundary } from "./ui/error-boundary";
import { getTimelineAnchorEvent, type TimelineItem } from "./ui/task-run-detail-page";

const originalFetch = globalThis.fetch;

function createFetchResponse(body: unknown, status: number = 200): Response {
  return {
    ok: status >= 200 && status < 300,
    status,
    headers: new Headers(),
    json: async () => body,
  } as Response;
}

function createTaskRunDetailResponse(
  runId: string,
  benchmarkLinks: unknown[] = [],
  extra: Record<string, unknown> = {},
): Response {
  return createFetchResponse({ runId, benchmarkLinks, ...extra });
}

function deferred<T>() {
  let resolve!: (value: T | PromiseLike<T>) => void;
  let reject!: (reason?: unknown) => void;
  const promise = new Promise<T>((res, rej) => {
    resolve = res;
    reject = rej;
  });
  return { promise, resolve, reject };
}

function renderApp(initialEntries: string[] = ["/tasks"], projects: unknown[] = [defaultProjectSummary()]) {
  installProjectFetchMock(projects);
  return render(
    <MemoryRouter
      initialEntries={initialEntries}
      future={{ v7_startTransition: true, v7_relativeSplatPath: true }}
    >
      <AppRoutes />
    </MemoryRouter>,
  );
}

function defaultProjectSummary() {
  return {
    slug: "default",
    name: "default",
    isDefault: true,
    uiEnabled: true,
    eventStorageEnabled: true,
    taskVerificationEnabled: true,
  };
}

function researchProjectSummary() {
  return {
    slug: "research",
    name: "Research",
    isDefault: false,
    uiEnabled: true,
    eventStorageEnabled: true,
    taskVerificationEnabled: true,
  };
}

function installProjectFetchMock(projects: unknown[]) {
  const configuredFetch = globalThis.fetch;
  globalThis.fetch = vi.fn((input: RequestInfo | URL, init?: RequestInit) => {
    if (String(input) === "/api/projects") {
      return Promise.resolve(
        createFetchResponse({
          projects,
        }),
      );
    }
    return configuredFetch(input, init);
  }) as typeof fetch;
}

function nonProjectFetchCalls() {
  return vi.mocked(globalThis.fetch).mock.calls.filter(([input]) => String(input) !== "/api/projects");
}

afterEach(() => {
  vi.useRealTimers();
  vi.restoreAllMocks();
  globalThis.fetch = originalFetch;
  clearStoredApiAuth();
});

function ThrowingRoute(): null {
  throw new Error("render crash");
}

describe("error boundary", () => {
  it("renders a fallback when a child route crashes during render", () => {
    vi.spyOn(console, "error").mockImplementation(() => {});

    render(
      <ErrorBoundary>
        <ThrowingRoute />
      </ErrorBoundary>,
    );

    expect(screen.getByText("Task run UI crashed")).toBeInTheDocument();
    expect(screen.getByText("Reload the page or return to the task list.")).toBeInTheDocument();
  });
});

describe("task run list", () => {
  it("shows a loading state before the api resolves", async () => {
    const pending = deferred<Response>();
    globalThis.fetch = vi.fn(() => pending.promise) as typeof fetch;

    renderApp();

    expect(screen.getByTestId("task-run-loading")).toBeInTheDocument();
    pending.resolve(createFetchResponse([]));
    await waitFor(() => {
      expect(screen.getByText("No task runs yet")).toBeInTheDocument();
    });
  });

  it("renders returned task runs", async () => {
    globalThis.fetch = vi.fn(() =>
      Promise.resolve(
        createFetchResponse([
          {
            runId: "tr_1742947200123_0000000001",
            templateId: "python_tdd_demo",
            startedAt: 1742947200123,
            status: "succeeded",
            currentPhase: "execution.implement_fix",
            taskEventCount: 2,
            actionEventCount: 3,
            eventCount: 5,
          },
        ]),
      ),
    ) as typeof fetch;

    renderApp();

    expect(await screen.findByText("Python TDD Demo")).toBeInTheDocument();
    expect(screen.getByText("Execution / Implement Fix")).toBeInTheDocument();
    expect(screen.getByText("5")).toBeInTheDocument();
  });

  it("shows an empty state when no task runs are returned", async () => {
    globalThis.fetch = vi.fn(() => Promise.resolve(createFetchResponse([]))) as typeof fetch;

    renderApp();

    expect(await screen.findByText("No task runs yet")).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: "Run IT Ops Demo" })).not.toBeInTheDocument();
  });

  it("polls the task run list and refreshes new runs in place", async () => {
    globalThis.fetch = vi
      .fn()
      .mockResolvedValueOnce(createFetchResponse([]))
      .mockResolvedValueOnce(
        createFetchResponse([
          {
            runId: "tr_1742947200123_0000000001",
            templateId: "python_tdd_demo",
            startedAt: 1742947200123,
            status: "active",
            currentPhase: "planning.review",
            taskEventCount: 2,
            actionEventCount: 3,
            eventCount: 5,
          },
        ]),
      ) as typeof fetch;

    renderApp();

    expect(await screen.findByText("No task runs yet")).toBeInTheDocument();

    await waitFor(() => {
      expect(nonProjectFetchCalls()).toHaveLength(2);
    }, { timeout: 3500 });

    expect(await screen.findByText("Python TDD Demo")).toBeInTheDocument();
  }, 8000);

  it("shows an error state when the api request fails", async () => {
    globalThis.fetch = vi.fn(() => Promise.reject(new Error("network down"))) as typeof fetch;

    renderApp();

    expect(await screen.findByText("Task run feed unavailable")).toBeInTheDocument();
  });

  it("prompts for an api key on unauthorized list access and retries with the stored header", async () => {
    const user = userEvent.setup();
    globalThis.fetch = vi
      .fn()
      .mockResolvedValueOnce({
        ok: false,
        status: 401,
        headers: new Headers({ "X-Centian-Auth-Header": "X-Centian-Auth" }),
      } as Response)
      .mockResolvedValueOnce(
        createFetchResponse([
          {
            runId: "tr_1742947200123_0000000001",
            templateId: "python_tdd_demo",
            startedAt: 1742947200123,
            status: "succeeded",
            currentPhase: "planning.review",
            taskEventCount: 2,
            actionEventCount: 3,
            eventCount: 5,
          },
        ]),
      ) as typeof fetch;

    renderApp();

    expect(await screen.findByText("Task run feed is protected")).toBeInTheDocument();
    await user.type(screen.getByLabelText("API key"), "plain-key");
    await user.click(screen.getByRole("button", { name: "Save and retry" }));

    expect(await screen.findByText("Python TDD Demo")).toBeInTheDocument();
    expect(nonProjectFetchCalls()[1]).toEqual([
      "/api/default/task-runs",
      expect.objectContaining({
        headers: expect.any(Headers),
      }),
    ]);
    const headers = (nonProjectFetchCalls()[1]?.[1] as RequestInit)?.headers as Headers;
    expect(headers.get("X-Centian-Auth")).toBe("plain-key");
  });

  it("clears a stored api key from the unauthorized prompt", async () => {
    const user = userEvent.setup();
    globalThis.fetch = vi.fn(() =>
      Promise.resolve({
        ok: false,
        status: 401,
        headers: new Headers({ "X-Centian-Auth-Header": "X-Centian-Auth" }),
      } as Response),
    ) as typeof fetch;

    renderApp();

    await user.type(await screen.findByLabelText("API key"), "plain-key");
    await user.click(screen.getByRole("button", { name: "Clear stored key" }));

    expect((screen.getByLabelText("API key") as HTMLInputElement).value).toBe("");
  });

  it("maps active success failed and timed out status badges", async () => {
    globalThis.fetch = vi.fn(() =>
      Promise.resolve(
        createFetchResponse([
          {
            runId: "tr_1742947200123_0000000001",
            templateId: "active_task",
            startedAt: Date.now() - 3000,
            status: "succeeded",
            currentPhase: "execution.run",
            taskEventCount: 1,
            actionEventCount: 0,
            eventCount: 1,
          },
          {
            runId: "tr_1742947200124_0000000002",
            templateId: "completed_task",
            startedAt: 1742947200123,
            endedAt: 1742947210123,
            status: "succeeded",
            currentPhase: "execution.done",
            taskEventCount: 2,
            actionEventCount: 1,
            eventCount: 3,
          },
          {
            runId: "tr_1742947200125_0000000003",
            templateId: "failed_task",
            startedAt: 1742947200123,
            endedAt: 1742947210123,
            status: "failed",
            currentPhase: "execution.run",
            taskEventCount: 2,
            actionEventCount: 1,
            eventCount: 3,
          },
          {
            runId: "tr_1742947200126_0000000004",
            templateId: "timed_out_task",
            startedAt: 1742947200123,
            status: "timed_out",
            currentPhase: "execution.run",
            taskEventCount: 2,
            actionEventCount: 1,
            eventCount: 3,
          },
        ]),
      ),
    ) as typeof fetch;

    renderApp();

    expect(await screen.findByText("active")).toBeInTheDocument();
    expect(screen.getByText("success")).toBeInTheDocument();
    expect(screen.getByText("failed")).toBeInTheDocument();
    expect(screen.getByText("timed_out")).toBeInTheDocument();
  });

  it("uses endedAt for finished run duration instead of current time", async () => {
    globalThis.fetch = vi.fn(() =>
      Promise.resolve(
        createFetchResponse([
          {
            runId: "tr_1742947200124_0000000002",
            templateId: "completed_task",
            startedAt: 1_000,
            endedAt: 11_000,
            status: "succeeded",
            currentPhase: "execution.done",
            taskEventCount: 2,
            actionEventCount: 1,
            eventCount: 3,
          },
        ]),
      ),
    ) as typeof fetch;

    renderApp();

    expect(await screen.findByText("Completed Task")).toBeInTheDocument();
    expect(screen.getByText("10s")).toBeInTheDocument();
    expect(screen.getByText("success")).toBeInTheDocument();
    expect(screen.getByText("Completed")).toBeInTheDocument();
  });

  it("navigates to the detail route when a row is clicked", async () => {
    const user = userEvent.setup();
    globalThis.fetch = vi
      .fn()
      .mockResolvedValueOnce(
        createFetchResponse([
          {
            runId: "tr_1742947200123_0000000001",
            templateId: "python_tdd_demo",
            startedAt: 1742947200123,
            status: "succeeded",
            currentPhase: "planning.review",
            taskEventCount: 2,
            actionEventCount: 3,
            eventCount: 5,
          },
        ]),
      )
      .mockResolvedValueOnce(
        createFetchResponse([
          {
            source: "task",
            id: "te_1742947200123_0000000001",
            createdAtUnixMilli: 1742947200123,
            eventType: "task_registered",
            outcome: "succeeded",
            phasePath: "planning.review",
            resultingPhasePath: "planning.review",
            nodeKind: "planning",
            payloadJson: { status: "active" },
          },
        ]),
      )
      .mockResolvedValueOnce(createTaskRunDetailResponse("tr_1742947200123_0000000001")) as typeof fetch;

    renderApp();

    await user.click(await screen.findByRole("link", { name: /tr_1742947200123_0000000001/i }));

    expect(await screen.findByText("Agent Task Details")).toBeInTheDocument();
    expect(screen.queryByLabelText(/Task progress:/)).not.toBeInTheDocument();
    expect(screen.queryByRole("heading", { name: "Task Runs" })).not.toBeInTheDocument();
    expect(screen.queryByText("Centian Monitor")).not.toBeInTheDocument();
  });

  it("applies the benchmark suite filter from the url", async () => {
    globalThis.fetch = vi.fn(() =>
      Promise.resolve(
        createFetchResponse([
          {
            runId: "tr_1742947200123_0000000001",
            templateId: "python_tdd_demo",
            startedAt: 1742947200123,
            status: "succeeded",
            currentPhase: "planning.review",
            taskEventCount: 2,
            actionEventCount: 3,
            eventCount: 5,
          },
        ]),
      ),
    ) as typeof fetch;

    renderApp(["/tasks?benchmarkSuite=simple_tdd_v1"]);

    expect(await screen.findByText("Python TDD Demo")).toBeInTheDocument();
    expect(screen.getByText("Benchmark suite: simple_tdd_v1")).toBeInTheDocument();
    expect(globalThis.fetch).toHaveBeenCalledWith(
      "/api/default/task-runs?benchmarkSuite=simple_tdd_v1",
      expect.objectContaining({ headers: expect.any(Headers) }),
    );
    expect(screen.getByRole("link", { name: /tr_1742947200123_0000000001/i })).toHaveAttribute(
      "href",
      "/default/tasks/tr_1742947200123_0000000001?benchmarkSuite=simple_tdd_v1",
    );
  });

  it("loads task runs from a project-scoped route", async () => {
    globalThis.fetch = vi.fn((input: RequestInfo | URL) => {
      const url = String(input);
      if (url === "/api/research/task-runs") {
        return Promise.resolve(
          createFetchResponse([
            {
              runId: "tr_1742947200123_0000000001",
              templateId: "research_demo",
              startedAt: 1742947200123,
              status: "succeeded",
              currentPhase: "execution.review",
              taskEventCount: 1,
              actionEventCount: 0,
              eventCount: 1,
            },
          ]),
        );
      }
      return Promise.reject(new Error(`unexpected url ${url}`));
    }) as typeof fetch;

    renderApp(["/research/tasks"], [defaultProjectSummary(), researchProjectSummary()]);

    expect(await screen.findByText("Research Demo")).toBeInTheDocument();
    expect(globalThis.fetch).toHaveBeenCalledWith("/api/research/task-runs", expect.anything());
    expect(screen.queryByRole("link", { name: "Benchmarks" })).not.toBeInTheDocument();
  });

  it("switches project-scoped task routes from the project selector", async () => {
    const user = userEvent.setup();
    globalThis.fetch = vi.fn((input: RequestInfo | URL) => {
      const url = String(input);
      if (url === "/api/default/task-runs") {
        return Promise.resolve(createFetchResponse([]));
      }
      if (url === "/api/research/task-runs") {
        return Promise.resolve(
          createFetchResponse([
            {
              runId: "tr_1742947200123_0000000002",
              templateId: "research_demo",
              startedAt: 1742947200123,
              status: "succeeded",
              currentPhase: "execution.review",
              taskEventCount: 1,
              actionEventCount: 0,
              eventCount: 1,
            },
          ]),
        );
      }
      return Promise.reject(new Error(`unexpected url ${url}`));
    }) as typeof fetch;

    renderApp(["/default/tasks"], [defaultProjectSummary(), researchProjectSummary()]);

    expect(await screen.findByText("No task runs yet")).toBeInTheDocument();
    await user.selectOptions(screen.getByLabelText("Project"), "research");
    expect(await screen.findByText("Research Demo")).toBeInTheDocument();
    expect(globalThis.fetch).toHaveBeenCalledWith("/api/research/task-runs", expect.anything());
  });
});

describe("event list", () => {
  it("defaults the ui root to the default project events route", async () => {
    globalThis.fetch = vi.fn(() =>
      Promise.resolve(
        createFetchResponse({
          items: [],
        }),
      ),
    ) as typeof fetch;

    renderApp(["/"]);

    expect(await screen.findByText("No persisted events yet")).toBeInTheDocument();
    expect(globalThis.fetch).toHaveBeenCalledWith("/api/default/events", expect.anything());
    expect(screen.getByRole("heading", { name: "Events" })).toBeInTheDocument();
  });

  it("renders the events route and primary nav", async () => {
    globalThis.fetch = vi.fn(() =>
      Promise.resolve(
        createFetchResponse({
          items: [
            {
              id: "ae_1742947200123_0000000001",
              createdAtUnixMilli: 1742947200123,
              toolName: "shell__exec",
              gateway: "gw",
              serverName: "server-a",
              direction: "[CLIENT -> SERVER]",
              messageType: "request",
              success: true,
              isError: false,
              requestId: "req-1",
            },
          ],
          nextCursor: "cursor-2",
        }),
      ),
    ) as typeof fetch;

    renderApp(["/events"]);

    expect(await screen.findByText("shell__exec")).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "Events" })).toBeInTheDocument();
    expect(within(screen.getByRole("navigation", { name: "Primary" })).getAllByRole("link").map((link) => link.textContent)).toEqual([
      "Events",
      "Tasks",
      "Benchmarks",
    ]);
    expect(screen.queryByText("Observed MCP events")).not.toBeInTheDocument();
    expect(screen.queryByText("Global Feed")).not.toBeInTheDocument();
    expect(screen.queryByText("Newest first across all proxied traffic.")).not.toBeInTheDocument();
    expect(screen.getByRole("button", { name: /Sort by Time/i })).toBeInTheDocument();
    expect(screen.getByRole("button", { name: /Sort by Type/i })).toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /Sort by Direction/i })).not.toBeInTheDocument();
    expect(screen.queryByRole("button", { name: /Sort by Status/i })).not.toBeInTheDocument();
    expect(screen.getByLabelText("Request")).toHaveClass("event-card__message-type--success");
    expect(screen.getByRole("button", { name: /Sort by Gov. Events/i })).toBeInTheDocument();
  });

  it("reflects URL filters in the request and active chips", async () => {
    globalThis.fetch = vi.fn(() => Promise.resolve(createFetchResponse({ items: [] }))) as typeof fetch;

    renderApp(["/events?gateway=gw&server=server-a&success=false&withGovernanceEvent=true"]);

    expect(await screen.findByText("No matching events")).toBeInTheDocument();
    expect(globalThis.fetch).toHaveBeenCalledWith("/api/default/events?gateway=gw&server=server-a&success=false&withGovernanceEvent=true", expect.anything());
    expect(screen.getByText("Gateway: gw")).toBeInTheDocument();
    expect(screen.getByText("Server: server-a")).toBeInTheDocument();
    expect(screen.getByText("Success: false")).toBeInTheDocument();
    expect(screen.getAllByText("With governance event").length).toBeGreaterThan(0);
  });

  it("loads events from a project-scoped route", async () => {
    globalThis.fetch = vi.fn((input: RequestInfo | URL) => {
      const url = String(input);
      if (url === "/api/research/events") {
        return Promise.resolve(
          createFetchResponse({
            items: [
              {
                id: "ae_1742947200123_0000000001",
                createdAtUnixMilli: 1742947200123,
                toolName: "research__tool",
                success: true,
                isError: false,
              },
            ],
          }),
        );
      }
      return Promise.reject(new Error(`unexpected url ${url}`));
    }) as typeof fetch;

    renderApp(["/research/events"], [defaultProjectSummary(), researchProjectSummary()]);

    expect(await screen.findByText("research__tool")).toBeInTheDocument();
    expect(globalThis.fetch).toHaveBeenCalledWith("/api/research/events", expect.anything());
    expect(screen.queryByRole("link", { name: "Benchmarks" })).not.toBeInTheDocument();
  });

  it("renders governance annotations in the events view", async () => {
    globalThis.fetch = vi.fn(() =>
      Promise.resolve(
        createFetchResponse({
          items: [
            {
              id: "ae_1742947200123_0000000001",
              createdAtUnixMilli: 1742947200123,
              toolName: "shell__exec",
              success: true,
              isError: false,
              annotations: [
                {
                  type: "governance_events",
                  action: "redacted",
                  category: "security",
                  severity: "high",
                  message: "masked secret output",
                },
              ],
            },
            {
              id: "ae_1742947201123_0000000002",
              createdAtUnixMilli: 1742947201123,
              toolName: "centian.task_complete_step",
              requestId: "complete_step_2_early",
              success: false,
              isError: true,
              annotations: [
                {
                  type: "governance_events",
                  action: "stopped",
                  category: "quality",
                  severity: "medium",
                  message: "Ticket was not updated.",
                },
              ],
            },
          ],
        }),
      ),
    ) as typeof fetch;

    renderApp(["/events"]);

    const governanceSection = await screen.findByLabelText("Governance Events: 2");
    expect(within(governanceSection).getAllByText("Security")).toHaveLength(2);
    expect(within(governanceSection).getByTitle("Security: 1")).toBeInTheDocument();
    expect(within(governanceSection).getAllByText("Quality")).toHaveLength(2);
    expect(within(governanceSection).getByTitle("Quality: 1")).toBeInTheDocument();
    expect(within(governanceSection).getByText("Redacted")).toBeInTheDocument();
    expect(within(governanceSection).getByText("shell__exec")).toBeInTheDocument();
    expect(within(governanceSection).getByText("masked secret output")).toBeInTheDocument();
    expect(screen.getByLabelText("Governance events: Security")).toBeInTheDocument();
    expect(screen.getByLabelText("Governance events: Quality")).toBeInTheDocument();
  });

  it("collapses repeated governance annotations for the same request id", async () => {
    const user = userEvent.setup();
    const duplicateGovernanceAnnotation = {
      type: "governance_events",
      action: "redacted",
      category: "security",
      severity: "high",
      message: "masked secret output",
    };
    globalThis.fetch = vi.fn(() =>
      Promise.resolve(
        createFetchResponse({
          items: [
            {
              id: "ae_1742947200123_0000000001",
              createdAtUnixMilli: 1742947200123,
              toolName: "shell__exec",
              requestId: "req-1",
              messageType: "request",
              success: true,
              isError: false,
              annotations: [duplicateGovernanceAnnotation],
            },
            {
              id: "ae_1742947201123_0000000002",
              createdAtUnixMilli: 1742947201123,
              toolName: "shell__exec",
              requestId: "req-1",
              messageType: "response",
              success: true,
              isError: false,
              annotations: [duplicateGovernanceAnnotation],
            },
          ],
        }),
      ),
    ) as typeof fetch;

    renderApp(["/events"]);

    const governanceSection = await screen.findByLabelText("Governance Events: 1");
    expect(within(governanceSection).getAllByRole("listitem")).toHaveLength(1);
    expect(within(governanceSection).getByTitle("Security: 1")).toBeInTheDocument();

    await user.click(within(governanceSection).getByRole("button", { name: /Governance Events: 1/i }));
    expect(within(governanceSection).queryByText("masked secret output")).not.toBeInTheDocument();
    expect(within(governanceSection).getByTitle("Security: 1")).toBeInTheDocument();
  });

  it("counts repeated governance annotations separately across request ids", async () => {
    const duplicateGovernanceAnnotation = {
      type: "governance_events",
      action: "redacted",
      category: "security",
      severity: "high",
      message: "masked secret output",
    };
    globalThis.fetch = vi.fn(() =>
      Promise.resolve(
        createFetchResponse({
          items: [
            {
              id: "ae_1742947200123_0000000001",
              createdAtUnixMilli: 1742947200123,
              toolName: "shell__exec",
              requestId: "req-1",
              success: true,
              isError: false,
              annotations: [duplicateGovernanceAnnotation],
            },
            {
              id: "ae_1742947201123_0000000002",
              createdAtUnixMilli: 1742947201123,
              toolName: "shell__exec",
              requestId: "req-2",
              success: true,
              isError: false,
              annotations: [duplicateGovernanceAnnotation],
            },
          ],
        }),
      ),
    ) as typeof fetch;

    renderApp(["/events"]);

    const governanceSection = await screen.findByLabelText("Governance Events: 2");
    expect(within(governanceSection).getAllByRole("listitem")).toHaveLength(2);
    expect(within(governanceSection).getByTitle("Security: 2")).toBeInTheDocument();
    expect(within(governanceSection).getAllByText("masked secret output")).toHaveLength(2);
  });

  it("appends older events using the cursor URL token", async () => {
    const user = userEvent.setup();
    globalThis.fetch = vi.fn((input: RequestInfo | URL) => {
      const url = String(input);
      if (url === "/api/default/events") {
        return Promise.resolve(
          createFetchResponse({
            items: [
              {
                id: "ae_1742947200123_0000000003",
                createdAtUnixMilli: 1742947200123,
                toolName: "tool-new",
                success: true,
                isError: false,
              },
            ],
            nextCursor: "cursor-2",
          }),
        );
      }
      if (url === "/api/default/events?cursor=cursor-2") {
        return Promise.resolve(
          createFetchResponse({
            items: [
              {
                id: "ae_1742947200123_0000000001",
                createdAtUnixMilli: 1742947100123,
                toolName: "tool-old",
                success: true,
                isError: false,
              },
            ],
          }),
        );
      }
      return Promise.reject(new Error(`unexpected url ${url}`));
    }) as typeof fetch;

    renderApp(["/events"]);

    expect(await screen.findByText("tool-new")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Load more..." }));
    expect(await screen.findByText("tool-old")).toBeInTheDocument();
    expect(screen.getByText("tool-new")).toBeInTheDocument();
    expect(screen.queryByText("Older page")).not.toBeInTheDocument();
    expect(globalThis.fetch).toHaveBeenCalledWith("/api/default/events?cursor=cursor-2", expect.anything());
  });

  it("sorts the visible event page by column like the benchmark overview", async () => {
    const user = userEvent.setup();
    globalThis.fetch = vi.fn(() =>
      Promise.resolve(
        createFetchResponse({
          items: [
            {
              id: "ae_1742947200123_0000000002",
              createdAtUnixMilli: 1742947200123,
              toolName: "zeta__tool",
              gateway: "gw",
              serverName: "server-z",
              success: true,
              isError: false,
              requestId: "req-z",
            },
            {
              id: "ae_1742947200123_0000000001",
              createdAtUnixMilli: 1742947200122,
              toolName: "alpha__tool",
              gateway: "gw",
              serverName: "server-a",
              success: false,
              isError: true,
              requestId: "req-a",
            },
          ],
        }),
      ),
    ) as typeof fetch;

    const { container } = renderApp(["/events"]);

    await screen.findByText("zeta__tool");
    let toolNodes = container.querySelectorAll(".event-card__tool");
    expect(toolNodes[0]?.textContent).toBe("zeta__tool");
    expect(toolNodes[1]?.textContent).toBe("alpha__tool");

    await user.click(screen.getByRole("button", { name: "Sort by Tool" }));
    toolNodes = container.querySelectorAll(".event-card__tool");
    expect(toolNodes[0]?.textContent).toBe("alpha__tool");
    expect(toolNodes[1]?.textContent).toBe("zeta__tool");

    await user.click(screen.getByRole("button", { name: "Sort by Tool (asc)" }));
    toolNodes = container.querySelectorAll(".event-card__tool");
    expect(toolNodes[0]?.textContent).toBe("zeta__tool");
    expect(toolNodes[1]?.textContent).toBe("alpha__tool");
  });

  it("expands rows to show payload, related task links, and session quick filters", async () => {
    const user = userEvent.setup();
    globalThis.fetch = vi.fn((input: RequestInfo | URL) => {
      const url = String(input);
      if (url === "/api/default/events") {
        return Promise.resolve(
          createFetchResponse({
            items: [
              {
                id: "ae_1742947200123_0000000001",
                createdAtUnixMilli: 1742947200123,
                toolName: "shell__exec",
                originalToolName: "shell__exec",
                success: true,
                isError: false,
                requestId: "req-1",
                sessionId: "sid-1",
                endpoint: "/mcp/gw",
                transport: "http",
                taskRunId: "tr_1742947200123_0000000001",
                invocationPhasePath: "planning.review",
                payloadJson: {
                  arguments: {
                    command: "pwd",
                  },
                },
              },
            ],
          }),
        );
      }
      if (url === "/api/default/events?sessionId=sid-1") {
        return Promise.resolve(
          createFetchResponse({
            items: [
              {
                id: "ae_1742947200123_0000000001",
                createdAtUnixMilli: 1742947200123,
                toolName: "shell__exec",
                success: true,
                isError: false,
                sessionId: "sid-1",
              },
            ],
          }),
        );
      }
      return Promise.reject(new Error(`unexpected url ${url}`));
    }) as typeof fetch;

    renderApp(["/events"]);

    await user.click(await screen.findByRole("button", { name: /shell__exec/i }));

    expect(screen.getByText(/Related task:/)).toBeInTheDocument();
    expect(screen.getByRole("link", { name: "tr_1742947200123_0000000001" })).toHaveAttribute(
      "href",
      "/default/tasks/tr_1742947200123_0000000001",
    );
    expect(screen.getByDisplayValue(/"command": "pwd"/)).toBeInTheDocument();
    expect(screen.getByText("Planning / Review")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: "Filter session" }));
    expect(await screen.findByText("Session: sid-1")).toBeInTheDocument();
  });
});

describe("task run detail", () => {
  it("shows a loading state before the event api resolves", async () => {
    const pending = deferred<Response>();
    globalThis.fetch = vi.fn((input) => {
      const url = String(input);
      if (url.endsWith("/events")) {
        return pending.promise;
      }
      return Promise.resolve(createTaskRunDetailResponse("tr_1742947200123_0000000001"));
    }) as typeof fetch;

    renderApp(["/tasks/tr_1742947200123_0000000001"]);

    expect(screen.getByTestId("task-run-detail-loading")).toBeInTheDocument();
    pending.resolve(createFetchResponse([]));
    await waitFor(() => {
      expect(screen.getByText("Agent Task Details")).toBeInTheDocument();
    });
  });

  it("shows task template metadata and description in the detail header", async () => {
    globalThis.fetch = vi
      .fn()
      .mockResolvedValueOnce(
        createFetchResponse([
          {
            source: "task",
            id: "te_1742947198123_0000000000",
            createdAtUnixMilli: 1742947198123,
            eventType: "onboarding_completed",
            outcome: "succeeded",
            phasePath: "onboarding",
            resultingPhasePath: "planning",
            nodeKind: "onboarding",
            payloadJson: { taskSummary: "Resolve the payment service incident." },
          },
          {
            source: "task",
            id: "te_1742947199123_0000000001",
            createdAtUnixMilli: 1742947199123,
            eventType: "planning_completed",
            outcome: "succeeded",
            phasePath: "planning",
            resultingPhasePath: "execution.root_cause_analysis",
            nodeKind: "planning",
            payloadJson: { planSummary: "Diagnose logs, document the root cause, then remediate." },
          },
          {
            source: "task",
            id: "te_1742947200123_0000000002",
            createdAtUnixMilli: 1742947200123,
            eventType: "step_started",
            outcome: "succeeded",
            phasePath: "execution.root_cause_analysis",
            resultingPhasePath: "execution.root_cause_analysis",
            nodeKind: "execution",
          },
          {
            source: "task",
            id: "te_1742947201123_0000000003",
            createdAtUnixMilli: 1742947201123,
            eventType: "step_completed",
            outcome: "succeeded",
            phasePath: "execution.root_cause_analysis",
            resultingPhasePath: "execution.root_cause_documentation",
            nodeKind: "execution",
          },
          {
            source: "task",
            id: "te_1742947202123_0000000004",
            createdAtUnixMilli: 1742947202123,
            eventType: "step_started",
            outcome: "succeeded",
            phasePath: "execution.root_cause_documentation",
            resultingPhasePath: "execution.root_cause_documentation",
            nodeKind: "execution",
          },
        ]),
      )
      .mockResolvedValueOnce(
        createTaskRunDetailResponse("tr_1742947200123_0000000001", [], {
          snapshot: {
            runId: "tr_1742947200123_0000000001",
            templateId: "it_incident_resolution",
            templateName: "IT Incident Resolution",
            status: "active",
            phase: "execution.root_cause_documentation",
            workflowReady: true,
            taskDescription:
              "Resolve the incident from the registered task description and keep the operator-oriented context concise.",
            selectedTemplate: {
              version: "0.1",
              task: {
                id: "it_incident_resolution",
                name: "IT Incident Resolution",
                description: "Resolve an unhealthy payment service incident.",
              },
              compiledWorkflow: {
                workflowSteps: [
                  { id: "root_cause_analysis", path: "execution.root_cause_analysis" },
                  { id: "root_cause_documentation", path: "execution.root_cause_documentation" },
                  { id: "resolution", path: "execution.resolution" },
                ],
              },
            },
          },
        }),
      ) as typeof fetch;

    renderApp(["/tasks/tr_1742947200123_0000000001"]);

    expect(await screen.findByText("Agent Task Details")).toBeInTheDocument();
    expect(
      screen.getByText("Resolve the incident from the registered task description and keep the operator-oriented context concise."),
    ).toBeInTheDocument();
    expect(screen.queryByText("Resolve the payment service incident.")).not.toBeInTheDocument();
    expect(screen.queryByText("Diagnose logs, document the root cause, then remediate.")).not.toBeInTheDocument();
    expect(screen.getByLabelText("Template: IT Incident Resolution")).toBeInTheDocument();
    expect(screen.queryByLabelText(/Task progress:/)).not.toBeInTheDocument();
  });

  it("polls the detail timeline once per second while the run is active", async () => {
    globalThis.fetch = vi
      .fn()
      .mockResolvedValueOnce(
        createFetchResponse([
          {
            source: "task",
            id: "te_1742947200123_0000000001",
            createdAtUnixMilli: 1742947200123,
            eventType: "task_registered",
            outcome: "succeeded",
            phasePath: "onboarding",
            resultingPhasePath: "onboarding",
            payloadJson: {
              status: "active",
              taskDescription: "Resolve the incident by following the governed workflow.",
            },
          },
        ]),
      )
      .mockResolvedValueOnce(createTaskRunDetailResponse("tr_1742947200123_0000000001"))
      .mockResolvedValueOnce(
        createFetchResponse([
          {
            source: "task",
            id: "te_1742947200123_0000000001",
            createdAtUnixMilli: 1742947200123,
            eventType: "task_registered",
            outcome: "succeeded",
            phasePath: "onboarding",
            resultingPhasePath: "onboarding",
            payloadJson: {
              status: "active",
              taskDescription: "Resolve the incident by following the governed workflow.",
            },
          },
          {
            source: "task",
            id: "te_1742947201123_0000000002",
            createdAtUnixMilli: 1742947201123,
            eventType: "step_completed",
            outcome: "succeeded",
            phasePath: "scaffolding.step_1",
            resultingPhasePath: "scaffolding.step_1",
            payloadJson: { status: "active", step: 1 },
          },
        ]),
      ) as typeof fetch;

    renderApp(["/tasks/tr_1742947200123_0000000001"]);

    expect(await screen.findByText("Agent Task Details")).toBeInTheDocument();

    await waitFor(() => {
      expect(nonProjectFetchCalls()).toHaveLength(3);
    }, { timeout: 2500 });
    await waitFor(() => {
      const titles = screen.getAllByTestId("timeline-event-title").map((element) => element.textContent ?? "");
      expect(titles.some((title) => title.includes("Step Completed"))).toBe(true);
    });
  }, 8000);

  it("keeps the detail duration counter ticking while the run is active", async () => {
    vi.useFakeTimers();
    vi.setSystemTime(10_000);

    globalThis.fetch = vi.fn(() =>
      Promise.resolve(
        createFetchResponse([
          {
            source: "task",
            id: "te_1742947200123_0000000001",
            createdAtUnixMilli: 5_000,
            eventType: "task_registered",
            outcome: "succeeded",
            phasePath: "onboarding",
            resultingPhasePath: "onboarding",
            payloadJson: {
              status: "active",
              taskDescription: "Resolve the incident by following the governed workflow.",
            },
          },
          {
            source: "task",
            id: "te_1742947200124_0000000002",
            createdAtUnixMilli: 8_000,
            eventType: "step_started",
            outcome: "succeeded",
            phasePath: "planning",
            resultingPhasePath: "execution.step_1",
            payloadJson: { status: "active" },
          },
        ]),
      ),
    ) as typeof fetch;

    renderApp(["/tasks/tr_1742947200123_0000000001"]);

    await act(async () => {});
    expect(screen.getByText("Agent Task Details")).toBeInTheDocument();
    expect(screen.getByText("Resolve the incident by following the governed workflow.")).toBeInTheDocument();
    expect(screen.getByText("5s")).toHaveClass("task-run-detail__duration-value--active");

    await act(async () => {
      await vi.advanceTimersByTimeAsync(2_000);
    });

    expect(screen.getByText("7s")).toHaveClass("task-run-detail__duration-value--active");
  });

  it("shows timeout once an active run has not received events for 15 minutes", async () => {
    vi.useFakeTimers();
    vi.setSystemTime(15 * 60 * 1000 + 1);

    globalThis.fetch = vi.fn(() =>
      Promise.resolve(
        createFetchResponse([
          {
            source: "task",
            id: "te_1742947200123_0000000001",
            createdAtUnixMilli: 0,
            eventType: "task_registered",
            outcome: "succeeded",
            phasePath: "onboarding",
            resultingPhasePath: "onboarding",
            payloadJson: { status: "active" },
          },
        ]),
      ),
    ) as typeof fetch;

    renderApp(["/tasks/tr_1742947200123_0000000001"]);

    await act(async () => {});
    expect(screen.getByText("Agent Task Details")).toBeInTheDocument();
    expect(screen.getByText("timeout")).toHaveClass("task-run-detail__duration-value--failed");
  });

  it("shows timeout once the run times out", async () => {
    vi.useFakeTimers();
    vi.setSystemTime(10_000);

    globalThis.fetch = vi.fn(() =>
      Promise.resolve(
        createFetchResponse([
          {
            source: "task",
            id: "te_1742947200123_0000000001",
            createdAtUnixMilli: 5_000,
            eventType: "task_registered",
            outcome: "succeeded",
            phasePath: "onboarding",
            resultingPhasePath: "onboarding",
            payloadJson: { status: "active" },
          },
          {
            source: "task",
            id: "te_1742947200124_0000000002",
            createdAtUnixMilli: 8_000,
            eventType: "task_timed_out",
            outcome: "failed",
            phasePath: "execution.step_1",
            resultingPhasePath: "execution.step_1",
            payloadJson: { status: "timed_out" },
          },
        ]),
      ),
    ) as typeof fetch;

    renderApp(["/tasks/tr_1742947200123_0000000001"]);

    await act(async () => {});
    expect(screen.getByText("Agent Task Details")).toBeInTheDocument();
    expect(screen.getByText("timeout")).toHaveClass("task-run-detail__duration-value--failed");

    await act(async () => {
      await vi.advanceTimersByTimeAsync(2_000);
    });

    expect(screen.getByText("timeout")).toHaveClass("task-run-detail__duration-value--failed");
  });

  it("stops polling the detail timeline after the run reaches a terminal state", async () => {
    globalThis.fetch = vi
      .fn()
      .mockResolvedValueOnce(
        createFetchResponse([
          {
            source: "task",
            id: "te_1742947200123_0000000001",
            createdAtUnixMilli: 1742947200123,
            eventType: "task_completed",
            outcome: "succeeded",
            phasePath: "execution.implement_solution",
            resultingPhasePath: "execution.implement_solution",
            payloadJson: { status: "completed" },
          },
        ]),
      )
      .mockResolvedValueOnce(createTaskRunDetailResponse("tr_1742947200123_0000000001")) as typeof fetch;

    renderApp(["/tasks/tr_1742947200123_0000000001"]);

    expect(await screen.findByText("Agent Task Details")).toBeInTheDocument();

    await new Promise((resolve) => window.setTimeout(resolve, 2200));

    expect(nonProjectFetchCalls()).toHaveLength(2);
  }, 8000);

  it("keeps polling after a failed step event while the task run remains active", async () => {
    globalThis.fetch = vi
      .fn()
      .mockResolvedValueOnce(
        createFetchResponse([
          {
            source: "task",
            id: "te_1742947200123_0000000001",
            createdAtUnixMilli: 1742947200123,
            eventType: "step_completed",
            outcome: "failed",
            phasePath: "execution.establish_failing_baseline",
            resultingPhasePath: "execution.establish_failing_baseline",
            payloadJson: { status: "active", step: 3, passed: false },
          },
        ]),
      )
      .mockResolvedValueOnce(
        createFetchResponse([
          {
            source: "task",
            id: "te_1742947200123_0000000001",
            createdAtUnixMilli: 1742947200123,
            eventType: "step_completed",
            outcome: "failed",
            phasePath: "execution.establish_failing_baseline",
            resultingPhasePath: "execution.establish_failing_baseline",
            payloadJson: { status: "active", step: 3, passed: false },
          },
          {
            source: "task",
            id: "te_1742947201123_0000000002",
            createdAtUnixMilli: 1742947201123,
            eventType: "step_started",
            outcome: "succeeded",
            phasePath: "execution.establish_failing_baseline",
            resultingPhasePath: "execution.implement_solution",
            payloadJson: { status: "active", step: 4 },
          },
        ]),
      ) as typeof fetch;

    renderApp(["/tasks/tr_1742947200123_0000000001"]);

    expect(await screen.findByText("Agent Task Details")).toBeInTheDocument();
    expect(screen.queryByLabelText(/Task progress:/)).not.toBeInTheDocument();
    expect(screen.getByLabelText("Governance Events: 0")).toBeInTheDocument();
    expect(screen.getByText("No governance events recorded.")).toBeInTheDocument();

    await waitFor(() => {
      expect(nonProjectFetchCalls()).toHaveLength(2);
    }, { timeout: 2500 });
  }, 8000);

  it("prompts for an api key on unauthorized detail access", async () => {
    globalThis.fetch = vi.fn(() =>
      Promise.resolve({
        ok: false,
        status: 401,
        headers: new Headers({ "X-Centian-Auth-Header": "X-Centian-Auth" }),
      } as Response),
    ) as typeof fetch;

    renderApp(["/tasks/tr_1742947200123_0000000001"]);

    expect(await screen.findByText("Task timeline is protected")).toBeInTheDocument();
    expect(screen.getByText("Back to task runs")).toBeInTheDocument();
  });

  it("renders a benchmark run link for benchmark-linked task runs", async () => {
    globalThis.fetch = vi.fn((input) => {
      const url = String(input);
      if (url.endsWith("/events")) {
        return Promise.resolve(
          createFetchResponse([
            {
              source: "task",
              id: "te_1742947200123_0000000001",
              createdAtUnixMilli: 1742947200123,
              eventType: "task_registered",
              outcome: "succeeded",
              phasePath: "onboarding",
              resultingPhasePath: "onboarding",
              payloadJson: { status: "active" },
            },
          ]),
        );
      }
      return Promise.resolve(
        createTaskRunDetailResponse("tr_1742947200123_0000000001", [
          {
            benchmarkRunId: "ba_score",
            sessionId: "ba_session",
            suiteId: "simple_tdd_v1",
            suiteName: "Simple TDD Benchmark Suite v1",
            caseId: "assertion_failure_red",
            caseName: "Assertion-failure red baseline",
            agent: "codex",
            selectedModel: "gpt-5.4-mini",
            templateVariant: "current",
            attempt: 1,
            startedAtUnixMilli: 1742947200123,
          },
        ]),
      );
    }) as typeof fetch;

    renderApp(["/tasks/tr_1742947200123_0000000001"]);

    expect(await screen.findByRole("link", { name: "See Benchmark" })).toHaveAttribute(
      "href",
      "/benchmarks/simple_tdd_v1/runs/ba_score",
    );
    expect(screen.queryByText("Benchmark Context")).not.toBeInTheDocument();
  });

  it("renders grouped mixed timeline exchanges and shows selected details in the side inspector", async () => {
    const user = userEvent.setup();
    globalThis.fetch = vi.fn(() =>
      Promise.resolve(
        createFetchResponse([
          {
            source: "task",
            id: "te_1742947200123_0000000001",
            createdAtUnixMilli: 1742947200123,
            eventType: "task_registered",
            outcome: "succeeded",
            phasePath: "onboarding",
            resultingPhasePath: "onboarding",
            nodeKind: "onboarding",
            resultingNodeKind: "onboarding",
            payloadJson: { status: "active", templateId: "python_tdd_demo" },
          },
          {
            source: "action",
            id: "ae_1742947200124_0000000002",
            createdAtUnixMilli: 1742947201123,
            requestId: "req_1742947201123_0000000002",
            direction: "request",
            toolName: "execute_command",
            serverName: "shell",
            gateway: "taskverification",
            transport: "http",
            payloadJson: { command: "pwd" },
          },
          {
            source: "action",
            id: "ae_1742947200124_0000000008",
            createdAtUnixMilli: 1742947201423,
            requestId: "req_1742947201123_0000000002",
            direction: "response",
            toolName: "execute_command",
            success: true,
            serverName: "shell",
            gateway: "taskverification",
            transport: "http",
            payloadJson: { output: "/workspace/project" },
          },
          {
            source: "task",
            id: "te_1742947200125_0000000003",
            createdAtUnixMilli: 1742947202123,
            eventType: "step_completed",
            outcome: "succeeded",
            relatedActionRequestId: "req_1742947202123_0000000005",
            phasePath: "onboarding",
            resultingPhasePath: "execution.implement_fix",
            nodeKind: "planning",
            resultingNodeKind: "execution",
            payloadJson: { status: "active", step: 1 },
          },
          {
            source: "action",
            id: "ae_1742947200126_0000000005",
            createdAtUnixMilli: 1742947202500,
            requestId: "req_1742947202123_0000000005",
            direction: "request",
            toolName: "centian.task_complete_step",
            serverName: "centian",
            gateway: "taskverification",
            transport: "http",
            payloadJson: { step: 1 },
          },
          {
            source: "action",
            id: "ae_1742947200126_0000000006",
            createdAtUnixMilli: 1742947202600,
            requestId: "req_1742947202123_0000000005",
            direction: "response",
            toolName: "centian.task_complete_step",
            success: true,
            serverName: "centian",
            gateway: "taskverification",
            transport: "http",
            payloadJson: { completed: true },
          },
          {
            source: "action",
            id: "ae_1742947200127_0000000004",
            createdAtUnixMilli: 1742947203123,
            direction: "response",
            toolName: "edit_file",
            success: true,
            serverName: "filesystem",
            transport: "http",
            payloadJson: { nested: true },
            annotations: [
              {
                type: "governance_events",
                processor: "prompt_injection_guard",
                action: "redacted",
                category: "security",
                severity: "high",
                message: "Prompt injection content was redacted.",
              },
            ],
          },
        ]),
      ),
    ) as typeof fetch;

    renderApp(["/tasks/tr_1742947200123_0000000001"]);

    expect(await screen.findByText("Agent Task Details")).toBeInTheDocument();
    expect(screen.getByLabelText("Onboarding")).toBeInTheDocument();
    expect(screen.getByText("300ms")).toBeInTheDocument();

    const titles = screen.getAllByTestId("timeline-event-title").map((element) => element.textContent);
    expect(titles).toEqual([
      "Task Registered",
      "shell - execute_command",
      "Step Completed · Onboarding",
      "filesystem - edit_file",
    ]);
    expect(screen.getByLabelText("Governance Events: 1")).toBeInTheDocument();
    expect(screen.getByLabelText("Security: Redacted filesystem - edit_file - Prompt injection content was redacted.")).toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: /Governance Events: 1/i }));
    expect(screen.queryByLabelText("Security: Redacted filesystem - edit_file - Prompt injection content was redacted.")).not.toBeInTheDocument();
    expect(screen.queryByLabelText(/Run Quality:/)).not.toBeInTheDocument();
    expect(screen.getByLabelText("Agent Actions (Process/MCP): Process 2, MCP 2")).toBeInTheDocument();
    expect(screen.queryByText("Show details")).not.toBeInTheDocument();
    expect(screen.queryByLabelText(/Errors \(Process\/MCP\):/)).not.toBeInTheDocument();
    expect(screen.queryByText("Request · execute_command")).not.toBeInTheDocument();
    expect(screen.queryByText("centian.task_complete_step")).not.toBeInTheDocument();

    expect(screen.queryByText("Inspector")).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: /show event details for execute_command/i }));
    expect(screen.getAllByText("Request")).toHaveLength(2);
    expect(screen.getAllByText("Response")).toHaveLength(2);
    expect(screen.getByText(/"command": "pwd"/)).toBeInTheDocument();
    expect(screen.getByText(/"output": "\/workspace\/project"/)).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: /show event details for step completed/i }));
    expect(screen.getAllByText("Centian Request")).toHaveLength(2);
    expect(screen.getAllByText("Centian Response")).toHaveLength(2);

    expect(screen.queryByText(/"nested": true/)).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: /show event details for edit_file/i }));
    expect(screen.getByText(/"nested": true/)).toBeInTheDocument();

    const onboardingSection = screen.getByLabelText("Onboarding");
    expect(within(onboardingSection).getByText("Step Completed · Onboarding")).toBeInTheDocument();
    expect(within(onboardingSection).getByText(/− Saved/)).toBeInTheDocument();
  });

  it("collapses request-only centian task tool events into matching task lifecycle rows", async () => {
    globalThis.fetch = vi.fn(() =>
      Promise.resolve(
        createFetchResponse([
          {
            source: "task",
            id: "te_1742947200123_0000000001",
            createdAtUnixMilli: 1742947200123,
            eventType: "onboarding_completed",
            outcome: "succeeded",
            phasePath: "onboarding",
            resultingPhasePath: "planning",
            nodeKind: "onboarding",
            resultingNodeKind: "planning",
            payloadJson: { status: "active" },
          },
          {
            source: "action",
            id: "ae_1742947200124_0000000002",
            createdAtUnixMilli: 1742947201123,
            requestId: "req_1742947201123_0000000002",
            direction: "[CLIENT -> SERVER]",
            messageType: "request",
            toolName: "centian.task_complete_planning",
            serverName: "centian",
            gateway: "taskverification",
            transport: "http",
            payloadJson: { planning: { parameters: { testCommand: "python" } } },
          },
          {
            source: "task",
            id: "te_1742947200125_0000000003",
            createdAtUnixMilli: 1742947201123,
            eventType: "planning_completed",
            outcome: "succeeded",
            phasePath: "planning",
            resultingPhasePath: "scaffolding.setup_test_file",
            nodeKind: "planning",
            resultingNodeKind: "scaffolding",
            payloadJson: { status: "active", step: 1 },
          },
        ]),
      ),
    ) as typeof fetch;

    renderApp(["/tasks/tr_1742947200123_0000000001"]);

    expect(await screen.findByText("Agent Task Details")).toBeInTheDocument();

    const titles = screen.getAllByTestId("timeline-event-title").map((element) => element.textContent);
    expect(titles).toEqual([
      "Onboarding Completed",
      "Planning Completed",
    ]);
    expect(screen.queryByText("centian.task_complete_planning")).not.toBeInTheDocument();
  });

  it("renders request-only exchanges as pending and orphan responses from response state", async () => {
    const user = userEvent.setup();
    globalThis.fetch = vi.fn(() =>
      Promise.resolve(
        createFetchResponse([
          {
            source: "task",
            id: "te_1742947200123_0000000001",
            createdAtUnixMilli: 1742947200123,
            eventType: "task_registered",
            outcome: "succeeded",
            phasePath: "onboarding",
            resultingPhasePath: "onboarding",
            payloadJson: { status: "active" },
          },
          {
            source: "action",
            id: "ae_1742947200124_0000000002",
            createdAtUnixMilli: 1742947201123,
            requestId: "req_1742947201123_0000000002",
            direction: "request",
            toolName: "execute_command",
            serverName: "shell",
            payloadJson: { command: "pwd" },
          },
          {
            source: "action",
            id: "ae_1742947200125_0000000003",
            createdAtUnixMilli: 1742947202123,
            requestId: "req_1742947202123_0000000003",
            direction: "response",
            toolName: "edit_file",
            success: false,
            isError: true,
            serverName: "filesystem",
            payloadJson: { message: "write failed" },
          },
        ]),
      ),
    ) as typeof fetch;

    renderApp(["/tasks/tr_1742947200123_0000000001"]);

    expect(await screen.findByText("Agent Task Details")).toBeInTheDocument();
    const titles = screen.getAllByTestId("timeline-event-title").map((element) => element.textContent);
    expect(titles).toEqual(["Task Registered", "shell - execute_command", "filesystem - edit_file"]);
    expect(screen.getByLabelText("Governance Events: 0")).toBeInTheDocument();
    expect(screen.queryByLabelText(/Run Quality:/)).not.toBeInTheDocument();
    expect(screen.getByLabelText("Agent Actions (Process/MCP): Process 1, MCP 2")).toBeInTheDocument();
    expect(screen.queryByText("Show details")).not.toBeInTheDocument();
    expect(screen.queryByLabelText(/Errors \(Process\/MCP\):/)).not.toBeInTheDocument();
    expect(screen.getByText("error")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: /show event details for execute_command/i }));
    expect(screen.getAllByText("pending")).toHaveLength(2);

    await user.click(screen.getByRole("button", { name: /show event details for edit_file/i }));
    expect(screen.getAllByText("failed")).toHaveLength(2);
  });

  it("orders governance events by processor action, blocked, then stopped", async () => {
    globalThis.fetch = vi.fn(() =>
      Promise.resolve(
        createFetchResponse([
          {
            source: "task",
            id: "te_1742947200123_0000000001",
            createdAtUnixMilli: 1742947200123,
            eventType: "task_registered",
            outcome: "succeeded",
            phasePath: "onboarding",
            resultingPhasePath: "onboarding",
            payloadJson: { status: "active" },
          },
          {
            source: "task",
            id: "te_1742947201123_0000000002",
            createdAtUnixMilli: 1742947201123,
            eventType: "step_completed",
            outcome: "failed",
            phasePath: "execution.root_cause_documentation",
            resultingPhasePath: "execution.root_cause_documentation",
            payloadJson: { status: "active", step: 2, passed: false },
            annotations: [
              {
                type: "governance_events",
                action: "stopped",
                category: "quality",
                severity: "low",
                message: "checks failed",
              },
            ],
          },
          {
            source: "action",
            id: "ae_1742947201623_0000000004",
            createdAtUnixMilli: 1742947201623,
            direction: "response",
            toolName: "query",
            success: true,
            serverName: "postgres",
            payloadJson: { rows: [] },
            annotations: [
              {
                type: "governance_events",
                processor: "pii_redactor",
                action: "redacted",
                category: "quality",
                severity: "medium",
                message: "redacted PII content from result",
              },
            ],
          },
          {
            source: "action",
            id: "ae_1742947200125_0000000003",
            createdAtUnixMilli: 1742947202123,
            requestId: "req_1742947202123_0000000003",
            direction: "response",
            toolName: "restart_service",
            originalToolName: "kubectl___restart_service",
            success: false,
            isError: true,
            serverName: "centian",
            payloadJson: {
              result: {
                structuredContent: {
                  blocked: true,
                  reason: "tool_not_allowed",
                  phase: "onboarding",
                },
              },
            },
            annotations: [
              {
                type: "governance_events",
                action: "blocked",
                category: "policy",
                severity: "high",
                message: "tool not allowed in phase \"Onboarding\"",
              },
            ],
          },
        ]),
      ),
    ) as typeof fetch;

    renderApp(["/tasks/tr_1742947200123_0000000001"]);

    expect(await screen.findByText("Agent Task Details")).toBeInTheDocument();
    expect(screen.getByLabelText("Governance Events: 3")).toBeInTheDocument();
    expect(screen.getByLabelText("Policy: Blocked kubectl - restart_service - tool not allowed in phase \"Onboarding\"")).toBeInTheDocument();
    const descriptions = within(screen.getByLabelText("Governance Events: 3"))
      .getAllByRole("listitem")
      .map((item) => item.textContent);
    expect(descriptions).toEqual([
      "Quality:Redactedpostgres - query-redacted PII content from result",
      "Policy:Blockedkubectl - restart_service-tool not allowed in phase \"Onboarding\"",
      "Quality:StoppedRoot Cause Documentation-checks failed",
    ]);
  });

  it("renders governance annotations from centian task event payloads", async () => {
    globalThis.fetch = vi.fn(() =>
      Promise.resolve(
        createFetchResponse([
          {
            source: "task",
            id: "te_1742947201123_0000000002",
            createdAtUnixMilli: 1742947201123,
            eventType: "step_started",
            outcome: "failed",
            phasePath: "execution.root_cause_documentation",
            resultingPhasePath: "execution.root_cause_documentation",
            payloadJson: {
              status: "active",
              step: 2,
              annotations: [
                {
                  type: "governance_events",
                  action: "stopped",
                  category: "quality",
                  severity: "medium",
                  message: "expected root-cause documentation before resolution",
                },
              ],
            },
          },
        ]),
      ),
    ) as typeof fetch;

    renderApp(["/tasks/tr_1742947200123_0000000001"]);

    expect(await screen.findByText("Agent Task Details")).toBeInTheDocument();
    expect(screen.getByLabelText("Governance Events: 1")).toBeInTheDocument();

    const governanceSection = screen.getByLabelText("Governance Events: 1");
    expect(
      within(governanceSection).getByLabelText(
        "Quality: Stopped Root Cause Documentation - expected root-cause documentation before resolution",
      ),
    ).toBeInTheDocument();
    expect(within(governanceSection).getByTitle("Category: quality")).toBeInTheDocument();
    expect(within(governanceSection).getByText("Quality")).toBeInTheDocument();
  });

  it("renders governance category icons and filters invalid annotations", async () => {
    globalThis.fetch = vi.fn(() =>
      Promise.resolve(
        createFetchResponse([
          {
            source: "action",
            id: "ae_1742947200125_0000000003",
            createdAtUnixMilli: 1742947202123,
            direction: "response",
            toolName: "inspect",
            success: true,
            serverName: "centian",
            payloadJson: { ok: true },
            annotations: [
              { type: "governance_events", action: "redacted", category: "security", severity: "low", message: "security event" },
              { type: "governance_events", action: "redacted", category: "observability", severity: "medium", message: "observability event" },
              { type: "governance_events", action: "redacted", category: "policy", severity: "high", message: "policy event" },
              { type: "governance_events", action: "redacted", category: "quality", severity: "high", message: "quality high event" },
              { type: "governance_events", action: "redacted", category: "quality", severity: "low", message: "quality event" },
              { type: "governance_events", action: "redacted", category: "unknown", severity: "low", message: "unknown category event" },
              { type: "governance_events", action: "redacted", severity: "low", message: "missing category event" },
              { type: "governance_events", action: "redacted", category: "security", severity: "info", message: "invalid severity event" },
              { action: "redacted", category: "security", severity: "high", message: "missing type event" },
            ],
          },
        ]),
      ),
    ) as typeof fetch;

    renderApp(["/tasks/tr_1742947200123_0000000001"]);

    expect(await screen.findByText("Agent Task Details")).toBeInTheDocument();
    expect(screen.getByLabelText("Governance Events: 6")).toBeInTheDocument();

    const governanceSection = screen.getByLabelText("Governance Events: 6");
    expect(within(governanceSection).getAllByRole("listitem")).toHaveLength(6);
    expect(within(governanceSection).getByTitle("Category: security")).toBeInTheDocument();
    expect(within(governanceSection).getByTitle("Category: observability")).toBeInTheDocument();
    expect(within(governanceSection).getByTitle("Category: policy")).toBeInTheDocument();
    expect(within(governanceSection).getAllByTitle("Category: quality")).toHaveLength(2);
    expect(within(governanceSection).queryByTitle("Category: unknown")).not.toBeInTheDocument();
    expect(within(governanceSection).queryByText("missing category event")).not.toBeInTheDocument();
    expect(within(governanceSection).queryByText("invalid severity event")).not.toBeInTheDocument();
    expect(within(governanceSection).queryByText("missing type event")).not.toBeInTheDocument();
    expect(within(governanceSection).getByText("Policy")).toHaveClass("task-run-detail__governance-category--policy");
  });

  it("pairs persisted MCP direction values into one exchange card", async () => {
    const user = userEvent.setup();
    globalThis.fetch = vi.fn(() =>
      Promise.resolve(
        createFetchResponse([
          {
            source: "task",
            id: "te_1742947200123_0000000001",
            createdAtUnixMilli: 1742947200123,
            eventType: "task_registered",
            outcome: "succeeded",
            phasePath: "onboarding",
            resultingPhasePath: "onboarding",
            payloadJson: { status: "active" },
          },
          {
            source: "action",
            id: "ae_1742947200124_0000000002",
            createdAtUnixMilli: 1742947201123,
            requestId: "req_1742947201123_0000000002",
            direction: "[CLIENT -> SERVER]",
            messageType: "request",
            toolName: "create_directory",
            serverName: "filesystem",
            payloadJson: {
              request_id: "req_1742947201123_0000000002",
              direction: "[CLIENT -> SERVER]",
              message_type: "request",
              path: "/workspace/project/tmp",
            },
          },
          {
            source: "action",
            id: "ae_1742947201124_0000000003",
            createdAtUnixMilli: 1742947201423,
            requestId: "req_1742947201123_0000000002",
            direction: "[SERVER -> CLIENT]",
            messageType: "response",
            toolName: "create_directory",
            serverName: "filesystem",
            success: true,
            payloadJson: {
              request_id: "req_1742947201123_0000000002",
              direction: "[SERVER -> CLIENT]",
              message_type: "response",
              success: true,
            },
          },
        ]),
      ),
    ) as typeof fetch;

    renderApp(["/tasks/tr_1742947200123_0000000001"]);

    expect(await screen.findByText("Agent Task Details")).toBeInTheDocument();
    const titles = screen.getAllByTestId("timeline-event-title").map((element) => element.textContent);
    expect(titles).toEqual(["Task Registered", "filesystem - create_directory"]);
    expect(screen.getByText("300ms")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: /show event details for create_directory/i }));
    expect(screen.getAllByText("Request")).toHaveLength(2);
    expect(screen.getAllByText("Response")).toHaveLength(2);
  });

  it("skips malformed action events that cannot form a renderable exchange", async () => {
    globalThis.fetch = vi.fn(() =>
      Promise.resolve(
        createFetchResponse([
          {
            source: "task",
            id: "te_1742947200123_0000000001",
            createdAtUnixMilli: 1742947200123,
            eventType: "task_registered",
            outcome: "succeeded",
            phasePath: "onboarding",
            resultingPhasePath: "onboarding",
            payloadJson: { status: "active" },
          },
          {
            source: "action",
            id: "ae_1742947200124_0000000002",
            createdAtUnixMilli: 1742947201123,
            requestId: "req_1742947201123_0000000002",
            toolName: "unknown_exchange",
            serverName: "filesystem",
            payloadJson: { path: "/workspace/project/tmp" },
          },
        ]),
      ),
    ) as typeof fetch;

    renderApp(["/tasks/tr_1742947200123_0000000001"]);

    expect(await screen.findByText("Agent Task Details")).toBeInTheDocument();
    const titles = screen.getAllByTestId("timeline-event-title").map((element) => element.textContent);
    expect(titles).toEqual(["Task Registered"]);
  });

  it("throws for exchange items without a request or response anchor", () => {
    const invalidItem: TimelineItem = {
      id: "invalid",
      kind: "exchange",
      exchange: {},
    };

    expect(() => getTimelineAnchorEvent(invalidItem)).toThrow(
      "timeline exchange is missing both request and response events",
    );
  });

  it("collapses and re-expands the side inspector", async () => {
    const user = userEvent.setup();
    globalThis.fetch = vi.fn(() =>
      Promise.resolve(
        createFetchResponse([
          {
            source: "task",
            id: "te_1742947200123_0000000001",
            createdAtUnixMilli: 1742947200123,
            eventType: "task_registered",
            outcome: "succeeded",
            phasePath: "onboarding",
            resultingPhasePath: "onboarding",
            payloadJson: { status: "active" },
          },
        ]),
      ),
    ) as typeof fetch;

    renderApp(["/tasks/tr_1742947200123_0000000001"]);

    expect(await screen.findByText("Agent Task Details")).toBeInTheDocument();
    expect(screen.queryByText("Inspector")).not.toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: /show event details for task registered/i }));
    expect(screen.getByText("Inspector")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: "Hide detail panel" }));
    expect(screen.queryByText("Inspector")).not.toBeInTheDocument();
  });

  it("keeps onboarding and planning completion events in their source phase", async () => {
    globalThis.fetch = vi.fn(() =>
      Promise.resolve(
        createFetchResponse([
          {
            source: "task",
            id: "te_1742947200123_0000000001",
            createdAtUnixMilli: 1742947200123,
            eventType: "onboarding_completed",
            outcome: "succeeded",
            phasePath: "onboarding",
            resultingPhasePath: "planning",
            payloadJson: { status: "active" },
          },
          {
            source: "task",
            id: "te_1742947200124_0000000002",
            createdAtUnixMilli: 1742947201123,
            eventType: "planning_completed",
            outcome: "succeeded",
            phasePath: "planning",
            resultingPhasePath: "scaffolding.step_1",
            payloadJson: { status: "active" },
          },
          {
            source: "task",
            id: "te_1742947200125_0000000003",
            createdAtUnixMilli: 1742947202123,
            eventType: "step_started",
            outcome: "succeeded",
            phasePath: "planning",
            resultingPhasePath: "scaffolding.step_1",
            payloadJson: { status: "active" },
          },
        ]),
      ),
    ) as typeof fetch;

    renderApp(["/tasks/tr_1742947200123_0000000001"]);

    expect(await screen.findByText("Agent Task Details")).toBeInTheDocument();

    const onboardingSection = screen.getByLabelText("Onboarding");
    expect(within(onboardingSection).getByText("Onboarding Completed")).toBeInTheDocument();
    expect(within(onboardingSection).getByText("− passed")).toBeInTheDocument();

    const planningSection = screen.getByLabelText("Planning");
    expect(within(planningSection).getByText("Planning Completed")).toBeInTheDocument();
    expect(within(planningSection).getByText("− passed")).toBeInTheDocument();
  });

  it("normalizes inspector status labels to success", async () => {
    const user = userEvent.setup();
    globalThis.fetch = vi.fn(() =>
      Promise.resolve(
        createFetchResponse([
          {
            source: "task",
            id: "te_1742947200123_0000000001",
            createdAtUnixMilli: 1742947200123,
            eventType: "planning_completed",
            outcome: "succeeded",
            phasePath: "planning",
            resultingPhasePath: "scaffolding.step_1",
            payloadJson: {},
          },
          {
            source: "action",
            id: "ae_1742947200124_0000000002",
            createdAtUnixMilli: 1742947201123,
            requestId: "req_1742947201123_0000000002",
            direction: "response",
            toolName: "execute_command",
            success: true,
            serverName: "shell",
            payloadJson: { output: "ok" },
          },
        ]),
      ),
    ) as typeof fetch;

    renderApp(["/tasks/tr_1742947200123_0000000001"]);

    expect(await screen.findByText("Agent Task Details")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: /show event details for planning completed/i }));
    expect(screen.getAllByText("success").length).toBeGreaterThan(0);
    expect(screen.queryByText("succeeded")).not.toBeInTheDocument();
    expect(screen.queryByText("ok")).not.toBeInTheDocument();
  });

  it("uses terminal inspector labels for centian task events even when payload status stays active", async () => {
    const user = userEvent.setup();
    globalThis.fetch = vi.fn(() =>
      Promise.resolve(
        createFetchResponse([
          {
            source: "task",
            id: "te_1742947200123_0000000001",
            createdAtUnixMilli: 1742947200123,
            eventType: "planning_completed",
            outcome: "succeeded",
            phasePath: "planning",
            resultingPhasePath: "scaffolding.step_1",
            payloadJson: { status: "active" },
          },
          {
            source: "task",
            id: "te_1742947200124_0000000002",
            createdAtUnixMilli: 1742947201123,
            eventType: "step_completed",
            outcome: "failed",
            phasePath: "execution.step_1",
            resultingPhasePath: "execution.step_1",
            payloadJson: { status: "active" },
          },
        ]),
      ),
    ) as typeof fetch;

    renderApp(["/tasks/tr_1742947200123_0000000001"]);

    expect(await screen.findByText("Agent Task Details")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: /show event details for planning completed/i }));
    const inspector = screen.getByText("Inspector").closest(".task-detail-panel__surface");
    expect(inspector).not.toBeNull();
    expect(within(inspector as HTMLElement).getAllByText("success").length).toBeGreaterThan(0);
    expect(within(inspector as HTMLElement).queryByText("active")).not.toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: /show event details for step completed/i }));
    expect(within(inspector as HTMLElement).getAllByText("error").length).toBeGreaterThan(0);
    expect(within(inspector as HTMLElement).queryByText("active")).not.toBeInTheDocument();
  });

  it("shows success for successful start events in the inspector even while the run stays active", async () => {
    const user = userEvent.setup();
    globalThis.fetch = vi.fn(() =>
      Promise.resolve(
        createFetchResponse([
          {
            source: "task",
            id: "te_1742947200123_0000000001",
            createdAtUnixMilli: 1742947200123,
            eventType: "task_registered",
            outcome: "succeeded",
            phasePath: "onboarding",
            resultingPhasePath: "onboarding",
            payloadJson: { status: "active" },
          },
          {
            source: "task",
            id: "te_1742947200124_0000000002",
            createdAtUnixMilli: 1742947201123,
            eventType: "step_started",
            outcome: "succeeded",
            phasePath: "planning",
            resultingPhasePath: "scaffolding.step_1",
            payloadJson: { status: "active", step: 1 },
          },
        ]),
      ),
    ) as typeof fetch;

    renderApp(["/tasks/tr_1742947200123_0000000001"]);

    expect(await screen.findByText("Agent Task Details")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: /show event details for step started/i }));
    const inspector = screen.getByText("Inspector").closest(".task-detail-panel__surface");
    expect(inspector).not.toBeNull();
    expect(within(inspector as HTMLElement).getAllByText("success").length).toBeGreaterThan(0);
    expect(within(inspector as HTMLElement).queryByText("active")).not.toBeInTheDocument();
  });

  it("collapses and expands phase sections", async () => {
    const user = userEvent.setup();
    globalThis.fetch = vi.fn(() =>
      Promise.resolve(
        createFetchResponse([
          {
            source: "task",
            id: "te_1742947200123_0000000001",
            createdAtUnixMilli: 1742947200123,
            eventType: "task_registered",
            outcome: "succeeded",
            phasePath: "onboarding",
            resultingPhasePath: "onboarding",
            payloadJson: { status: "active" },
          },
          {
            source: "task",
            id: "te_1742947200124_0000000002",
            createdAtUnixMilli: 1742947201123,
            eventType: "step_started",
            outcome: "succeeded",
            phasePath: "onboarding",
            resultingPhasePath: "planning",
            payloadJson: { status: "active" },
          },
        ]),
      ),
    ) as typeof fetch;

    renderApp(["/tasks/tr_1742947200123_0000000001"]);

    expect(await screen.findByText("Agent Task Details")).toBeInTheDocument();
    const onboardingSection = screen.getByLabelText("Onboarding");
    expect(within(onboardingSection).getByText("Task Registered")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: /01 onboarding 1 events/i }));
    expect(within(onboardingSection).queryByText("Task Registered")).not.toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: /01 onboarding 1 events/i }));
    expect(within(onboardingSection).getByText("Task Registered")).toBeInTheDocument();
  });

  it("collapses only the selected repeated phase section after a restart", async () => {
    const user = userEvent.setup();
    globalThis.fetch = vi.fn(() =>
      Promise.resolve(
        createFetchResponse([
          {
            source: "task",
            id: "te_1742947200123_0000000001",
            createdAtUnixMilli: 1742947200123,
            eventType: "task_registered",
            outcome: "succeeded",
            phasePath: "onboarding",
            resultingPhasePath: "onboarding",
            payloadJson: { status: "active" },
          },
          {
            source: "task",
            id: "te_1742947200123_0000000002",
            createdAtUnixMilli: 1742947201123,
            eventType: "planning_completed",
            outcome: "succeeded",
            phasePath: "planning",
            resultingPhasePath: "execution.step_1",
            payloadJson: { status: "active" },
          },
          {
            source: "task",
            id: "te_1742947200123_0000000003",
            createdAtUnixMilli: 1742947202123,
            eventType: "restarted",
            outcome: "succeeded",
            phasePath: "execution.step_1",
            resultingPhasePath: "onboarding",
            payloadJson: { status: "active" },
          },
          {
            source: "task",
            id: "te_1742947200123_0000000004",
            createdAtUnixMilli: 1742947203123,
            eventType: "restarted",
            outcome: "succeeded",
            phasePath: "execution.step_1",
            resultingPhasePath: "onboarding",
            payloadJson: { status: "active" },
          },
          {
            source: "task",
            id: "te_1742947200123_0000000005",
            createdAtUnixMilli: 1742947204123,
            eventType: "planning_completed",
            outcome: "succeeded",
            phasePath: "planning",
            resultingPhasePath: "execution.step_1",
            payloadJson: { status: "active" },
          },
        ]),
      ),
    ) as typeof fetch;

    renderApp(["/tasks/tr_1742947200123_0000000001"]);

    expect(await screen.findByText("Agent Task Details")).toBeInTheDocument();
    expect(screen.queryByLabelText(/Run Quality:/)).not.toBeInTheDocument();

    const planningSections = screen.getAllByLabelText("Planning");
    expect(planningSections).toHaveLength(2);
    expect(within(planningSections[0] as HTMLElement).getByText("Planning Completed")).toBeInTheDocument();
    expect(within(planningSections[1] as HTMLElement).getByText("Planning Completed")).toBeInTheDocument();

    const planningButtons = screen.getAllByRole("button", { name: /planning 1 events/i });
    expect(planningButtons).toHaveLength(2);

    await user.click(planningButtons[0] as HTMLElement);

    expect(within(planningSections[0] as HTMLElement).queryByText("Planning Completed")).not.toBeInTheDocument();
    expect(within(planningSections[1] as HTMLElement).getByText("Planning Completed")).toBeInTheDocument();
  });

  it("renders a not-found state for 404 responses", async () => {
    globalThis.fetch = vi.fn(() => Promise.resolve(createFetchResponse({ error: "missing" }, 404))) as typeof fetch;

    renderApp(["/tasks/tr_1742947200123_0000000001"]);

    expect(await screen.findByText("Task run not found")).toBeInTheDocument();
  });

  it("renders an invalid-run state for 400 responses", async () => {
    globalThis.fetch = vi.fn(() => Promise.resolve(createFetchResponse({ error: "invalid" }, 400))) as typeof fetch;

    renderApp(["/tasks/not-a-run-id"]);

    expect(await screen.findByText("Invalid task run id")).toBeInTheDocument();
  });

  it("renders a generic error state for network or server failures", async () => {
    globalThis.fetch = vi.fn(() => Promise.reject(new Error("boom"))) as typeof fetch;

    renderApp(["/tasks/tr_1742947200123_0000000001"]);

    expect(await screen.findByText("Task timeline unavailable")).toBeInTheDocument();
  });

  it("navigates back to the task list", async () => {
    const user = userEvent.setup();
    globalThis.fetch = vi
      .fn()
      .mockResolvedValueOnce(
        createFetchResponse([
          {
            source: "task",
            id: "te_1742947200123_0000000001",
            createdAtUnixMilli: 1742947200123,
            eventType: "task_registered",
            outcome: "succeeded",
            phasePath: "planning.review",
            resultingPhasePath: "planning.review",
            payloadJson: { status: "active" },
          },
        ]),
      )
      .mockResolvedValueOnce(createTaskRunDetailResponse("tr_1742947200123_0000000001"))
      .mockResolvedValueOnce(createFetchResponse([])) as typeof fetch;

    renderApp(["/tasks/tr_1742947200123_0000000001"]);

    await user.click(await screen.findByRole("link", { name: "Back to task runs" }));
    expect(await screen.findByText("No task runs yet")).toBeInTheDocument();
  });
});

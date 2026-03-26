import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import { afterEach, describe, expect, it, vi } from "vitest";

import { AppRoutes } from "./routes";

const originalFetch = globalThis.fetch;

function createFetchResponse(body: unknown, status: number = 200): Response {
  return {
    ok: status >= 200 && status < 300,
    status,
    json: async () => body,
  } as Response;
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

function renderApp(initialEntries: string[] = ["/tasks"]) {
  return render(
    <MemoryRouter
      initialEntries={initialEntries}
      future={{ v7_startTransition: true, v7_relativeSplatPath: true }}
    >
      <AppRoutes />
    </MemoryRouter>,
  );
}

afterEach(() => {
  vi.restoreAllMocks();
  globalThis.fetch = originalFetch;
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

    expect(await screen.findByText("python_tdd_demo")).toBeInTheDocument();
    expect(screen.getByText("Execution / Implement Fix")).toBeInTheDocument();
    expect(screen.getByText("5")).toBeInTheDocument();
  });

  it("shows an empty state when no task runs are returned", async () => {
    globalThis.fetch = vi.fn(() => Promise.resolve(createFetchResponse([]))) as typeof fetch;

    renderApp();

    expect(await screen.findByText("No task runs yet")).toBeInTheDocument();
  });

  it("shows an error state when the api request fails", async () => {
    globalThis.fetch = vi.fn(() => Promise.reject(new Error("network down"))) as typeof fetch;

    renderApp();

    expect(await screen.findByText("Task run feed unavailable")).toBeInTheDocument();
  });

  it("maps active completed and failed status badges", async () => {
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
        ]),
      ),
    ) as typeof fetch;

    renderApp();

    expect(await screen.findByText("active")).toBeInTheDocument();
    expect(screen.getByText("completed")).toBeInTheDocument();
    expect(screen.getByText("failed")).toBeInTheDocument();
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
      ) as typeof fetch;

    renderApp();

    await user.click(await screen.findByRole("link", { name: /tr_1742947200123_0000000001/i }));

    expect(await screen.findByText("Task run timeline")).toBeInTheDocument();
    expect(screen.getByText(/tr_1742947200123_0000000001/i)).toBeInTheDocument();
  });
});

describe("task run detail", () => {
  it("shows a loading state before the event api resolves", async () => {
    const pending = deferred<Response>();
    globalThis.fetch = vi.fn(() => pending.promise) as typeof fetch;

    renderApp(["/tasks/tr_1742947200123_0000000001"]);

    expect(screen.getByTestId("task-run-detail-loading")).toBeInTheDocument();
    pending.resolve(createFetchResponse([]));
    await waitFor(() => {
      expect(screen.getByText("Task run timeline")).toBeInTheDocument();
    });
  });

  it("renders a grouped mixed timeline and expands inline payload details", async () => {
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
            direction: "request",
            toolName: "execute_command",
            serverName: "shell",
            gateway: "taskverification",
            transport: "http",
            payloadJson: { command: "pwd" },
          },
          {
            source: "task",
            id: "te_1742947200125_0000000003",
            createdAtUnixMilli: 1742947202123,
            eventType: "step_completed",
            outcome: "succeeded",
            phasePath: "onboarding",
            resultingPhasePath: "execution.implement_fix",
            nodeKind: "planning",
            resultingNodeKind: "execution",
            payloadJson: { status: "active", step: 1 },
          },
          {
            source: "action",
            id: "ae_1742947200126_0000000004",
            createdAtUnixMilli: 1742947203123,
            direction: "response",
            toolName: "edit_file",
            success: true,
            serverName: "filesystem",
            transport: "http",
            payloadJson: { nested: true },
          },
        ]),
      ),
    ) as typeof fetch;

    renderApp(["/tasks/tr_1742947200123_0000000001"]);

    expect(await screen.findByText("Task run timeline")).toBeInTheDocument();
    expect(screen.getByText("Onboarding")).toBeInTheDocument();
    expect(screen.getByText("Task Registered")).toBeInTheDocument();
    expect(screen.getByText("Request · execute_command")).toBeInTheDocument();

    const titles = screen.getAllByTestId("timeline-event-title").map((element) => element.textContent);
    expect(titles).toEqual([
      "Task Registered",
      "Request · execute_command",
      "Step Completed",
      "Response · edit_file",
    ]);

    expect(screen.queryByText(/"nested": true/)).not.toBeInTheDocument();
    await user.click(screen.getByRole("button", { name: /toggle event details for response/i }));
    expect(screen.getByText(/"nested": true/)).toBeInTheDocument();

    const onboardingSection = screen.getByLabelText("Onboarding");
    expect(within(onboardingSection).getByText("Step Completed")).toBeInTheDocument();
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

    expect(await screen.findByText("Task run timeline")).toBeInTheDocument();
    expect(screen.getByText("Task Registered")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: /sector 01 onboarding 1 events/i }));
    expect(screen.queryByText("Task Registered")).not.toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: /sector 01 onboarding 1 events/i }));
    expect(screen.getByText("Task Registered")).toBeInTheDocument();
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
      .mockResolvedValueOnce(createFetchResponse([])) as typeof fetch;

    renderApp(["/tasks/tr_1742947200123_0000000001"]);

    await user.click(await screen.findByRole("link", { name: "Back to task runs" }));
    expect(await screen.findByText("No task runs yet")).toBeInTheDocument();
  });
});

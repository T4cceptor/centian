import { render, screen, waitFor, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import { afterEach, describe, expect, it, vi } from "vitest";

import { AppRoutes } from "./routes";
import { clearStoredApiAuth } from "./api/api-auth";

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
  clearStoredApiAuth();
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

    expect(await screen.findByText("python_tdd_demo")).toBeInTheDocument();
    expect(globalThis.fetch).toHaveBeenNthCalledWith(
      2,
      "/api/task-runs",
      expect.objectContaining({
        headers: expect.any(Headers),
      }),
    );
    const headers = (vi.mocked(globalThis.fetch).mock.calls[1]?.[1] as RequestInit)?.headers as Headers;
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

  it("maps active success and failed status badges", async () => {
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
    expect(screen.getByText("success")).toBeInTheDocument();
    expect(screen.getByText("failed")).toBeInTheDocument();
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

    expect(await screen.findByText("completed_task")).toBeInTheDocument();
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
      ) as typeof fetch;

    renderApp();

    await user.click(await screen.findByRole("link", { name: /tr_1742947200123_0000000001/i }));

    expect(await screen.findByText("Run Detail")).toBeInTheDocument();
    expect(screen.getByText(/TR · 0000000001/)).toBeInTheDocument();
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
      expect(screen.getByText("Run Detail")).toBeInTheDocument();
    });
  });

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
          },
        ]),
      ),
    ) as typeof fetch;

    renderApp(["/tasks/tr_1742947200123_0000000001"]);

    expect(await screen.findByText("Run Detail")).toBeInTheDocument();
    expect(screen.getByLabelText("Onboarding")).toBeInTheDocument();
    expect(screen.getByText("300ms")).toBeInTheDocument();

    const titles = screen.getAllByTestId("timeline-event-title").map((element) => element.textContent);
    expect(titles).toEqual([
      "Task Registered",
      "shell - execute_command",
      "Step Completed · Onboarding",
      "filesystem - edit_file",
    ]);
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

    expect(await screen.findByText("Run Detail")).toBeInTheDocument();
    const titles = screen.getAllByTestId("timeline-event-title").map((element) => element.textContent);
    expect(titles).toEqual(["Task Registered", "shell - execute_command", "filesystem - edit_file"]);
    expect(screen.getByText("error")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: /show event details for execute_command/i }));
    expect(screen.getAllByText("pending")).toHaveLength(2);

    await user.click(screen.getByRole("button", { name: /show event details for edit_file/i }));
    expect(screen.getAllByText("failed")).toHaveLength(2);
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

    expect(await screen.findByText("Run Detail")).toBeInTheDocument();
    const titles = screen.getAllByTestId("timeline-event-title").map((element) => element.textContent);
    expect(titles).toEqual(["Task Registered", "filesystem - create_directory"]);
    expect(screen.getByText("300ms")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: /show event details for create_directory/i }));
    expect(screen.getAllByText("Request")).toHaveLength(2);
    expect(screen.getAllByText("Response")).toHaveLength(2);
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

    expect(await screen.findByText("Run Detail")).toBeInTheDocument();
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

    expect(await screen.findByText("Run Detail")).toBeInTheDocument();

    const onboardingSection = screen.getByLabelText("Onboarding");
    expect(within(onboardingSection).getByText("Onboarding Completed")).toBeInTheDocument();

    const planningSection = screen.getByLabelText("Planning");
    expect(within(planningSection).getByText("Planning Completed")).toBeInTheDocument();
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

    expect(await screen.findByText("Run Detail")).toBeInTheDocument();
    const onboardingSection = screen.getByLabelText("Onboarding");
    expect(within(onboardingSection).getByText("Task Registered")).toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: /sector 01 onboarding 1 events/i }));
    expect(within(onboardingSection).queryByText("Task Registered")).not.toBeInTheDocument();

    await user.click(screen.getByRole("button", { name: /sector 01 onboarding 1 events/i }));
    expect(within(onboardingSection).getByText("Task Registered")).toBeInTheDocument();
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

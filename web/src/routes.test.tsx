import { render, screen, waitFor } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import { afterEach, describe, expect, it, vi } from "vitest";

import { AppRoutes } from "./routes";

const originalFetch = globalThis.fetch;

function createFetchResponse(body: unknown, ok: boolean = true): Response {
  return {
    ok,
    status: ok ? 200 : 500,
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

  it("navigates to the placeholder detail route when a row is clicked", async () => {
    const user = userEvent.setup();
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

    renderApp();

    await user.click(await screen.findByRole("link", { name: /tr_1742947200123_0000000001/i }));

    expect(await screen.findByText("Task run detail view")).toBeInTheDocument();
    expect(screen.getByText(/tr_1742947200123_0000000001/i)).toBeInTheDocument();
  });
});

describe("task run detail placeholder", () => {
  it("renders the selected run id on the detail route", async () => {
    renderApp(["/tasks/tr_1742947200123_0000000001"]);

    expect(await screen.findByText("Task run detail view")).toBeInTheDocument();
    expect(screen.getByText(/tr_1742947200123_0000000001/i)).toBeInTheDocument();
  });
});

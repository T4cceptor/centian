import { render, screen, within } from "@testing-library/react";
import userEvent from "@testing-library/user-event";
import { MemoryRouter } from "react-router-dom";
import { afterEach, describe, expect, it, vi } from "vitest";

import { clearStoredApiAuth } from "./api/api-auth";
import { AppRoutes } from "./routes";

const originalFetch = globalThis.fetch;

function createFetchResponse(body: unknown, status: number = 200): Response {
  return {
    ok: status >= 200 && status < 300,
    status,
    headers: new Headers(),
    json: async () => body,
  } as Response;
}

function renderApp(initialEntries: string[]) {
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

describe("benchmark routes", () => {
  it("renders the suite list", async () => {
    const user = userEvent.setup();
    globalThis.fetch = vi.fn((input) => {
      const url = String(input);
      if (url.includes("/template-scorecards")) {
        return Promise.resolve(
          createFetchResponse([
            {
              templateKey: "zeta_template",
              templateId: "zeta_template",
              templateName: "Zeta Template",
              runCount: 3,
              totalTaskToolCalls: 5,
              totalDownstreamToolCalls: 2,
              medianTaskToolCalls: 3,
              medianDownstreamToolCalls: 1,
              totalCentianErrors: 4,
              totalDownstreamToolErrors: 0,
              medianCentianErrors: 2,
              medianDownstreamToolErrors: 0,
              medianDurationMillis: 70000,
              successRate: 0.9,
              firstPassRate: 0.4,
            },
            {
              templateKey: "alpha_template",
              templateId: "alpha_template",
              templateName: "Alpha Template",
              runCount: 10,
              totalTaskToolCalls: 6,
              totalDownstreamToolCalls: 3,
              medianTaskToolCalls: 2,
              medianDownstreamToolCalls: 1,
              totalCentianErrors: 2,
              totalDownstreamToolErrors: 0,
              medianCentianErrors: 1,
              medianDownstreamToolErrors: 0,
              medianDurationMillis: 105000,
              successRate: 0.7,
              firstPassRate: 0.6,
            },
            {
              templateKey: "beta_template",
              templateId: "beta_template",
              templateName: "Beta Template",
              runCount: 10,
              totalTaskToolCalls: 7,
              totalDownstreamToolCalls: 2,
              medianTaskToolCalls: 4,
              medianDownstreamToolCalls: 2,
              totalCentianErrors: 1,
              totalDownstreamToolErrors: 1,
              medianCentianErrors: 2,
              medianDownstreamToolErrors: 2,
              medianDurationMillis: 50000,
              successRate: 0.85,
              firstPassRate: 0.8,
            },
          ]),
        );
      }
      if (url.includes("/agent-scorecards")) {
        return Promise.resolve(
          createFetchResponse([
            {
              agent: "codex",
              model: "gpt-5.4",
              models: ["gpt-5.4"],
              runCount: 4,
              totalTaskToolCalls: 124,
              totalDownstreamToolCalls: 48,
              medianTaskToolCalls: 31,
              medianDownstreamToolCalls: 12,
              totalCentianErrors: 4,
              totalDownstreamToolErrors: 0,
              medianCentianErrors: 1,
              medianDownstreamToolErrors: 0,
              medianDurationMillis: 103000,
              successRate: 0.5,
              firstPassRate: 0.25,
            },
            {
              agent: "alpha-agent",
              model: "gpt-5.4-mini",
              models: ["gpt-5.4-mini"],
              runCount: 10,
              totalTaskToolCalls: 300,
              totalDownstreamToolCalls: 120,
              medianTaskToolCalls: 30,
              medianDownstreamToolCalls: 12,
              totalCentianErrors: 20,
              totalDownstreamToolErrors: 10,
              medianCentianErrors: 2,
              medianDownstreamToolErrors: 1,
              medianDurationMillis: 90000,
              successRate: 0.8,
              firstPassRate: 0.7,
            },
            {
              agent: "alpha-agent",
              model: "gpt-5.4",
              models: ["gpt-5.4"],
              runCount: 8,
              totalTaskToolCalls: 220,
              totalDownstreamToolCalls: 88,
              medianTaskToolCalls: 27,
              medianDownstreamToolCalls: 11,
              totalCentianErrors: 12,
              totalDownstreamToolErrors: 4,
              medianCentianErrors: 1,
              medianDownstreamToolErrors: 1,
              medianDurationMillis: 80000,
              successRate: 0.65,
              firstPassRate: 0.55,
            },
          ]),
        );
      }
      return Promise.resolve(
        createFetchResponse([
          {
            suiteId: "simple_tdd_v1",
            suiteName: "Simple TDD Benchmark Suite v1",
            templateId: "simple_tdd",
            templateName: "Simple TDD Current",
            latestGeneratedAt: "2026-04-05T12:00:00Z",
            sessionCount: 2,
            runCount: 4,
          },
        ]),
      );
    }) as typeof fetch;

    renderApp(["/benchmarks"]);

    expect(await screen.findByRole("heading", { name: "Simple TDD Benchmark Suite v1" })).toBeInTheDocument();
    expect(screen.getAllByText("Simple TDD Current").length).toBeGreaterThan(0);
    expect(screen.getByText("Template Scorecards")).toBeInTheDocument();
    expect(screen.getByText("Success Rate")).toBeInTheDocument();
    expect(screen.getByText("MCP Events (Centian/MCP)")).toBeInTheDocument();
    expect(screen.getAllByText("Total").length).toBeGreaterThan(0);
    expect(screen.getAllByText("Median").length).toBeGreaterThan(0);

    const templateTable = screen.getByRole("table", { name: "Template scorecards" });
    expect(scorecardLabels(templateTable)).toEqual([
      "Alpha Template",
      "Beta Template",
      "Zeta Template",
    ]);

    await user.click(within(templateTable).getByRole("button", { name: /Sort by Runs/ }));
    expect(scorecardLabels(templateTable)).toEqual([
      "Zeta Template",
      "Alpha Template",
      "Beta Template",
    ]);

    await user.click(within(templateTable).getByRole("button", { name: /Sort by Runs/ }));
    expect(scorecardLabels(templateTable)).toEqual([
      "Alpha Template",
      "Beta Template",
      "Zeta Template",
    ]);

    await user.click(within(templateTable).getByRole("button", { name: /Sort by Success Rate/ }));
    expect(scorecardLabels(templateTable)).toEqual([
      "Alpha Template",
      "Beta Template",
      "Zeta Template",
    ]);

    await user.click(within(templateTable).getByRole("button", { name: /Sort by MCP Events/ }));
    expect(scorecardLabels(templateTable)).toEqual([
      "Zeta Template",
      "Alpha Template",
      "Beta Template",
    ]);

    await user.click(within(templateTable).getByRole("button", { name: /Sort by Errors/ }));
    expect(scorecardLabels(templateTable)).toEqual([
      "Alpha Template",
      "Beta Template",
      "Zeta Template",
    ]);

    await user.click(screen.getByRole("button", { name: "Agent" }));
    expect(screen.getByText("Agent Scorecards")).toBeInTheDocument();
    const agentTable = screen.getByRole("table", { name: "Agent scorecards" });
    expect(scorecardLabels(agentTable)).toEqual([
      "alpha-agent gpt-5.4",
      "alpha-agent gpt-5.4-mini",
      "codex gpt-5.4",
    ]);

    await user.click(within(agentTable).getByRole("button", { name: /Sort by Success Rate/ }));
    expect(scorecardLabels(agentTable)).toEqual([
      "codex gpt-5.4",
      "alpha-agent gpt-5.4",
      "alpha-agent gpt-5.4-mini",
    ]);

    expect(screen.getAllByText("gpt-5.4").length).toBeGreaterThan(0);
    expect(screen.getAllByText("gpt-5.4-mini").length).toBeGreaterThan(0);
    expect(screen.getAllByText("2").length).toBeGreaterThan(0);
  });

  it("renders scorecard empty state when no persisted metrics exist", async () => {
    globalThis.fetch = vi.fn((input) => {
      const url = String(input);
      if (url.includes("/template-scorecards") || url.includes("/agent-scorecards")) {
        return Promise.resolve(createFetchResponse([]));
      }
      return Promise.resolve(createFetchResponse([]));
    }) as typeof fetch;

    renderApp(["/benchmarks"]);

    expect(await screen.findByText("No persisted benchmark metrics are available yet.")).toBeInTheDocument();
    expect(screen.getByText("No benchmark suites yet")).toBeInTheDocument();
  });

  it("renders the suite overview", async () => {
    globalThis.fetch = vi.fn((input) => {
      const url = String(input);
      if (url.includes("/sessions")) {
        return Promise.resolve(
          createFetchResponse([
            {
              sessionId: "ba_session",
              suiteId: "simple_tdd_v1",
              suiteName: "Simple TDD Benchmark Suite v1",
              templateId: "simple_tdd",
              templateName: "Simple TDD Current",
              sessionPath: "/tmp/simple_tdd/session_one",
              generatedAt: "2026-04-05T12:00:00Z",
              runCount: 2,
              scoredRunCount: 2,
              failedToScoreCount: 0,
              aggregates: { byCase: [], byAgent: [], byTemplateVariant: [], byCaseAgentVariant: [] },
            },
          ]),
        );
      }
      if (url.includes("/comparison")) {
        return Promise.resolve(
          createFetchResponse({
            suiteId: "simple_tdd_v1",
            suiteName: "Simple TDD Benchmark Suite v1",
            templateId: "simple_tdd",
            templateName: "Simple TDD Current",
            sessionCount: 1,
            runCount: 1,
            sessions: [],
            runs: [],
            filters: {},
            aggregates: {
              bySession: [],
              byCase: [],
              byAgent: [{ key: "codex", agent: "codex", runCount: 1, scoredRunCount: 1, successRate: 1, firstPassSuccessRate: 1, finalVerificationPassRate: 1, invariantViolationRate: 0, restartFailTimeoutRate: 0, medianWallClockSeconds: 10, medianTotalToolCalls: 4, medianInputTokens: 100, medianOutputTokens: 50, medianFailedTaskToolCalls: 0, medianFailedDownstreamToolCalls: 0, medianEditedFilesCount: 1 }],
              byTemplateVariant: [],
              byCaseAgentVariant: [],
            },
          }),
        );
      }
      return Promise.resolve(
        createFetchResponse([
          {
            scorecardId: "ba_score",
            sessionId: "ba_session",
            sessionPath: "/tmp/simple_tdd/session_one",
            suiteId: "simple_tdd_v1",
            suiteName: "Simple TDD Benchmark Suite v1",
            templateId: "simple_tdd",
            templateName: "Simple TDD Current",
            caseId: "assertion_failure_red",
            caseName: "Assertion-failure red baseline",
            agent: "codex",
            selectedModel: "gpt-5.4-mini",
            templateVariant: "current",
            attempt: 1,
            rawStatus: "completed",
            scored: true,
            completedSuccessfully: true,
            finalVerificationPassed: true,
            firstPassSuccess: false,
            invariantViolation: false,
            restartOccurred: true,
            failOccurred: false,
            timeoutOccurred: false,
            wallClockSeconds: 10,
            totalToolCalls: 4,
            totalTaskToolCalls: 1,
            totalDownstreamToolCalls: 3,
            failedTaskToolCalls: 0,
            failedDownstreamToolCalls: 0,
            editedFilesCount: 1,
          },
        ]),
      );
    }) as typeof fetch;

    renderApp(["/benchmarks/simple_tdd_v1"]);

    expect(await screen.findByRole("heading", { name: "Simple TDD Benchmark Suite v1" })).toBeInTheDocument();
    expect(screen.getAllByText("Simple TDD Current").length).toBeGreaterThan(0);
    expect(screen.getByText("By Variant")).toBeInTheDocument();
    expect(screen.getByText("By Agent")).toBeInTheDocument();
    expect(screen.getAllByText("Total Actions (Centian/MCP)").length).toBeGreaterThan(0);
    expect(screen.getByText("1 Runs")).toBeInTheDocument();
    expect(screen.getByText("1 Sessions")).toBeInTheDocument();
    expect(
      screen.getAllByText((_, element) => (element?.textContent ?? "").replace(/\s+/g, "") === "1/1").length,
    ).toBeGreaterThan(0);
    expect(screen.getByText("Run History")).toBeInTheDocument();
    expect(screen.getAllByText("Assertion-failure red baseline").length).toBeGreaterThan(0);
    expect(screen.getAllByText("codex / gpt-5.4-mini").length).toBeGreaterThan(0);
    expect(screen.getByRole("link", { name: "Show task runs" })).toHaveAttribute(
      "href",
      "/tasks?benchmarkSuite=simple_tdd_v1",
    );
  });

  it("renders the session detail", async () => {
    globalThis.fetch = vi.fn(() =>
      Promise.resolve(
        createFetchResponse({
          sessionId: "ba_session",
          suiteId: "simple_tdd_v1",
          suiteName: "Simple TDD Benchmark Suite v1",
          templateId: "simple_tdd",
          templateName: "Simple TDD Current",
          sessionPath: "/tmp/simple_tdd/session_one",
          generatedAt: "2026-04-05T12:00:00Z",
          runCount: 2,
          scoredRunCount: 2,
          failedToScoreCount: 0,
          agents: ["codex"],
          templateVariants: ["current"],
          aggregates: {
            byCase: [{ key: "assertion_failure_red", caseId: "assertion_failure_red", runCount: 1, scoredRunCount: 1, successRate: 1, firstPassSuccessRate: 1, finalVerificationPassRate: 1, invariantViolationRate: 0, restartFailTimeoutRate: 0, medianWallClockSeconds: 10, medianTotalToolCalls: 4, medianInputTokens: 100, medianOutputTokens: 50, medianFailedTaskToolCalls: 0, medianFailedDownstreamToolCalls: 0, medianEditedFilesCount: 1 }],
            byAgent: [],
            byTemplateVariant: [],
            byCaseAgentVariant: [],
          },
          runs: [
            {
              scorecardId: "ba_score",
              sessionId: "ba_session",
              sessionPath: "/tmp/simple_tdd/session_one",
              suiteId: "simple_tdd_v1",
              suiteName: "Simple TDD Benchmark Suite v1",
              templateId: "simple_tdd",
              templateName: "Simple TDD Current",
              caseId: "assertion_failure_red",
              caseName: "Assertion-failure red baseline",
              agent: "codex",
              selectedModel: "gpt-5.4-mini",
              templateVariant: "current",
              attempt: 1,
              rawStatus: "completed",
              scored: true,
              completedSuccessfully: true,
              finalVerificationPassed: true,
              firstPassSuccess: true,
              invariantViolation: false,
              restartOccurred: false,
              failOccurred: false,
              timeoutOccurred: false,
              wallClockSeconds: 10,
              totalToolCalls: 4,
              totalTaskToolCalls: 1,
              totalDownstreamToolCalls: 3,
              failedTaskToolCalls: 0,
              failedDownstreamToolCalls: 0,
              editedFilesCount: 1,
            },
          ],
        }),
      ),
    ) as typeof fetch;

    renderApp(["/benchmarks/simple_tdd_v1/sessions/ba_session"]);

    expect(await screen.findByText("Session Runs")).toBeInTheDocument();
    expect(screen.getAllByText("Assertion-failure red baseline").length).toBeGreaterThan(0);
  });

  it("renders the run detail and handles unauthorized benchmark access", async () => {
    const user = userEvent.setup();
    globalThis.fetch = vi
      .fn()
      .mockResolvedValueOnce({
        ok: false,
        status: 401,
        headers: new Headers({ "X-Centian-Auth-Header": "X-Centian-Auth" }),
      } as Response)
      .mockResolvedValueOnce(
        createFetchResponse({
          scorecardId: "ba_score",
          sessionId: "ba_session",
          sessionPath: "/tmp/simple_tdd/session_one",
          suiteName: "Simple TDD Benchmark Suite v1",
          templateName: "Simple TDD Current",
          caseName: "Assertion-failure red baseline",
          scored: true,
          scorecard: {
            suiteId: "simple_tdd_v1",
            suiteName: "Simple TDD Benchmark Suite v1",
            caseId: "assertion_failure_red",
            caseName: "Assertion-failure red baseline",
            templateId: "simple_tdd",
            templateName: "Simple TDD Current",
            templateVariant: "current",
            agent: "codex",
            selectedModel: "gpt-5.4-mini",
            attempt: 1,
            rawStatus: "completed",
            outcome: {
              completedSuccessfully: true,
              finalVerificationPassed: true,
              firstPassSuccess: true,
              restartOccurred: false,
              failOccurred: false,
              timeoutOccurred: false,
              invariantViolation: false,
            },
            process: {
              failedTaskToolCalls: 0,
              failedDownstreamToolCalls: 0,
              totalTaskToolCalls: 2,
              totalDownstreamToolCalls: 2,
              restartCount: 0,
              failCount: 0,
              timeoutCount: 0,
            },
            efficiency: {
              wallClockSeconds: 10,
              totalToolCalls: 4,
              inputTokens: 100,
              outputTokens: 50,
              editedFilesCount: 1,
            },
            agentMetadata: { selectedModel: "gpt-5.4-mini" },
            generatedAt: "2026-04-05T12:00:00Z",
          },
        }),
      ) as typeof fetch;

    renderApp(["/benchmarks/simple_tdd_v1/runs/ba_score"]);

    expect(await screen.findByText("Benchmark run is protected")).toBeInTheDocument();
    await user.type(screen.getByLabelText("API key"), "plain-key");
    await user.click(screen.getByRole("button", { name: "Save and retry" }));

    expect(screen.getAllByText("Outcome").length).toBeGreaterThan(0);
    expect(screen.getByText("Edited Files")).toBeInTheDocument();
    expect(screen.getAllByText("gpt-5.4-mini").length).toBeGreaterThan(0);
    expect(screen.getByText("Restart Count")).toBeInTheDocument();
    expect(screen.getByRole("heading", { name: "Assertion-failure red baseline" })).toBeInTheDocument();
  });
});

function scorecardRowTexts(table: HTMLElement): string[] {
  return within(table)
    .getAllByRole("row")
    .slice(1)
    .map((row) => row.textContent ?? "");
}

function scorecardLabels(table: HTMLElement): string[] {
  return scorecardRowTexts(table).map((_, index) => {
    const row = within(table).getAllByRole("row").slice(1)[index];
    const label = row.querySelector(".benchmark-analysis-row__label");
    if (!label) {
      return "";
    }
    return Array.from(label.children)
      .map((child) => child.textContent ?? "")
      .join(" ")
      .replace(/\s+/g, " ")
      .trim();
  });
}

import { Link } from "react-router-dom";

import type { BenchmarkRunSummary } from "../api/benchmarks";
import { formatDuration, formatTemplateLabel } from "./format";
import { formatBenchmarkRate } from "./benchmark-format";

type BenchmarkRunTableProps = {
  suiteId: string;
  runs: BenchmarkRunSummary[];
};

export function BenchmarkRunTable({ suiteId, runs }: BenchmarkRunTableProps) {
  if (runs.length === 0) {
    return <p className="benchmark-empty">No benchmark runs match the current filters.</p>;
  }

  return (
    <div className="benchmark-table" role="table" aria-label="Benchmark runs">
      <div className="benchmark-table__header benchmark-table__header--runs" role="row">
        <span>Case</span>
        <span>Agent / Model</span>
        <span>Variant</span>
        <span>Template</span>
        <span>Success</span>
        <span>First Pass</span>
        <span>Duration</span>
        <span>Tokens</span>
      </div>
      <div className="benchmark-table__body" role="rowgroup">
        {runs.map((run) => (
          <Link
            key={run.scorecardId}
            className="benchmark-row benchmark-row--runs"
            to={`/benchmarks/${suiteId}/runs/${run.scorecardId}`}
          >
            <span title={run.caseId}>{run.caseName || run.caseId}</span>
            <span title={agentModelLabel(run.agent, selectedRunModel(run))}>
              {agentModelLabel(run.agent, selectedRunModel(run))}
            </span>
            <span>{run.templateVariant}</span>
            <span title={run.templateId}>{run.templateName || formatTemplateLabel(run.templateId)}</span>
            <span>{formatBenchmarkRate(run.completedSuccessfully ? 1 : 0)}</span>
            <span>{formatBenchmarkRate(run.firstPassSuccess ? 1 : 0)}</span>
            <span>{formatDuration(0, Math.round(run.wallClockSeconds * 1000))}</span>
            <span>
              {(run.inputTokens ?? 0) + (run.outputTokens ?? 0)}
            </span>
          </Link>
        ))}
      </div>
    </div>
  );
}

function selectedRunModel(run: BenchmarkRunSummary): string {
  return run.selectedModel || run.agentMetadata?.selectedModel || firstModelUsageKey(run.agentMetadata?.modelUsage) || "";
}

function firstModelUsageKey(modelUsage?: Record<string, unknown>): string {
  if (!modelUsage) {
    return "";
  }
  return Object.keys(modelUsage).sort()[0] ?? "";
}

function agentModelLabel(agent: string, model: string): string {
  return model ? `${agent} / ${model}` : agent;
}

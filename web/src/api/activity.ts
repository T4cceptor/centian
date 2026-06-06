// Activity ("Intervention Skyline") data client.
//
// Backed by GET /api/{projectSlug}/activity, which aggregates interventions,
// request-volume buckets, and headline stats from the event store.

import { projectApiPath, requestJSON } from "./task-runs";

export type ActivityRange = "1h" | "6h" | "1d" | "1w";

export type InterventionCategory = "security" | "policy" | "risk" | "quality" | "compliance";

export const interventionCategories: InterventionCategory[] = [
  "security",
  "policy",
  "risk",
  "quality",
  "compliance",
];

export const categoryLabels: Record<InterventionCategory, string> = {
  security: "Security",
  policy: "Policy",
  risk: "Risk",
  quality: "Quality",
  compliance: "Compliance",
};

export type Intervention = {
  id: string;
  category: InterventionCategory;
  timestampUnixMilli: number;
  severity: number; // 0..1 → marker height
  title: string;
  summary: string;
  ruleId: string;
  ruleExplanation: string;
  toolName: string;
  gateway: string;
  serverName: string;
  label?: string; // short chart label, e.g. "Redacted"
};

export type VolumePoint = {
  timeUnixMilli: number;
  volume: number;
};

export type ActivityStats = {
  interventions: number;
  threatsNeutralized: number;
  piiRedacted: number;
  riskyActionsHeld: number;
  requestsInspected: number;
};

export type ActivitySummary = {
  rangeStartUnixMilli: number;
  rangeEndUnixMilli: number;
  stats: ActivityStats;
  categoryCounts: Record<InterventionCategory, number>;
  volume: VolumePoint[];
  interventions: Intervention[];
};

// An activity window is either a quick relative preset or an explicit custom
// range with absolute start/end timestamps (unix milli).
export type ActivityQuery =
  | { kind: "preset"; range: ActivityRange }
  | { kind: "custom"; startUnixMilli: number; endUnixMilli: number };

function buildActivityQuery(query: ActivityQuery): string {
  if (query.kind === "custom") {
    const params = new URLSearchParams({
      start: String(query.startUnixMilli),
      end: String(query.endUnixMilli),
    });
    return `?${params.toString()}`;
  }
  return `?range=${encodeURIComponent(query.range)}`;
}

export async function fetchActivitySummary(
  projectSlug: string | undefined,
  query: ActivityQuery,
  signal?: AbortSignal,
): Promise<ActivitySummary> {
  const summary = await requestJSON<ActivitySummary>(
    `${projectApiPath(projectSlug, "/activity")}${buildActivityQuery(query)}`,
    signal,
  );
  return {
    rangeStartUnixMilli: summary?.rangeStartUnixMilli ?? 0,
    rangeEndUnixMilli: summary?.rangeEndUnixMilli ?? 0,
    stats: summary?.stats ?? {
      interventions: 0,
      threatsNeutralized: 0,
      piiRedacted: 0,
      riskyActionsHeld: 0,
      requestsInspected: 0,
    },
    categoryCounts: summary?.categoryCounts ?? {
      security: 0,
      policy: 0,
      risk: 0,
      quality: 0,
      compliance: 0,
    },
    volume: Array.isArray(summary?.volume) ? summary.volume : [],
    interventions: Array.isArray(summary?.interventions) ? summary.interventions : [],
  };
}

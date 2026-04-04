// Formats an API timestamp using the user's locale with date and minute precision.
export function formatTimestamp(unixMilli: number): string {
  return new Intl.DateTimeFormat(undefined, {
    year: "numeric",
    month: "short",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
  }).format(unixMilli);
}

function padTimestampPart(value: number): string {
  return String(value).padStart(2, "0");
}

// Formats an API timestamp into compact date and time parts for dense table layouts.
export function formatTimestampCompact(unixMilli: number): { date: string; time: string } {
  const value = new Date(unixMilli);
  const date = [
    value.getFullYear(),
    padTimestampPart(value.getMonth() + 1),
    padTimestampPart(value.getDate()),
  ].join("/");
  const time = [
    padTimestampPart(value.getHours()),
    padTimestampPart(value.getMinutes()),
    padTimestampPart(value.getSeconds()),
  ].join(":");

  return { date, time };
}

// Renders elapsed time for both finished and still-running task runs.
export function formatDuration(startedAt: number, endedAt?: number, now: number = Date.now()): string {
  const totalSeconds = Math.max(0, Math.floor(((endedAt ?? now) - startedAt) / 1000));
  const hours = Math.floor(totalSeconds / 3600);
  const minutes = Math.floor((totalSeconds % 3600) / 60);
  const seconds = totalSeconds % 60;

  if (hours > 0) {
    return `${hours}h ${minutes}m`;
  }
  if (minutes > 0) {
    return `${minutes}m ${seconds}s`;
  }
  return `${seconds}s`;
}

// Turns a dotted phase path into a readable breadcrumb for the UI.
export function humanizePhase(phasePath: string): string {
  if (!phasePath) {
    return "Unknown";
  }
  return phasePath
    .split(".")
    .filter(Boolean)
    .map((segment) =>
      segment
        .split("_")
        .filter(Boolean)
        .map((word) => word[0].toUpperCase() + word.slice(1))
        .join(" "),
    )
    .join(" / ");
}

// Converts machine identifiers into title-cased labels.
export function humanizeIdentifier(value: string): string {
  if (!value) {
    return "Unknown";
  }

  return value
    .replace(/[._-]+/g, " ")
    .split(" ")
    .filter(Boolean)
    .map((word) => word[0].toUpperCase() + word.slice(1))
    .join(" ");
}

// Converts template ids into UI labels while preserving common acronyms.
export function formatTemplateLabel(templateId: string, templateName?: string): string {
  const base = (templateName ?? "").trim() || humanizeIdentifier(templateId);

  return base.replace(/\b(Api|Cli|Mcp|Sql|Sqlite|Tdd|Ui)\b/g, (match) => match.toUpperCase());
}

// Shortens canonical task run ids while still preserving a stable suffix for operators.
export function formatTaskRunId(value: string): string {
  if (!value) {
    return "TR";
  }

  const match = /^tr_\d{13}_([a-z0-9]{10})$/.exec(value);
  if (match) {
    return `TR · ${match[1]}`;
  }

  if (value.length <= 16) {
    return value;
  }

  return `${value.slice(0, 8)}…${value.slice(-6)}`;
}

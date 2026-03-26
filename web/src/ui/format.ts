export function formatTimestamp(unixMilli: number): string {
  return new Intl.DateTimeFormat(undefined, {
    year: "numeric",
    month: "short",
    day: "2-digit",
    hour: "2-digit",
    minute: "2-digit",
  }).format(unixMilli);
}

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

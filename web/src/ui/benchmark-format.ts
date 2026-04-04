import { humanizeIdentifier } from "./format";

export function formatBenchmarkRate(value: number): string {
  return `${Math.round(value * 100)}%`;
}

export function formatBenchmarkSeconds(value: number): string {
  if (!Number.isFinite(value) || value <= 0) {
    return "0s";
  }
  if (value >= 60) {
    const minutes = Math.floor(value / 60);
    const seconds = Math.round(value % 60);
    return `${minutes}m ${seconds}s`;
  }
  return `${Math.round(value)}s`;
}

export function formatBenchmarkLabel(value: string): string {
  return humanizeIdentifier(value);
}

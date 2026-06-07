import { describe, expect, it } from "vitest";

import { formatCompactNumber, formatDuration, formatTaskRunId, formatTimestampCompact } from "./format";

describe("formatDuration", () => {
  it("uses endedAt for finished runs", () => {
    expect(formatDuration(1_000, 11_000, 99_000)).toBe("10s");
  });

  it("uses now for active runs", () => {
    expect(formatDuration(1_000, undefined, 7_000)).toBe("6s");
  });
});

describe("formatCompactNumber", () => {
  it("returns small counts verbatim", () => {
    expect(formatCompactNumber(0)).toBe("0");
    expect(formatCompactNumber(999)).toBe("999");
  });

  it("abbreviates thousands and millions with ~3 significant digits", () => {
    expect(formatCompactNumber(1234)).toBe("1.23k");
    expect(formatCompactNumber(12_345)).toBe("12.3k");
    expect(formatCompactNumber(4_500_000)).toBe("4.5M");
    expect(formatCompactNumber(2_000_000_000)).toBe("2B");
  });
});

describe("formatTaskRunId", () => {
  it("compacts canonical task run identifiers down to their suffix", () => {
    expect(formatTaskRunId("tr_1774532248833_85kws87i5l")).toBe("TR · 85kws87i5l");
  });
});

describe("formatTimestampCompact", () => {
  it("returns zero-padded local date and time parts", () => {
    const value = new Date(2026, 3, 4, 9, 7, 5).getTime();
    expect(formatTimestampCompact(value)).toEqual({
      date: "2026/04/04",
      time: "09:07:05",
    });
  });
});

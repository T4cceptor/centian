import { describe, expect, it } from "vitest";

import { formatDuration, formatTaskRunId } from "./format";

describe("formatDuration", () => {
  it("uses endedAt for finished runs", () => {
    expect(formatDuration(1_000, 11_000, 99_000)).toBe("10s");
  });

  it("uses now for active runs", () => {
    expect(formatDuration(1_000, undefined, 7_000)).toBe("6s");
  });
});

describe("formatTaskRunId", () => {
  it("compacts canonical task run identifiers down to their suffix", () => {
    expect(formatTaskRunId("tr_1774532248833_85kws87i5l")).toBe("TR · 85kws87i5l");
  });
});

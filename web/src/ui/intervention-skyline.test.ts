import { describe, expect, it } from "vitest";

import { viewBoxXToTime } from "./intervention-skyline";

// PLOT_LEFT=16, PLOT_RIGHT=1400 (internal viewBox geometry).
describe("viewBoxXToTime", () => {
  const start = 1_000;
  const end = 2_000;

  it("maps the plot's left edge to the window start", () => {
    expect(viewBoxXToTime(start, end, 16)).toBe(1_000);
  });

  it("maps the plot's right edge to the window end", () => {
    expect(viewBoxXToTime(start, end, 1400)).toBe(2_000);
  });

  it("maps the midpoint to the window midpoint", () => {
    expect(viewBoxXToTime(start, end, (16 + 1400) / 2)).toBe(1_500);
  });

  it("clamps coordinates outside the plot to the window bounds", () => {
    expect(viewBoxXToTime(start, end, -50)).toBe(1_000);
    expect(viewBoxXToTime(start, end, 5_000)).toBe(2_000);
  });
});

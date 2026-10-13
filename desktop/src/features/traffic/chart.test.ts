import { describe, expect, it } from "vitest";
import { areaPath, formatRate, monotonePath, niceTicks, sampleAt, type Point } from "./chart";

const segments = (d: string) =>
  d
    .split("C")
    .slice(1)
    .map((c) => c.split(/[,LZ]/).slice(0, 6).map(Number));

describe("formatRate", () => {
  it("picks the unit and rounds to a readable precision", () => {
    expect(formatRate(0)).toBe("0 B/s");
    expect(formatRate(512.4)).toBe("512 B/s");
    expect(formatRate(1500)).toBe("1.5 KB/s");
    expect(formatRate(2000)).toBe("2 KB/s");
    expect(formatRate(12_345)).toBe("12 KB/s");
    expect(formatRate(4_210_000)).toBe("4.2 MB/s");
    expect(formatRate(3_000_000_000)).toBe("3 GB/s");
  });

  it("moves to the next unit instead of showing 1000", () => {
    expect(formatRate(999.6)).toBe("1 KB/s");
    expect(formatRate(999_700)).toBe("1 MB/s");
    expect(formatRate(9_960)).toBe("10 KB/s");
  });

  it("treats negative input as idle", () => {
    expect(formatRate(-5)).toBe("0 B/s");
  });
});

describe("niceTicks", () => {
  it("returns three round ticks from zero that cover the maximum", () => {
    expect(niceTicks(3_200_000)).toEqual([0, 2_000_000, 4_000_000]);
    expect(niceTicks(4_000_000)).toEqual([0, 2_000_000, 4_000_000]);
    expect(niceTicks(7_100)).toEqual([0, 5_000, 10_000]);
    expect(niceTicks(180_000)).toEqual([0, 100_000, 200_000]);
    expect(niceTicks(4_100_000)).toEqual([0, 2_500_000, 5_000_000]);
  });

  it("keeps a small scale when the network is idle", () => {
    expect(niceTicks(0)).toEqual([0, 500, 1000]);
  });
});

describe("sampleAt", () => {
  const samples = [1000, 2000, 3000].map((at) => ({ at, rx: at, tx: 0 }));

  it("finds the nearest sample", () => {
    expect(sampleAt(samples, 2400)?.at).toBe(2000);
  });

  it("finds nothing when no sample is close", () => {
    expect(sampleAt(samples, 6000)).toBeUndefined();
    expect(sampleAt([], 1000)).toBeUndefined();
  });
});

describe("monotonePath", () => {
  it("passes through every point", () => {
    const pts: Point[] = [
      [0, 50],
      [10, 20],
      [20, 80],
    ];
    const d = monotonePath(pts);
    expect(d.startsWith("M0,50C")).toBe(true);
    expect(segments(d).map((s) => s.slice(4))).toEqual([
      [10, 20],
      [20, 80],
    ]);
  });

  it("never overshoots the data between two points", () => {
    const pts: Point[] = [
      [0, 100],
      [10, 90],
      [20, 0],
      [30, 0],
      [40, 100],
      [50, 100],
      [60, 40],
    ];
    segments(monotonePath(pts)).forEach(([, c1, , c2], i) => {
      const lo = Math.min(pts[i][1], pts[i + 1][1]);
      const hi = Math.max(pts[i][1], pts[i + 1][1]);
      for (const y of [c1, c2]) {
        expect(y).toBeGreaterThanOrEqual(lo);
        expect(y).toBeLessThanOrEqual(hi);
      }
    });
  });

  it("handles one point and none", () => {
    expect(monotonePath([[5, 5]])).toBe("M5,5");
    expect(monotonePath([])).toBe("");
  });
});

describe("areaPath", () => {
  it("closes the line down to the baseline", () => {
    const pts: Point[] = [
      [0, 40],
      [10, 60],
    ];
    expect(areaPath(pts, 100)).toBe(`${monotonePath(pts)}L10,100L0,100Z`);
  });

  it("is empty without a line to fill", () => {
    expect(areaPath([[0, 40]], 100)).toBe("");
  });
});

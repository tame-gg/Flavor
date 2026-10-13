export type Sample = { at: number; rx: number; tx: number };

export type Point = readonly [x: number, y: number];

const units = ["B/s", "KB/s", "MB/s", "GB/s"];

export function formatRate(bytesPerSecond: number): string {
  let v = Math.max(0, bytesPerSecond);
  let i = 0;
  while (i < units.length - 1 && Math.round(v) >= 1000) {
    v /= 1000;
    i++;
  }
  return `${i === 0 || v >= 10 ? Math.round(v) : Number(v.toFixed(1))} ${units[i]}`;
}

export function niceTicks(max: number, count = 3): number[] {
  const raw = Math.max(max, 1000) / (count - 1);
  const magnitude = 10 ** Math.floor(Math.log10(raw));
  const step = ([1, 2, 2.5, 5].find((m) => m * magnitude >= raw) ?? 10) * magnitude;
  return Array.from({ length: count }, (_, i) => i * step);
}

export function sampleAt(samples: readonly Sample[], at: number, tolerance = 1500): Sample | undefined {
  const best = samples.reduce<Sample | undefined>(
    (b, s) => (!b || Math.abs(s.at - at) < Math.abs(b.at - at) ? s : b),
    undefined,
  );
  return best && Math.abs(best.at - at) <= tolerance ? best : undefined;
}

const r = (v: number) => Math.round(v * 100) / 100;

export function monotonePath(points: readonly Point[]): string {
  const n = points.length;
  if (n === 0) return "";
  let d = `M${r(points[0][0])},${r(points[0][1])}`;
  if (n === 1) return d;
  const dx = points.slice(1).map(([x], i) => x - points[i][0]);
  const slope = points.slice(1).map(([, y], i) => (dx[i] ? (y - points[i][1]) / dx[i] : 0));
  const m = points.map((_, i) => {
    if (i === 0) return slope[0];
    if (i === n - 1) return slope[n - 2];
    return slope[i - 1] * slope[i] <= 0 ? 0 : (slope[i - 1] + slope[i]) / 2;
  });
  for (let i = 0; i < n - 1; i++) {
    if (slope[i] === 0) {
      m[i] = 0;
      m[i + 1] = 0;
      continue;
    }
    const a = m[i] / slope[i];
    const b = m[i + 1] / slope[i];
    const s = a * a + b * b;
    if (s > 9) {
      const t = 3 / Math.sqrt(s);
      m[i] = t * a * slope[i];
      m[i + 1] = t * b * slope[i];
    }
  }
  for (let i = 0; i < n - 1; i++) {
    const h = dx[i] / 3;
    const [x0, y0] = points[i];
    const [x1, y1] = points[i + 1];
    d += `C${r(x0 + h)},${r(y0 + m[i] * h)},${r(x1 - h)},${r(y1 - m[i + 1] * h)},${r(x1)},${r(y1)}`;
  }
  return d;
}

export function areaPath(points: readonly Point[], baseline: number): string {
  if (points.length < 2) return "";
  const [first, last] = [points[0][0], points[points.length - 1][0]];
  return `${monotonePath(points)}L${r(last)},${r(baseline)}L${r(first)},${r(baseline)}Z`;
}

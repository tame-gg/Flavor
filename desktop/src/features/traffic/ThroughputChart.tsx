import { useState, type KeyboardEvent, type PointerEvent } from "react";
import { cn } from "@/lib/cn";
import { ToggleGroup, ToggleGroupItem } from "@/ui/toggle-group";
import { areaPath, formatRate, monotonePath, niceTicks, sampleAt, type Sample } from "./chart";

export type Series = { id: string; name: string; color: string; samples: Sample[] };

type Direction = "rx" | "tx";

const WINDOW = 120_000;
const W = 1000;
const H = 100;
const xLabels = ["2m", "90s", "1m", "30s", "now"];
const directionName: Record<Direction, string> = { rx: "Download", tx: "Upload" };

const keys: Record<string, (age: number) => number> = {
  ArrowLeft: (a) => Math.min(WINDOW, a + 1000),
  ArrowRight: (a) => Math.max(0, a - 1000),
  Home: () => WINDOW,
  End: () => 0,
};

const latest = (s: Series, d: Direction) => {
  const last = s.samples.at(-1);
  return last ? formatRate(last[d]) : "—";
};

export function ThroughputChart({ series }: { series: Series[] }) {
  const [dir, setDir] = useState<Direction>("rx");
  const [age, setAge] = useState<number | null>(null);
  const now = Math.max(0, ...series.flatMap((s) => s.samples.map((p) => p.at)));
  const visible = series.map((s) => ({ ...s, samples: s.samples.filter((p) => p.at >= now - WINDOW) }));
  const ticks = niceTicks(Math.max(0, ...visible.flatMap((s) => s.samples.map((p) => p[dir]))));
  const top = ticks[ticks.length - 1];
  const lines = visible.map((s) => ({
    ...s,
    points: s.samples.map((p) => [(1 - (now - p.at) / WINDOW) * W, (1 - p[dir] / top) * H] as const),
  }));
  const empty = visible.every((s) => s.samples.length < 2);
  const cursor =
    age === null
      ? null
      : {
          label: age === 0 ? "Now" : `${age / 1000}s ago`,
          x: (1 - age / WINDOW) * 100,
          values: visible.map((s) => ({ ...s, value: sampleAt(s.samples, now - age)?.[dir] })),
        };
  const summary = `${directionName[dir]} rate over the last 2 minutes for ${series.map((s) => s.name).join(", ")}`;

  const point = (e: PointerEvent<HTMLDivElement>) => {
    const box = e.currentTarget.getBoundingClientRect();
    const f = Math.min(1, Math.max(0, (e.clientX - box.left) / box.width));
    setAge(Math.round(((1 - f) * WINDOW) / 1000) * 1000);
  };
  const key = (e: KeyboardEvent<HTMLDivElement>) => {
    if (e.key === "Escape") return setAge(null);
    const move = keys[e.key];
    if (!move) return;
    e.preventDefault();
    setAge((a) => move(a ?? 0));
  };

  return (
    <div className="flex flex-col gap-3 border-t border-border px-4 py-3 first:border-t-0">
      <div className="flex flex-wrap items-center justify-between gap-3">
        <ul className="flex flex-wrap items-center gap-x-4 gap-y-1 text-xs" aria-hidden="true">
          {series.map((s) => (
            <li key={s.id} className="flex items-center gap-1.5">
              <span className="size-2.5 shrink-0 rounded-[3px]" style={{ background: s.color }} />
              <span className="font-medium">{s.name}</span>
              <span className="font-mono text-muted-foreground">{latest(s, dir)}</span>
            </li>
          ))}
        </ul>
        <ToggleGroup
          type="single"
          variant="outline"
          size="sm"
          value={dir}
          onValueChange={(v) => v && setDir(v as Direction)}
          aria-label="Traffic direction"
          className="ml-auto"
        >
          <ToggleGroupItem value="rx" className="px-2.5">
            Down
          </ToggleGroupItem>
          <ToggleGroupItem value="tx" className="px-2.5">
            Up
          </ToggleGroupItem>
        </ToggleGroup>
      </div>

      {empty ? (
        <p role="status" className="flex h-46 items-center justify-center text-sm text-muted-foreground">
          Measuring traffic…
        </p>
      ) : (
        <div className="grid grid-cols-[4rem_minmax(0,1fr)] gap-x-3 gap-y-2">
          <div className="relative h-40" aria-hidden="true">
            {ticks.map((t) => (
              <span
                key={t}
                className="absolute right-0 font-mono text-xs text-muted-foreground"
                style={{ top: `${(1 - t / top) * 100}%`, transform: `translateY(-${(1 - t / top) * 100}%)` }}
              >
                {formatRate(t)}
              </span>
            ))}
          </div>
          <div
            role="img"
            aria-label={summary}
            tabIndex={0}
            className="relative h-40 rounded-sm"
            onPointerMove={point}
            onPointerLeave={() => setAge(null)}
            onFocus={() => setAge((a) => a ?? 0)}
            onBlur={() => setAge(null)}
            onKeyDown={key}
          >
            <svg
              viewBox={`0 0 ${W} ${H}`}
              preserveAspectRatio="none"
              className="absolute inset-0 size-full overflow-visible"
              aria-hidden="true"
            >
              {ticks.map((t) => (
                <line
                  key={t}
                  x1={0}
                  x2={W}
                  y1={(1 - t / top) * H}
                  y2={(1 - t / top) * H}
                  className="stroke-border"
                  vectorEffect="non-scaling-stroke"
                />
              ))}
              {lines.map((s) => (
                <path key={`area:${s.id}`} d={areaPath(s.points, H)} style={{ fill: s.color }} fillOpacity={0.08} />
              ))}
              {lines.map((s) => (
                <path
                  key={`line:${s.id}`}
                  d={monotonePath(s.points)}
                  fill="none"
                  style={{ stroke: s.color }}
                  strokeWidth={1.5}
                  strokeLinejoin="round"
                  vectorEffect="non-scaling-stroke"
                />
              ))}
            </svg>
            {cursor !== null && (
              <>
                <div
                  className="pointer-events-none absolute inset-y-0 w-px bg-foreground/25"
                  style={{ left: `${cursor.x}%` }}
                />
                {cursor.values.map(
                  (s) =>
                    s.value !== undefined && (
                      <span
                        key={s.id}
                        className="pointer-events-none absolute size-2 -translate-x-1/2 -translate-y-1/2 rounded-full ring-2 ring-card"
                        style={{ left: `${cursor.x}%`, top: `${(1 - s.value / top) * 100}%`, background: s.color }}
                      />
                    ),
                )}
                <div
                  className={cn(
                    "pointer-events-none absolute top-2 z-10 flex min-w-40 flex-col gap-1 rounded-md border border-border bg-popover px-3 py-2 text-xs text-popover-foreground shadow-md",
                    cursor.x > 50 ? "-ml-2 -translate-x-full" : "ml-2",
                  )}
                  style={{ left: `${cursor.x}%` }}
                >
                  <span className="text-muted-foreground">{cursor.label}</span>
                  {cursor.values.map((s) => (
                    <span key={s.id} className="flex items-center gap-2">
                      <span className="size-2.5 shrink-0 rounded-[3px]" style={{ background: s.color }} />
                      <span className="flex-1">{s.name}</span>
                      <span className="font-mono">{s.value === undefined ? "—" : formatRate(s.value)}</span>
                    </span>
                  ))}
                </div>
              </>
            )}
          </div>
          <div />
          <div className="relative h-4 font-mono text-xs text-muted-foreground" aria-hidden="true">
            {xLabels.map((label, i) => (
              <span
                key={label}
                className="absolute top-0"
                style={{ left: `${i * 25}%`, transform: `translateX(-${i * 25}%)` }}
              >
                {label}
              </span>
            ))}
          </div>
        </div>
      )}

      <table className="sr-only">
        <caption>Current traffic</caption>
        <thead>
          <tr>
            <th scope="col">Network</th>
            <th scope="col">Download</th>
            <th scope="col">Upload</th>
          </tr>
        </thead>
        <tbody>
          {series.map((s) => (
            <tr key={s.id}>
              <th scope="row">{s.name}</th>
              <td>{latest(s, "rx")}</td>
              <td>{latest(s, "tx")}</td>
            </tr>
          ))}
        </tbody>
      </table>
    </div>
  );
}

import { formatRate, monotonePath, type Sample } from "./chart";

const W = 64;
const H = 20;

export function Sparkline({ samples, color }: { samples: Sample[]; color: string }) {
  const top = Math.max(1000, ...samples.map((s) => s.rx));
  const step = W / Math.max(1, samples.length - 1);
  const d = monotonePath(samples.map((s, i) => [i * step, H - 1 - (s.rx / top) * (H - 2)] as const));
  return (
    <span className="flex shrink-0 items-center gap-2">
      <svg width={W} height={H} viewBox={`0 0 ${W} ${H}`} className="overflow-visible" aria-hidden="true">
        <path d={d} fill="none" style={{ stroke: color }} strokeWidth={1.5} strokeLinejoin="round" />
      </svg>
      <span className="w-16 text-right font-mono text-xs text-muted-foreground">
        <span className="sr-only">Download </span>
        {formatRate(samples.at(-1)?.rx ?? 0)}
      </span>
    </span>
  );
}

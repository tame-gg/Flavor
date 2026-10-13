import type { Conflict } from "@gen/flavor/v1/conflicts_pb";

export type OverlapNetwork = { id: string; name: string };
export type Overlap = { networks: OverlapNetwork[]; counts: number[][]; max: number };

export function shares(c: Conflict, a: string, b: string): boolean {
  const ids = c.members.map((m) => m.network?.id);
  const onA = ids.filter((id) => id === a).length;
  return a === b ? onA > 1 : onA > 0 && ids.includes(b);
}

export function overlapMatrix(list: readonly Conflict[]): Overlap {
  const names = new Map<string, string>();
  for (const c of list) for (const m of c.members) if (m.network) names.set(m.network.id, m.network.displayName);
  const networks = [...names]
    .map(([id, name]) => ({ id, name }))
    .sort((x, y) => x.name.localeCompare(y.name) || x.id.localeCompare(y.id));
  const counts = networks.map((a) => networks.map((b) => list.filter((c) => shares(c, a.id, b.id)).length));
  return { networks, counts, max: Math.max(0, ...counts.flat()) };
}

export function heat(count: number, max: number): 0 | 1 | 2 | 3 {
  if (count <= 0 || max <= 0) return 0;
  return Math.min(3, Math.ceil((3 * count) / max)) as 1 | 2 | 3;
}

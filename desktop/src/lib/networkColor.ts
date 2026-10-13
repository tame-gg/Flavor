import { timestampMs } from "@bufbuild/protobuf/wkt";
import type { Network } from "@gen/flavor/v1/network_pb";

export const NETWORK_COLORS = 8;

export function networkColorIndex(networks: Iterable<Network>): Map<string, number> {
  const ordered = [...networks].sort((a, b) => {
    const at = a.createdAt ? timestampMs(a.createdAt) : 0;
    const bt = b.createdAt ? timestampMs(b.createdAt) : 0;
    return at - bt || a.id.localeCompare(b.id);
  });
  return new Map(ordered.map((n, i) => [n.id, i % NETWORK_COLORS]));
}

export const networkColorVar = (index: number | undefined) => `var(--ui-net-${(index ?? NETWORK_COLORS - 1) + 1})`;

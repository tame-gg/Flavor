import { NetworkConnectionState as S } from "@gen/lattice/v1/common_pb";
import type { Network } from "@gen/lattice/v1/network_pb";
import type { Workspace } from "@gen/lattice/v1/workspaces_pb";

const running = new Set([S.CONNECTING, S.AUTHENTICATING, S.AWAITING_APPROVAL, S.CONNECTED, S.DEGRADED, S.RECONNECTING]);

export const isRunning = (n: Network) => running.has(n.state);

export type ActivationPlan = { connect: Network[]; alreadyUp: Network[]; outside: Network[] };

export function activationPlan(ws: Workspace, networks: ReadonlyMap<string, Network>): ActivationPlan {
  const members = new Set(ws.networkIds);
  const plan: ActivationPlan = { connect: [], alreadyUp: [], outside: [] };
  const sorted = [...networks.values()].sort((a, b) => a.displayName.localeCompare(b.displayName));
  for (const n of sorted) {
    if (members.has(n.id)) (isRunning(n) ? plan.alreadyUp : plan.connect).push(n);
    else if (isRunning(n)) plan.outside.push(n);
  }
  return plan;
}

export function memberSummary(ws: Workspace, networks: ReadonlyMap<string, Network>): string {
  const members = ws.networkIds.map((id) => networks.get(id)).filter((n): n is Network => n !== undefined);
  if (members.length === 0) return "No networks";
  const connected = members.filter((n) => n.state === S.CONNECTED || n.state === S.DEGRADED).length;
  return `${connected} of ${members.length} network${members.length === 1 ? "" : "s"} connected`;
}

import { NetworkConnectionState as S } from "@gen/flavor/v1/common_pb";
import type { Device } from "@gen/flavor/v1/device_pb";
import type { Network } from "@gen/flavor/v1/network_pb";

export type Tone = "ok" | "warn" | "danger" | "neutral";

export type NeedsYou = {
  key: string;
  tone: Tone;
  networkId?: string;
  title: string;
  detail: string;
  action: "sign-in" | "open-network" | "conflicts";
};

export type Overview = {
  tone: Tone;
  headline: string;
  connected: number;
  total: number;
  devicesOnline: number;
  devicesTotal: number;
  needsYou: NeedsYou[];
};

const live = new Set([S.CONNECTED, S.DEGRADED]);

export function summarize(
  networks: Iterable<Network>,
  devices: Iterable<Device>,
  ambiguousConflicts: number,
): Overview {
  const nets = [...networks].sort((a, b) => a.displayName.localeCompare(b.displayName));
  const needsYou: NeedsYou[] = [];
  for (const n of nets) {
    if (n.authentication?.kind === "authentication" || n.state === S.AUTHENTICATING) {
      needsYou.push({
        key: `signin:${n.id}`,
        tone: "warn",
        networkId: n.id,
        title: n.displayName,
        detail: "needs you to sign in",
        action: "sign-in",
      });
    } else if (n.state === S.AWAITING_APPROVAL) {
      needsYou.push({
        key: `approval:${n.id}`,
        tone: "warn",
        networkId: n.id,
        title: n.displayName,
        detail: "is waiting for an administrator to approve this device",
        action: "open-network",
      });
    } else if (n.state === S.ERROR) {
      needsYou.push({
        key: `error:${n.id}`,
        tone: "danger",
        networkId: n.id,
        title: n.displayName,
        detail: n.statusMessage || "stopped because of an error",
        action: "open-network",
      });
    } else if (n.state === S.DEGRADED) {
      needsYou.push({
        key: `degraded:${n.id}`,
        tone: "warn",
        networkId: n.id,
        title: n.displayName,
        detail: n.statusMessage || "the control server reports a problem",
        action: "open-network",
      });
    }
  }
  if (ambiguousConflicts > 0) {
    needsYou.push({
      key: "conflicts",
      tone: "warn",
      title: `${ambiguousConflicts} ambiguous ${ambiguousConflicts === 1 ? "destination" : "destinations"}`,
      detail: "exist on more than one network",
      action: "conflicts",
    });
  }

  const connected = nets.filter((n) => live.has(n.state)).length;
  const others = [...devices].filter((d) => !d.local);
  const devicesOnline = others.filter((d) => d.online).length;

  let headline: string;
  let tone: Tone;
  if (nets.length === 0) {
    headline = "No networks yet";
    tone = "neutral";
  } else {
    headline = `${connected} of ${nets.length} ${nets.length === 1 ? "network" : "networks"} connected`;
    const first = needsYou.find((i) => i.networkId);
    if (first) headline += ` · ${first.title} needs attention`;
    tone = needsYou.some((i) => i.tone === "danger")
      ? "danger"
      : needsYou.length > 0
        ? "warn"
        : connected === nets.length
          ? "ok"
          : "neutral";
  }

  return { tone, headline, connected, total: nets.length, devicesOnline, devicesTotal: others.length, needsYou };
}

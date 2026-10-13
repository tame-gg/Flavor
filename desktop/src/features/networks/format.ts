import { NetworkConnectionState as S, ProviderType } from "@gen/flavor/v1/common_pb";
import type { Network } from "@gen/flavor/v1/network_pb";

export type Tone = "ok" | "warn" | "danger" | "info" | "neutral";

export const providerName = (p: ProviderType) => (p === ProviderType.HEADSCALE ? "Headscale" : "Tailscale");

export function controlHost(n: Network): string {
  if (!n.controlUrl) return "Tailscale (default)";
  try {
    return new URL(n.controlUrl).host;
  } catch {
    return n.controlUrl;
  }
}

export const networkSource = (n: Network) =>
  n.controlUrl ? `${providerName(n.provider)} · ${controlHost(n)}` : providerName(n.provider);

const active = new Set([S.CONNECTING, S.AUTHENTICATING, S.AWAITING_APPROVAL, S.CONNECTED, S.DEGRADED, S.RECONNECTING]);

export const isActive = (state: S) => active.has(state);

export const isLive = (state: S) => state === S.CONNECTED || state === S.DEGRADED;

const tones: Partial<Record<S, Tone>> = {
  [S.CONNECTED]: "ok",
  [S.CONNECTING]: "info",
  [S.RECONNECTING]: "info",
  [S.AUTHENTICATING]: "warn",
  [S.AWAITING_APPROVAL]: "warn",
  [S.DEGRADED]: "warn",
  [S.ERROR]: "danger",
};

export const stateTone = (state: S): Tone => tones[state] ?? "neutral";

export function deviceSummary(count: number, active: boolean): string {
  if (count > 0) return `${count} device${count === 1 ? "" : "s"} on this network, including this one.`;
  return active ? "No devices reported yet." : "Connect to see devices on this network.";
}

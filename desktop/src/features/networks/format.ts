import { ProviderType } from "@gen/lattice/v1/common_pb";
import type { Network } from "@gen/lattice/v1/network_pb";

export const providerName = (p: ProviderType) => (p === ProviderType.HEADSCALE ? "Headscale" : "Tailscale");

export function controlHost(n: Network): string {
  if (!n.controlUrl) return "Tailscale (default)";
  try {
    return new URL(n.controlUrl).host;
  } catch {
    return n.controlUrl;
  }
}

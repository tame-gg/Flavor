import type { Device } from "@gen/lattice/v1/device_pb";
import type { Network } from "@gen/lattice/v1/network_pb";
import { deviceKey } from "../../app/sync/controller";
import { providerName } from "../networks/format";

export type DeviceRow = { key: string; device: Device; network: Network | undefined; networkName: string };

type Token = { field: "is" | "network" | "os" | "tag" | "node" | "text"; value: string };

const qualifiers = new Set(["is", "network", "os", "tag", "node"]);

export function parseSearch(query: string): Token[] {
  return query
    .trim()
    .toLowerCase()
    .split(/\s+/)
    .filter(Boolean)
    .map((t) => {
      const i = t.indexOf(":");
      const field = i > 0 ? t.slice(0, i) : "";
      return qualifiers.has(field) && i < t.length - 1
        ? { field: field as Token["field"], value: t.slice(i + 1) }
        : { field: "text", value: t };
    });
}

function matches(t: Token, d: Device, network: Network | undefined, networkName: string): boolean {
  const name = networkName.toLowerCase();
  switch (t.field) {
    case "is":
      return t.value === "online" ? d.online : t.value === "offline" ? !d.online : t.value === "local" ? d.local : false;
    case "network":
      return name.includes(t.value) || (d.id?.networkId.toLowerCase() ?? "") === t.value;
    case "os":
      return d.os.toLowerCase().includes(t.value);
    case "tag":
      return d.tags.some((x) => x.toLowerCase() === t.value || x.toLowerCase() === `tag:${t.value}`);
    case "node":
      return (d.id?.nodeId.toLowerCase() ?? "").includes(t.value);
    default:
      return (
        d.hostname.toLowerCase().includes(t.value) ||
        d.dnsName.toLowerCase().includes(t.value) ||
        name.includes(t.value) ||
        (network !== undefined && providerName(network.provider).toLowerCase() === t.value) ||
        d.addresses.some((a) => a.toLowerCase().includes(t.value)) ||
        d.os.toLowerCase().includes(t.value) ||
        d.tags.some((x) => x.toLowerCase().includes(t.value)) ||
        d.routes.some((r) => r.startsWith(t.value))
      );
  }
}

function rank(tokens: Token[], d: Device): number {
  const texts = tokens.filter((t) => t.field === "text").map((t) => t.value);
  if (texts.length === 0) return 2;
  const host = d.hostname.toLowerCase();
  const dns = d.dnsName.toLowerCase().replace(/\.$/, "");
  const exact = texts.some((t) => t === host || t === dns || t === dns.split(".")[0] || d.addresses.includes(t));
  if (exact) return 0;
  return texts.some((t) => host.startsWith(t)) ? 1 : 2;
}

export function deviceRows(
  devices: ReadonlyMap<string, Device>,
  networks: ReadonlyMap<string, Network>,
  networkId: string,
  query: string,
): DeviceRow[] {
  const tokens = parseSearch(query);
  const rows: (DeviceRow & { rank: number })[] = [];
  for (const d of devices.values()) {
    const id = d.id;
    if (!id || (networkId && id.networkId !== networkId)) continue;
    const network = networks.get(id.networkId);
    const networkName = network?.displayName ?? "";
    if (!tokens.every((t) => matches(t, d, network, networkName))) continue;
    rows.push({ key: deviceKey(id.networkId, id.nodeId), device: d, network, networkName, rank: rank(tokens, d) });
  }
  rows.sort((a, b) => a.rank - b.rank || a.device.hostname.localeCompare(b.device.hostname) || a.key.localeCompare(b.key));
  return rows.map(({ rank: _rank, ...row }) => row);
}

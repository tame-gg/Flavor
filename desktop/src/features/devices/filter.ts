import type { Device } from "@gen/lattice/v1/device_pb";
import type { Network } from "@gen/lattice/v1/network_pb";
import { deviceKey } from "../../app/sync/controller";

export type DeviceRow = { key: string; device: Device; networkName: string };

export function deviceRows(
  devices: ReadonlyMap<string, Device>,
  networks: ReadonlyMap<string, Network>,
  networkId: string,
  query: string,
): DeviceRow[] {
  const q = query.trim().toLowerCase();
  const rows: DeviceRow[] = [];
  for (const d of devices.values()) {
    const id = d.id;
    if (!id || (networkId && id.networkId !== networkId)) continue;
    const networkName = networks.get(id.networkId)?.displayName ?? "";
    if (
      q &&
      !d.hostname.toLowerCase().includes(q) &&
      !d.dnsName.toLowerCase().includes(q) &&
      !networkName.toLowerCase().includes(q) &&
      !d.addresses.some((a) => a.includes(q))
    ) {
      continue;
    }
    rows.push({ key: deviceKey(id.networkId, id.nodeId), device: d, networkName });
  }
  return rows.sort((a, b) => a.device.hostname.localeCompare(b.device.hostname) || a.key.localeCompare(b.key));
}

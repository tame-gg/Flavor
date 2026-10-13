import type { Device } from "@gen/flavor/v1/device_pb";

export type Example = { value: string; hint: string; networkIds: string[] };

const firstHost = (prefix: string) => {
  const m = /^(\d+)\.(\d+)\.(\d+)\.(\d+)\/(\d+)$/.exec(prefix);
  return m && Number(m[5]) >= 8 && Number(m[5]) < 32 && Number(m[4]) < 255
    ? `${m[1]}.${m[2]}.${m[3]}.${Number(m[4]) + 1}`
    : null;
};

export function exampleDestinations(devices: Iterable<Device>): Example[] {
  const remote = [...devices].filter((d) => d.id && !d.local);
  const online = remote.filter((d) => d.online);
  const out: Example[] = [];

  const byAddress = new Map<string, Set<string>>();
  for (const d of remote)
    for (const a of d.addresses) byAddress.set(a, (byAddress.get(a) ?? new Set()).add(d.id!.networkId));
  const shared = [...byAddress].filter(([, ids]) => ids.size > 1).sort((a, b) => b[1].size - a[1].size)[0];
  if (shared) out.push({ value: shared[0], hint: `Used on ${shared[1].size} networks`, networkIds: [...shared[1]] });

  const named = online.find((d) => d.dnsName);
  if (named) out.push({ value: named.dnsName, hint: "Full DNS name", networkIds: [named.id!.networkId] });

  const host = online.find((d) => d.hostname && d !== named);
  if (host) out.push({ value: host.hostname, hint: "Device name", networkIds: [host.id!.networkId] });

  for (const d of remote) {
    const prefix = d.routes.find(firstHost);
    if (!prefix) continue;
    out.push({
      value: firstHost(prefix)!,
      hint: `In ${prefix}, routed by ${d.hostname}`,
      networkIds: [d.id!.networkId],
    });
    break;
  }
  return out;
}

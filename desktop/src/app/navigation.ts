import { Capability } from "@gen/lattice/v1/common_pb";

export type Page = "networks" | "devices" | "inspector" | "conflicts" | "diagnostics" | "settings";

const pages: [Page, string, Capability?][] = [
  ["networks", "Networks"],
  ["devices", "Devices"],
  ["inspector", "Connection Inspector", Capability.CONNECTION_INSPECTOR],
  ["conflicts", "Conflicts", Capability.CONFLICT_CENTER],
  ["diagnostics", "Diagnostics"],
  ["settings", "Settings"],
];

export function visiblePages(capabilities: readonly Capability[]): [Page, string][] {
  return pages.filter(([, , needs]) => needs === undefined || capabilities.includes(needs)).map(([id, label]) => [id, label]);
}

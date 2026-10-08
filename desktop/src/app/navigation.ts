import { Capability } from "@gen/flavor/v1/common_pb";

export type Page = "networks" | "workspaces" | "devices" | "inspector" | "conflicts" | "diagnostics" | "settings";

const pages: [Page, string, Capability?][] = [
  ["networks", "Networks"],
  ["workspaces", "Workspaces", Capability.WORKSPACES],
  ["devices", "Devices"],
  ["inspector", "Connection Inspector", Capability.CONNECTION_INSPECTOR],
  ["conflicts", "Conflicts", Capability.CONFLICT_CENTER],
  ["diagnostics", "Diagnostics"],
  ["settings", "Settings"],
];

export function visiblePages(capabilities: readonly Capability[]): [Page, string][] {
  return pages.filter(([, , needs]) => needs === undefined || capabilities.includes(needs)).map(([id, label]) => [id, label]);
}

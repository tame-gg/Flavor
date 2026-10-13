import type { DaemonStatus } from "../../app/sync/controller";
import { isUiError } from "../../lib/api/types";

export function lastSeen(date: Date | undefined, now: Date = new Date()): string | null {
  if (!date || date.getTime() <= 0) return null;
  const seconds = Math.max(0, Math.round((now.getTime() - date.getTime()) / 1000));
  if (seconds < 60) return "just now";
  const minutes = Math.round(seconds / 60);
  if (minutes < 60) return `${minutes} minute${minutes === 1 ? "" : "s"} ago`;
  const hours = Math.round(minutes / 60);
  if (hours < 48) return `${hours} hour${hours === 1 ? "" : "s"} ago`;
  const days = Math.round(hours / 24);
  return `${days} days ago`;
}

export type DeviceBadge = { label: string; active: boolean };

export function deviceBadges(d: {
  local: boolean;
  exitNode: boolean;
  exitNodeOption: boolean;
  routes: readonly string[];
}): DeviceBadge[] {
  const badges: DeviceBadge[] = [];
  if (d.local) badges.push({ label: "This device", active: false });
  if (d.exitNode) badges.push({ label: "Exit node in use", active: true });
  else if (d.exitNodeOption) badges.push({ label: "Exit node", active: false });
  if (d.routes.length > 0) badges.push({ label: "Router", active: false });
  return badges;
}

export function sshTarget(dnsName: string, addresses: readonly string[]): string | null {
  const host = dnsName.replace(/\.$/, "") || addresses[0];
  return host ? `ssh ${host}` : null;
}

export function canRetryDescribe(error: unknown, status: DaemonStatus): boolean {
  return status === "ready" && !(isUiError(error) && error.code === "FLAVOR_ERROR_CODE_DEVICE_NOT_FOUND");
}

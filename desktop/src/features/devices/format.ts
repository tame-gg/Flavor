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

export function sshTarget(dnsName: string, addresses: readonly string[]): string | null {
  const host = dnsName.replace(/\.$/, "") || addresses[0];
  return host ? `ssh ${host}` : null;
}

export const qualifiedSuffix = "lattice.internal";

export function latticeName(
  device: { hostname: string; dnsName: string },
  network: { id: string; label: string } | undefined,
): string | null {
  if (!network) return null;
  const host = device.hostname.toLowerCase();
  const bare = host && !host.includes(".") ? host : device.dnsName.toLowerCase().split(".")[0];
  if (!bare) return null;
  return `${bare}.${network.label || network.id.toLowerCase()}.${qualifiedSuffix}`;
}

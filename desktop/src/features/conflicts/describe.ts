import { ConflictScope, ConflictSeverity, ConflictType, type Conflict } from "@gen/flavor/v1/conflicts_pb";

export const typeLabel: Record<ConflictType, string> = {
  [ConflictType.UNSPECIFIED]: "Overlap",
  [ConflictType.ADDRESS_COLLISION]: "Address",
  [ConflictType.DNS_NAME_COLLISION]: "DNS name",
  [ConflictType.HOSTNAME_COLLISION]: "Device name",
  [ConflictType.SUBNET_OVERLAP]: "Subnet route",
};

export type SeverityFilter = "all" | "ambiguous" | "expected" | "resolved";
export type TypeFilter = "all" | ConflictType;

export function networkCount(c: Conflict): number {
  return new Set(c.members.map((m) => m.network?.id)).size;
}

export function describe(c: Conflict): string {
  const n = networkCount(c);
  if (c.type === ConflictType.SUBNET_OVERLAP) {
    if (c.severity === ConflictSeverity.AMBIGUOUS) {
      return `${n} networks advertise ${c.value}. Addresses inside it cannot be assigned to one network automatically.`;
    }
    return `${c.value} sits inside a wider route on another network. The more specific route decides for addresses inside ${c.value}.`;
  }
  if (c.scope === ConflictScope.WITHIN_NETWORK) {
    return `${c.members.length} devices on ${c.members[0]?.network?.displayName ?? "one network"} report ${c.value}.`;
  }
  if (c.type === ConflictType.DNS_NAME_COLLISION) {
    return `The full DNS name ${c.value} is the same on ${n} networks, so the name alone cannot pick one of them.`;
  }
  if (c.severity === ConflictSeverity.EXPECTED) {
    return `${c.value} exists on ${n} networks. Each device also has its own full DNS name, so it can always be reached unambiguously.`;
  }
  return `${c.value} exists on ${n} networks, and not every device has a network-specific name that tells it apart.`;
}

export function category(c: Conflict): Exclude<SeverityFilter, "all"> {
  if (c.preferredNetworkId) return "resolved";
  return c.severity === ConflictSeverity.AMBIGUOUS ? "ambiguous" : "expected";
}

export function filterConflicts(list: readonly Conflict[], severity: SeverityFilter, type: TypeFilter): Conflict[] {
  return list.filter((c) => (severity === "all" || category(c) === severity) && (type === "all" || c.type === type));
}

export function counts(list: readonly Conflict[]): { ambiguous: number; expected: number; resolved: number } {
  const out = { ambiguous: 0, expected: 0, resolved: 0 };
  for (const c of list) out[category(c)]++;
  return out;
}

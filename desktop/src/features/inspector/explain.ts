import {
  CandidateStatus,
  DecisionReason,
  DestinationKind,
  MatchKind,
  PreferenceState,
  ResolutionDecision,
  type InspectDestinationResponse,
} from "@gen/flavor/v1/inspector_pb";

export const matchLabel: Record<MatchKind, string> = {
  [MatchKind.UNSPECIFIED]: "Match",
  [MatchKind.DEVICE_ADDRESS]: "Exact device address",
  [MatchKind.DEVICE_DNS_NAME]: "Full DNS name",
  [MatchKind.DEVICE_HOSTNAME]: "Device name",
  [MatchKind.SUBNET_ROUTE]: "Subnet route",
  [MatchKind.QUALIFIED_NAME]: "Flavor name",
  [MatchKind.DNS_RECORD]: "DNS record from the network",
  [MatchKind.EXIT_NODE]: "Exit node",
};

export const statusLabel: Record<CandidateStatus, string> = {
  [CandidateStatus.UNSPECIFIED]: "",
  [CandidateStatus.SELECTED]: "Selected",
  [CandidateStatus.TIED]: "Equal match",
  [CandidateStatus.OUTRANKED]: "Weaker match",
};

export function candidateLabel(r: InspectDestinationResponse, status: CandidateStatus, match?: MatchKind): string {
  if (status === CandidateStatus.OUTRANKED && r.reason === DecisionReason.DESTINATION_PREFERENCE) return "Not preferred";
  if (status === CandidateStatus.OUTRANKED && match === MatchKind.SUBNET_ROUTE) return "Less specific route";
  return statusLabel[status];
}

const basis: Record<MatchKind, string> = {
  [MatchKind.UNSPECIFIED]: "a match",
  [MatchKind.DEVICE_ADDRESS]: "an exact device address",
  [MatchKind.DEVICE_DNS_NAME]: "a full DNS name",
  [MatchKind.DEVICE_HOSTNAME]: "a device name",
  [MatchKind.SUBNET_ROUTE]: "a subnet route",
  [MatchKind.QUALIFIED_NAME]: "its Flavor name, which names the network explicitly",
  [MatchKind.DNS_RECORD]: "a DNS record the network publishes",
  [MatchKind.EXIT_NODE]: "an exit node on the network",
};

const what = (r: InspectDestinationResponse) => (r.kind === DestinationKind.ADDRESS ? "address" : "name");

export function explain(r: InspectDestinationResponse): { title: string; detail: string } {
  const tied = r.candidates.filter((c) => c.status === CandidateStatus.TIED);
  const selected = r.candidates.find((c) => c.status === CandidateStatus.SELECTED);
  const outranked = r.candidates.filter((c) => c.status === CandidateStatus.OUTRANKED);
  switch (r.decision) {
    case ResolutionDecision.UNIQUE: {
      const recordLabel = r.decidedBy === MatchKind.DNS_RECORD ? selected?.matchedValue : undefined;
      const device = selected?.device?.hostname || selected?.device?.dnsName || recordLabel || "one device";
      const network = selected?.network?.displayName ?? "one network";
      if (r.reason === DecisionReason.DESTINATION_PREFERENCE) {
        const others = new Set(outranked.map((c) => c.network?.id)).size;
        return {
          title: `${device} on ${network}`,
          detail:
            `Chosen by your Flavor preference for ${network}.` +
            (others > 0 ? ` Without it, ${r.normalized} would match on ${others + 1} networks.` : ""),
        };
      }
      if (r.reason === DecisionReason.EXIT_NODE) {
        return {
          title: `${device} on ${network}`,
          detail: `${r.normalized} is not on any of your networks, so it goes through the exit node chosen for ${network}.`,
        };
      }
      if (r.reason === DecisionReason.SUBNET_ROUTE || r.reason === DecisionReason.LONGEST_PREFIX) {
        const others = outranked.length;
        return {
          title: `${network}, via ${device}`,
          detail:
            `${device} routes ${selected?.matchedValue ?? "this subnet"} on ${network}.` +
            (r.reason === DecisionReason.LONGEST_PREFIX
              ? ` It is more specific than the other matching route${others === 1 ? "" : "s"}, so it wins.`
              : ""),
        };
      }
      const by = basis[r.decidedBy];
      const others =
        outranked.length > 0
          ? ` ${outranked.length} weaker ${outranked.length === 1 ? "match was" : "matches were"} set aside because ${by} is more specific.`
          : "";
      return { title: `${device} on ${network}`, detail: `Matched by ${by}.${others}` };
    }
    case ResolutionDecision.AMBIGUOUS: {
      if (r.reason === DecisionReason.AMBIGUOUS_NETWORK_LABEL) {
        return {
          title: `${r.normalized} uses a network name more than one network shares`,
          detail: "Several networks have the same name, so this Flavor name could point at either of them. Use the device's stable name, which includes the network ID.",
        };
      }
      const count = new Set(tied.map((c) => c.network?.id)).size;
      if (r.decidedBy === MatchKind.SUBNET_ROUTE) {
        return {
          title: `${r.normalized} is routed by ${count} networks`,
          detail: `Each one advertises a route of the same length that covers this address, so Flavor will not pick one on its own.`,
        };
      }
      if (r.decidedBy === MatchKind.EXIT_NODE) {
        return {
          title: `${r.normalized} could leave through ${count} exit nodes`,
          detail: `Each of those networks has an exit node selected, so Flavor will not pick one on its own.`,
        };
      }
      const where = count > 1 ? `${count} networks` : "one network, on more than one device";
      return {
        title: `${r.normalized} exists on ${where}`,
        detail:
          count > 1
            ? `Each network has its own device with this ${what(r)}. Flavor keeps them separate; a full DNS name always points at exactly one of them.`
            : `More than one device on this network reports this ${what(r)}.`,
      };
    }
    default:
      return {
        title: `Nothing on your connected networks matches ${r.normalized}`,
        detail:
          r.notInspected.length > 0
            ? `${r.notInspected.map((n) => n.displayName).join(", ")} ${r.notInspected.length === 1 ? "is" : "are"} not connected, so ${r.notInspected.length === 1 ? "its" : "their"} devices could not be checked.`
            : `No device address, DNS name or device name matched.`,
      };
  }
}

export function preferenceNote(r: InspectDestinationResponse): string | null {
  const p = r.preference;
  if (!p || p.state === PreferenceState.APPLIED) return null;
  const net = p.network?.displayName || "the preferred network";
  return p.state === PreferenceState.NETWORK_NOT_CONNECTED
    ? `Your preference for ${net} is not applied because ${net} is not connected.`
    : `Your preference for ${net} is not applied because no device on ${net} matches ${r.normalized}.`;
}

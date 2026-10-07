import { CandidateStatus, DestinationKind, MatchKind, ResolutionDecision, type InspectDestinationResponse } from "@gen/lattice/v1/inspector_pb";

export const matchLabel: Record<MatchKind, string> = {
  [MatchKind.UNSPECIFIED]: "Match",
  [MatchKind.DEVICE_ADDRESS]: "Exact device address",
  [MatchKind.DEVICE_DNS_NAME]: "Full DNS name",
  [MatchKind.DEVICE_HOSTNAME]: "Device name",
};

export const statusLabel: Record<CandidateStatus, string> = {
  [CandidateStatus.UNSPECIFIED]: "",
  [CandidateStatus.SELECTED]: "Selected",
  [CandidateStatus.TIED]: "Equal match",
  [CandidateStatus.OUTRANKED]: "Weaker match",
};

const basis: Record<MatchKind, string> = {
  [MatchKind.UNSPECIFIED]: "a match",
  [MatchKind.DEVICE_ADDRESS]: "an exact device address",
  [MatchKind.DEVICE_DNS_NAME]: "a full DNS name",
  [MatchKind.DEVICE_HOSTNAME]: "a device name",
};

const what = (r: InspectDestinationResponse) => (r.kind === DestinationKind.ADDRESS ? "address" : "name");

export function explain(r: InspectDestinationResponse): { title: string; detail: string } {
  const tied = r.candidates.filter((c) => c.status === CandidateStatus.TIED);
  const selected = r.candidates.find((c) => c.status === CandidateStatus.SELECTED);
  const outranked = r.candidates.filter((c) => c.status === CandidateStatus.OUTRANKED);
  switch (r.decision) {
    case ResolutionDecision.UNIQUE: {
      const device = selected?.device?.hostname || selected?.device?.dnsName || "one device";
      const network = selected?.network?.displayName ?? "one network";
      const by = basis[r.decidedBy];
      const others =
        outranked.length > 0
          ? ` ${outranked.length} weaker ${outranked.length === 1 ? "match was" : "matches were"} set aside because ${by} is more specific.`
          : "";
      return { title: `${device} on ${network}`, detail: `Matched by ${by}.${others}` };
    }
    case ResolutionDecision.AMBIGUOUS: {
      const count = new Set(tied.map((c) => c.network?.id)).size;
      const where = count > 1 ? `${count} networks` : "one network, on more than one device";
      return {
        title: `${r.normalized} exists on ${where}`,
        detail:
          count > 1
            ? `Each network has its own device with this ${what(r)}. Lattice keeps them separate; a full DNS name always points at exactly one of them.`
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

import { create } from "@bufbuild/protobuf";
import {
  CandidateStatus,
  DecisionReason,
  DestinationKind,
  PreferenceState,
  InspectDestinationResponseSchema,
  MatchKind,
  ResolutionDecision,
} from "@gen/lattice/v1/inspector_pb";
import { describe, expect, it } from "vitest";
import { candidateLabel, explain, preferenceNote } from "./explain";

const candidate = (networkId: string, network: string, host: string, status: CandidateStatus, match = MatchKind.DEVICE_ADDRESS) => ({
  network: { id: networkId, displayName: network },
  device: { id: { networkId, nodeId: host }, hostname: host, addresses: ["100.64.0.1"] },
  match,
  status,
});

describe("explain", () => {
  it("describes duplicate addresses across networks as expected, not as an error", () => {
    const r = create(InspectDestinationResponseSchema, {
      normalized: "100.64.0.1",
      kind: DestinationKind.ADDRESS,
      decision: ResolutionDecision.AMBIGUOUS,
      decidedBy: MatchKind.DEVICE_ADDRESS,
      candidates: [candidate("b", "Home", "desktop", CandidateStatus.TIED), candidate("a", "LunarLabs", "prod-api", CandidateStatus.TIED)],
    });
    const e = explain(r);
    expect(e.title).toBe("100.64.0.1 exists on 2 networks");
    expect(e.detail).toMatch(/keeps them separate/);
    expect(`${e.title} ${e.detail}`).not.toMatch(/conflict|error|critical/i);
  });

  it("names the selected device and network and explains outranked matches", () => {
    const r = create(InspectDestinationResponseSchema, {
      normalized: "db.home.ts.net",
      kind: DestinationKind.NAME,
      decision: ResolutionDecision.UNIQUE,
      decidedBy: MatchKind.DEVICE_DNS_NAME,
      candidates: [
        candidate("b", "Home", "postgres", CandidateStatus.SELECTED, MatchKind.DEVICE_DNS_NAME),
        candidate("c", "Customer", "db.home.ts.net", CandidateStatus.OUTRANKED, MatchKind.DEVICE_HOSTNAME),
      ],
    });
    expect(explain(r)).toEqual({
      title: "postgres on Home",
      detail: "Matched by a full DNS name. 1 weaker match was set aside because a full DNS name is more specific.",
    });
  });

  it("explains a miss and names networks that could not be checked", () => {
    const r = create(InspectDestinationResponseSchema, {
      normalized: "10.10.20.15",
      decision: ResolutionDecision.NO_MATCH,
      notInspected: [{ id: "w", displayName: "Work" }],
    });
    expect(explain(r).detail).toBe("Work is not connected, so its devices could not be checked.");
  });

  it("flags two devices in one network sharing an address", () => {
    const r = create(InspectDestinationResponseSchema, {
      normalized: "100.64.0.1",
      kind: DestinationKind.ADDRESS,
      decision: ResolutionDecision.AMBIGUOUS,
      candidates: [candidate("a", "LunarLabs", "x", CandidateStatus.TIED), candidate("a", "LunarLabs", "y", CandidateStatus.TIED)],
    });
    expect(explain(r).title).toBe("100.64.0.1 exists on one network, on more than one device");
  });

  it("explains a decision made by a preference, and a preference that could not apply", () => {
    const chosen = create(InspectDestinationResponseSchema, {
      normalized: "100.64.0.1",
      kind: DestinationKind.ADDRESS,
      decision: ResolutionDecision.UNIQUE,
      reason: DecisionReason.DESTINATION_PREFERENCE,
      candidates: [candidate("a", "LunarLabs", "prod-api", CandidateStatus.SELECTED), candidate("b", "Home", "desktop", CandidateStatus.OUTRANKED)],
      preference: { destination: "100.64.0.1", network: { id: "a", displayName: "LunarLabs" }, state: PreferenceState.APPLIED },
    });
    expect(explain(chosen)).toEqual({
      title: "prod-api on LunarLabs",
      detail: "Chosen by your Lattice preference for LunarLabs. Without it, 100.64.0.1 would match on 2 networks.",
    });
    expect(preferenceNote(chosen)).toBeNull();
    expect(candidateLabel(chosen, CandidateStatus.OUTRANKED)).toBe("Not preferred");
    const offline = create(InspectDestinationResponseSchema, {
      normalized: "100.64.0.1",
      preference: { network: { id: "a", displayName: "LunarLabs" }, state: PreferenceState.NETWORK_NOT_CONNECTED },
    });
    expect(preferenceNote(offline)).toBe("Your preference for LunarLabs is not applied because LunarLabs is not connected.");
  });
});


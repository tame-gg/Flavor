import { create } from "@bufbuild/protobuf";
import {
  CandidateStatus,
  DecisionReason,
  DestinationKind,
  PreferenceState,
  InspectDestinationResponseSchema,
  MatchKind,
  ResolutionDecision,
} from "@gen/flavor/v1/inspector_pb";
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
      detail: "Chosen by your Flavor preference for LunarLabs. Without it, 100.64.0.1 would match on 2 networks.",
    });
    expect(preferenceNote(chosen)).toBeNull();
    expect(candidateLabel(chosen, CandidateStatus.OUTRANKED)).toBe("Not preferred");
    const offline = create(InspectDestinationResponseSchema, {
      normalized: "100.64.0.1",
      preference: { network: { id: "a", displayName: "LunarLabs" }, state: PreferenceState.NETWORK_NOT_CONNECTED },
    });
    expect(preferenceNote(offline)).toBe("Your preference for LunarLabs is not applied because LunarLabs is not connected.");
  });

  it("explains subnet routes and the longest-prefix rule", () => {
    const routed = create(InspectDestinationResponseSchema, {
      normalized: "10.20.5.12",
      kind: DestinationKind.ADDRESS,
      decision: ResolutionDecision.UNIQUE,
      reason: DecisionReason.LONGEST_PREFIX,
      decidedBy: MatchKind.SUBNET_ROUTE,
      candidates: [
        { ...candidate("b", "Customer", "vpn", CandidateStatus.SELECTED, MatchKind.SUBNET_ROUTE), matchedValue: "10.20.0.0/16" },
        { ...candidate("a", "Company", "edge", CandidateStatus.OUTRANKED, MatchKind.SUBNET_ROUTE), matchedValue: "10.0.0.0/8" },
      ],
    });
    expect(explain(routed)).toEqual({
      title: "Customer, via vpn",
      detail: "vpn routes 10.20.0.0/16 on Customer. It is more specific than the other matching route, so it wins.",
    });
    const tie = create(InspectDestinationResponseSchema, {
      normalized: "10.10.1.1",
      kind: DestinationKind.ADDRESS,
      decision: ResolutionDecision.AMBIGUOUS,
      decidedBy: MatchKind.SUBNET_ROUTE,
      candidates: [candidate("a", "A", "x", CandidateStatus.TIED, MatchKind.SUBNET_ROUTE), candidate("b", "B", "y", CandidateStatus.TIED, MatchKind.SUBNET_ROUTE)],
    });
    expect(explain(tie).title).toBe("10.10.1.1 is routed by 2 networks");
    expect(candidateLabel(routed, CandidateStatus.OUTRANKED, MatchKind.SUBNET_ROUTE)).toBe("Less specific route");
  });

  it("explains a DNS record, with and without an owning device", () => {
    const owned = create(InspectDestinationResponseSchema, {
      normalized: "grafana.corp.example",
      kind: DestinationKind.NAME,
      decision: ResolutionDecision.UNIQUE,
      reason: DecisionReason.DNS_RECORD,
      decidedBy: MatchKind.DNS_RECORD,
      candidates: [{ ...candidate("a", "LunarLabs", "monitoring", CandidateStatus.SELECTED, MatchKind.DNS_RECORD), matchedValue: "100.64.0.1" }],
    });
    expect(explain(owned)).toEqual({
      title: "monitoring on LunarLabs",
      detail: "Matched by a DNS record the network publishes.",
    });
    const bare = create(InspectDestinationResponseSchema, {
      normalized: "grafana.corp.example",
      kind: DestinationKind.NAME,
      decision: ResolutionDecision.UNIQUE,
      reason: DecisionReason.DNS_RECORD,
      decidedBy: MatchKind.DNS_RECORD,
      candidates: [
        {
          network: { id: "a", displayName: "LunarLabs" },
          device: {},
          match: MatchKind.DNS_RECORD,
          matchedValue: "10.0.0.5",
          status: CandidateStatus.SELECTED,
        },
      ],
    });
    expect(explain(bare).title).toBe("10.0.0.5 on LunarLabs");
  });

  it("explains an exit node, unique and ambiguous", () => {
    const unique = create(InspectDestinationResponseSchema, {
      normalized: "203.0.113.7",
      kind: DestinationKind.ADDRESS,
      decision: ResolutionDecision.UNIQUE,
      reason: DecisionReason.EXIT_NODE,
      decidedBy: MatchKind.EXIT_NODE,
      candidates: [candidate("a", "LunarLabs", "gateway", CandidateStatus.SELECTED, MatchKind.EXIT_NODE)],
    });
    expect(explain(unique)).toEqual({
      title: "gateway on LunarLabs",
      detail: "203.0.113.7 is not on any of your networks, so it goes through the exit node chosen for LunarLabs.",
    });
    const tie = create(InspectDestinationResponseSchema, {
      normalized: "203.0.113.7",
      kind: DestinationKind.ADDRESS,
      decision: ResolutionDecision.AMBIGUOUS,
      reason: DecisionReason.MULTIPLE_MATCHES,
      decidedBy: MatchKind.EXIT_NODE,
      candidates: [candidate("a", "A", "x", CandidateStatus.TIED, MatchKind.EXIT_NODE), candidate("b", "B", "y", CandidateStatus.TIED, MatchKind.EXIT_NODE)],
    });
    expect(explain(tie).title).toBe("203.0.113.7 could leave through 2 exit nodes");
  });

  it("explains a Flavor name whose network label is shared", () => {
    const r = create(InspectDestinationResponseSchema, {
      normalized: "postgres.home.flavor.internal",
      kind: DestinationKind.NAME,
      decision: ResolutionDecision.AMBIGUOUS,
      reason: DecisionReason.AMBIGUOUS_NETWORK_LABEL,
      candidates: [candidate("b", "Home", "postgres", CandidateStatus.TIED, MatchKind.QUALIFIED_NAME)],
    });
    expect(explain(r).title).toBe("postgres.home.flavor.internal uses a network name more than one network shares");
  });
});


import { create } from "@bufbuild/protobuf";
import { ConflictSchema, ConflictScope, ConflictSeverity, ConflictType } from "@gen/lattice/v1/conflicts_pb";
import { describe as suite, expect, it } from "vitest";
import { counts, describe, filterConflicts } from "./describe";

const member = (networkId: string, network: string, nodeId: string) => ({
  network: { id: networkId, displayName: network },
  device: { id: { networkId, nodeId }, hostname: nodeId, addresses: ["100.64.0.1"] },
});

const expected = create(ConflictSchema, {
  id: "address:100.64.0.1",
  type: ConflictType.ADDRESS_COLLISION,
  severity: ConflictSeverity.EXPECTED,
  scope: ConflictScope.CROSS_NETWORK,
  value: "100.64.0.1",
  networkContextResolves: true,
  members: [member("a", "LunarLabs", "prod-api"), member("b", "Home", "desktop"), member("c", "Customer", "app")],
});

const within = create(ConflictSchema, {
  id: "address:100.64.0.7",
  type: ConflictType.ADDRESS_COLLISION,
  severity: ConflictSeverity.AMBIGUOUS,
  scope: ConflictScope.WITHIN_NETWORK,
  value: "100.64.0.7",
  members: [member("a", "LunarLabs", "x"), member("a", "LunarLabs", "y")],
});

const dns = create(ConflictSchema, {
  id: "dns:db.example.com",
  type: ConflictType.DNS_NAME_COLLISION,
  severity: ConflictSeverity.AMBIGUOUS,
  scope: ConflictScope.CROSS_NETWORK,
  value: "db.example.com",
  members: [member("a", "A", "db"), member("b", "B", "db")],
});

suite("conflict descriptions", () => {
  it("presents an address shared by three networks as an expected overlap", () => {
    const text = describe(expected);
    expect(text).toBe(
      "100.64.0.1 exists on 3 networks. Each device also has its own full DNS name, so it can always be reached unambiguously.",
    );
    expect(text).not.toMatch(/error|critical|broken/i);
  });

  it("explains a shared address inside one network and a colliding DNS name", () => {
    expect(describe(within)).toBe("2 devices on LunarLabs report 100.64.0.7.");
    expect(describe(dns)).toMatch(/cannot pick one/);
  });

  it("filters by severity and type and counts each", () => {
    const all = [expected, within, dns];
    expect(filterConflicts(all, "ambiguous", "all").map((c) => c.id)).toEqual(["address:100.64.0.7", "dns:db.example.com"]);
    expect(filterConflicts(all, "all", ConflictType.DNS_NAME_COLLISION)).toHaveLength(1);
    expect(filterConflicts(all, "expected", ConflictType.DNS_NAME_COLLISION)).toHaveLength(0);
    expect(counts(all)).toEqual({ ambiguous: 2, expected: 1, resolved: 0 });
  });

  it("moves a conflict settled by a preference into its own category", () => {
    const settled = create(ConflictSchema, { ...dns, preferredNetworkId: "a" });
    expect(counts([expected, settled])).toEqual({ ambiguous: 0, expected: 1, resolved: 1 });
    expect(filterConflicts([expected, settled], "resolved", "all")).toEqual([settled]);
    expect(filterConflicts([expected, settled], "ambiguous", "all")).toEqual([]);
  });
});

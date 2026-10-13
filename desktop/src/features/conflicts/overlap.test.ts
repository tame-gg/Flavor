import { create } from "@bufbuild/protobuf";
import { ConflictSchema, ConflictScope, ConflictSeverity, ConflictType } from "@gen/flavor/v1/conflicts_pb";
import { describe, expect, it } from "vitest";
import { heat, overlapMatrix, shares } from "./overlap";

const member = (networkId: string, network: string, nodeId: string) => ({
  network: { id: networkId, displayName: network },
  device: { id: { networkId, nodeId }, hostname: nodeId, addresses: ["100.64.0.1"] },
});

const conflict = (value: string, scope: ConflictScope, members: ReturnType<typeof member>[]) =>
  create(ConflictSchema, {
    id: `address:${value}`,
    type: ConflictType.ADDRESS_COLLISION,
    severity: ConflictSeverity.AMBIGUOUS,
    scope,
    value,
    members,
  });

const everywhere = conflict("100.64.0.14", ConflictScope.CROSS_NETWORK, [
  member("l", "LunarLabs", "1"),
  member("d", "Darkless", "2"),
  member("p", "PlutoFlavor", "3"),
]);
const pair = conflict("100.64.0.20", ConflictScope.CROSS_NETWORK, [
  member("l", "LunarLabs", "4"),
  member("d", "Darkless", "5"),
]);
const inside = conflict("100.64.0.7", ConflictScope.WITHIN_NETWORK, [
  member("l", "LunarLabs", "6"),
  member("l", "LunarLabs", "7"),
]);

describe("overlap matrix", () => {
  it("counts the conflicts each pair of networks shares, with conflicts inside one network on the diagonal", () => {
    const m = overlapMatrix([everywhere, pair, inside]);
    expect(m.networks.map((n) => n.name)).toEqual(["Darkless", "LunarLabs", "PlutoFlavor"]);
    expect(m.counts).toEqual([
      [0, 2, 1],
      [2, 1, 1],
      [1, 1, 0],
    ]);
    expect(m.max).toBe(2);
  });

  it("is empty without conflicts", () => {
    expect(overlapMatrix([])).toEqual({ networks: [], counts: [], max: 0 });
  });

  it("matches a conflict to a cell in either order", () => {
    expect(shares(pair, "l", "d")).toBe(true);
    expect(shares(pair, "d", "l")).toBe(true);
    expect(shares(pair, "l", "p")).toBe(false);
    expect(shares(pair, "l", "l")).toBe(false);
    expect(shares(inside, "l", "l")).toBe(true);
  });

  it("scales the cell shade with the count", () => {
    expect([0, 1, 2, 3].map((n) => heat(n, 3))).toEqual([0, 1, 2, 3]);
    expect(heat(1, 1)).toBe(3);
    expect(heat(1, 10)).toBe(1);
    expect(heat(0, 0)).toBe(0);
  });
});

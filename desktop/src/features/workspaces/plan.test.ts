import { create } from "@bufbuild/protobuf";
import { NetworkConnectionState as S } from "@gen/lattice/v1/common_pb";
import { NetworkSchema } from "@gen/lattice/v1/network_pb";
import { WorkspaceSchema } from "@gen/lattice/v1/workspaces_pb";
import { describe, expect, it } from "vitest";
import { activationPlan, memberSummary } from "./plan";

const net = (id: string, name: string, state: S) => create(NetworkSchema, { id, displayName: name, state });
const networks = new Map([
  ["a", net("a", "LunarLabs", S.DISCONNECTED)],
  ["m", net("m", "Monitoring", S.CONNECTED)],
  ["h", net("h", "Home", S.CONNECTED)],
  ["c", net("c", "Customer", S.DISCONNECTED)],
]);
const onCall = create(WorkspaceSchema, { id: "w", name: "On Call", networkIds: ["a", "m"] });

describe("workspace activation plan", () => {
  it("splits members into to-connect and already-up, and lists running outsiders", () => {
    const plan = activationPlan(onCall, networks);
    expect(plan.connect.map((n) => n.id)).toEqual(["a"]);
    expect(plan.alreadyUp.map((n) => n.id)).toEqual(["m"]);
    expect(plan.outside.map((n) => n.id)).toEqual(["h"]);
  });

  it("summarises member connectivity and ignores members that no longer exist", () => {
    expect(memberSummary(onCall, networks)).toBe("1 of 2 networks connected");
    const stale = create(WorkspaceSchema, { id: "x", networkIds: ["gone"] });
    expect(memberSummary(stale, networks)).toBe("No networks");
  });
});

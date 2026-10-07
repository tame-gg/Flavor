import { create } from "@bufbuild/protobuf";
import { DeviceSchema, type Device } from "@gen/lattice/v1/device_pb";
import { NetworkSchema } from "@gen/lattice/v1/network_pb";
import { describe, expect, it } from "vitest";
import { deviceKey } from "../../app/sync/controller";
import { deviceRows } from "./filter";

const networks = new Map([
  ["a", create(NetworkSchema, { id: "a", displayName: "Office" })],
  ["b", create(NetworkSchema, { id: "b", displayName: "Home" })],
]);

function devices(perNetwork: number) {
  const m = new Map<string, Device>();
  for (const net of ["a", "b"]) {
    for (let i = 0; i < perNetwork; i++) {
      const nodeId = `n${i}`;
      m.set(deviceKey(net, nodeId), create(DeviceSchema, { id: { networkId: net, nodeId }, hostname: `host-${i}`, addresses: [`100.64.${i >> 8}.${i & 255}`] }));
    }
  }
  return m;
}

describe("deviceRows", () => {
  it("keeps the same address on two networks as two rows with distinct keys", () => {
    const rows = deviceRows(devices(1), networks, "", "100.64.0.0");
    expect(rows.map((r) => r.key).sort()).toEqual(["a:n0", "b:n0"]);
    expect(new Set(rows.map((r) => r.networkName))).toEqual(new Set(["Office", "Home"]));
  });

  it("filters by network and by search text", () => {
    const all = devices(3);
    expect(deviceRows(all, networks, "b", "")).toHaveLength(3);
    expect(deviceRows(all, networks, "", "host-2")).toHaveLength(2);
    expect(deviceRows(all, networks, "", "home")).toHaveLength(3);
  });

  it("handles a thousand synthetic devices quickly", () => {
    const all = devices(500);
    const start = performance.now();
    const rows = deviceRows(all, networks, "", "");
    const filtered = deviceRows(all, networks, "", "host-49");
    expect(rows).toHaveLength(1000);
    expect(new Set(rows.map((r) => r.key)).size).toBe(1000);
    expect(filtered.length).toBeGreaterThan(0);
    expect(performance.now() - start).toBeLessThan(200);
  });
});

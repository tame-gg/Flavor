import { create } from "@bufbuild/protobuf";
import { ProviderType } from "@gen/flavor/v1/common_pb";
import { DeviceSchema, type Device } from "@gen/flavor/v1/device_pb";
import { NetworkSchema } from "@gen/flavor/v1/network_pb";
import { describe, expect, it } from "vitest";
import { deviceKey } from "../../app/sync/controller";
import { deviceRows, parseSearch } from "./filter";

const networks = new Map([
  ["a", create(NetworkSchema, { id: "a", displayName: "LunarLabs", provider: ProviderType.HEADSCALE })],
  ["b", create(NetworkSchema, { id: "b", displayName: "Home", provider: ProviderType.TAILSCALE })],
]);

const dev = (networkId: string, nodeId: string, init: { hostname?: string; dnsName?: string; addresses?: string[]; os?: string; tags?: string[]; online?: boolean; local?: boolean },
) =>
  create(DeviceSchema, { id: { networkId, nodeId }, online: true, ...init });

const fixture = new Map<string, Device>(
  [
    dev("a", "1", { hostname: "prod-api", dnsName: "prod-api.lunar.ts.net", addresses: ["100.64.0.1"], os: "linux", tags: ["tag:prod"] }),
    dev("a", "2", { hostname: "postgres", dnsName: "postgres.lunar.ts.net", addresses: ["100.64.0.2"], os: "linux", tags: ["tag:db"] }),
    dev("a", "3", { hostname: "postgres-replica", addresses: ["100.64.0.3"], online: false }),
    dev("b", "1", { hostname: "desktop", dnsName: "desktop.home.ts.net", addresses: ["100.64.0.1"], os: "windows", local: true }),
    dev("b", "2", { hostname: "ubuntu-box", addresses: ["100.64.0.9"], os: "linux" }),
  ].map((d) => [deviceKey(d.id!.networkId, d.id!.nodeId), d]),
);

const keys = (q: string, networkId = "") => deviceRows(fixture, networks, networkId, q).map((r) => r.key);

describe("device search", () => {
  it("shows both devices when two networks share an address", () => {
    expect(keys("100.64.0.1").sort()).toEqual(["a:1", "b:1"]);
  });

  it("finds devices by partial name, network name and provider", () => {
    expect(keys("prod")).toEqual(["a:1"]);
    expect(keys("lunarlabs")).toHaveLength(3);
    expect(keys("tailscale").sort()).toEqual(["b:1", "b:2"]);
    expect(keys("ubuntu")).toEqual(["b:2"]);
  });

  it("ranks an exact name above partial matches", () => {
    const withMirror = new Map(fixture);
    withMirror.set("a:9", dev("a", "9", { hostname: "a-postgres-mirror", addresses: ["100.64.0.20"] }));
    expect(deviceRows(withMirror, networks, "", "postgres").map((r) => r.key)).toEqual(["a:2", "a:3", "a:9"]);
  });

  it("combines tokens and qualifiers", () => {
    expect(keys("is:offline")).toEqual(["a:3"]);
    expect(keys("is:local")).toEqual(["b:1"]);
    expect(keys("os:linux network:lunar")).toEqual(["a:2", "a:1"]);
    expect(keys("tag:db")).toEqual(["a:2"]);
    expect(keys("tag:prod postgres")).toEqual([]);
    expect(keys("node:2").sort()).toEqual(["a:2", "b:2"]);
  });

  it("treats IPv6 addresses with colons as plain text, not qualifiers", () => {
    expect(parseSearch("fd7a:115c::1")).toEqual([{ field: "text", value: "fd7a:115c::1" }]);
    expect(parseSearch("tag:")).toEqual([{ field: "text", value: "tag:" }]);
  });

  it("respects the network filter and never keys a row by address", () => {
    expect(keys("", "b").sort()).toEqual(["b:1", "b:2"]);
    const rows = deviceRows(fixture, networks, "", "");
    expect(new Set(rows.map((r) => r.key)).size).toBe(rows.length);
    expect(rows.every((r) => !r.key.includes("100.64"))).toBe(true);
  });

  it("handles a thousand synthetic devices quickly", () => {
    const many = new Map<string, Device>();
    for (const net of ["a", "b"]) {
      for (let i = 0; i < 500; i++) {
        many.set(deviceKey(net, `n${i}`), dev(net, `n${i}`, { hostname: `host-${i}`, addresses: [`100.64.${i >> 8}.${i & 255}`] }));
      }
    }
    const start = performance.now();
    expect(deviceRows(many, networks, "", "")).toHaveLength(1000);
    expect(deviceRows(many, networks, "", "host-49 is:online").length).toBeGreaterThan(0);
    expect(performance.now() - start).toBeLessThan(250);
  });
});

import { create } from "@bufbuild/protobuf";
import { DeviceSchema } from "@gen/flavor/v1/device_pb";
import { describe, expect, it } from "vitest";
import { exampleDestinations } from "./suggest";

const device = (networkId: string, host: string, ip: string, extra: { local?: boolean; online?: boolean; routes?: string[] } = {}) =>
  create(DeviceSchema, {
    id: { networkId, nodeId: host },
    hostname: host,
    dnsName: `${host}.${networkId}.ts.net`,
    addresses: [ip],
    online: extra.online ?? true,
    local: extra.local ?? false,
    routes: extra.routes ?? [],
  });

describe("exampleDestinations", () => {
  it("picks a shared address, a DNS name, a device name and an address inside a subnet route", () => {
    const examples = exampleDestinations([
      device("a", "me", "100.64.0.1", { local: true }),
      device("a", "db", "100.64.0.14"),
      device("b", "app", "100.64.0.14", { routes: ["0.0.0.0/0", "10.20.0.0/24"] }),
      device("c", "nas", "100.70.0.2", { online: false }),
    ]);
    expect(examples).toEqual([
      { value: "100.64.0.14", hint: "Used on 2 networks", networkIds: ["a", "b"] },
      { value: "db.a.ts.net", hint: "Full DNS name", networkIds: ["a"] },
      { value: "app", hint: "Device name", networkIds: ["b"] },
      { value: "10.20.0.1", hint: "In 10.20.0.0/24, routed by app", networkIds: ["b"] },
    ]);
  });

  it("ignores this computer and returns nothing without other devices", () => {
    expect(exampleDestinations([device("a", "me", "100.64.0.1", { local: true })])).toEqual([]);
  });
});

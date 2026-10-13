import { create } from "@bufbuild/protobuf";
import { NetworkConnectionState as S } from "@gen/flavor/v1/common_pb";
import { DeviceSchema } from "@gen/flavor/v1/device_pb";
import { NetworkSchema } from "@gen/flavor/v1/network_pb";
import { describe, expect, it } from "vitest";
import { summarize } from "./summary";

const net = (id: string, state: S, extra: Partial<{ statusMessage: string; authentication: { kind: string } }> = {}) =>
  create(NetworkSchema, { id, displayName: id, state, ...extra });
const dev = (online: boolean, local = false) => create(DeviceSchema, { online, local });

describe("summarize", () => {
  it("is calm when every network is healthy", () => {
    const o = summarize(
      [net("Lab", S.CONNECTED), net("Home", S.CONNECTED)],
      [dev(true), dev(false), dev(true, true)],
      0,
    );
    expect(o).toMatchObject({
      tone: "ok",
      headline: "2 of 2 networks connected",
      connected: 2,
      devicesOnline: 1,
      devicesTotal: 2,
    });
    expect(o.needsYou).toEqual([]);
  });

  it("lists what needs the user with the daemon's reason", () => {
    const o = summarize(
      [
        net("Work", S.DEGRADED, { statusMessage: "DERP region fra is unreachable" }),
        net("Home", S.AUTHENTICATING, { authentication: { kind: "authentication" } }),
        net("Lab", S.ERROR, { statusMessage: "control server unreachable" }),
        net("Office", S.AWAITING_APPROVAL),
      ],
      [],
      2,
    );
    expect(o.tone).toBe("danger");
    expect(o.headline).toBe("1 of 4 networks connected · Home needs attention");
    expect(o.needsYou.map((i) => [i.key, i.detail])).toEqual([
      ["signin:Home", "needs you to sign in"],
      ["error:Lab", "control server unreachable"],
      ["approval:Office", "is waiting for an administrator to approve this device"],
      ["degraded:Work", "DERP region fra is unreachable"],
      ["conflicts", "exist on more than one network"],
    ]);
  });

  it("handles no networks", () => {
    expect(summarize([], [], 0)).toMatchObject({ tone: "neutral", headline: "No networks yet", total: 0 });
  });
});

import { create } from "@bufbuild/protobuf";
import { describe, expect, it } from "vitest";
import { NetworkConnectionState as S, ProviderType } from "@gen/flavor/v1/common_pb";
import { NetworkSchema } from "@gen/flavor/v1/network_pb";
import { controlHost, deviceSummary, isActive, isLive, networkSource, stateTone } from "./format";

describe("network formatting", () => {
  it("names the control server", () => {
    expect(controlHost(create(NetworkSchema, {}))).toBe("Tailscale (default)");
    expect(controlHost(create(NetworkSchema, { controlUrl: "https://hs.example.com:8443/path" }))).toBe(
      "hs.example.com:8443",
    );
    expect(controlHost(create(NetworkSchema, { controlUrl: "not a url" }))).toBe("not a url");
  });

  it("describes where a network comes from without repeating the provider", () => {
    expect(networkSource(create(NetworkSchema, { provider: ProviderType.TAILSCALE }))).toBe("Tailscale");
    expect(
      networkSource(create(NetworkSchema, { provider: ProviderType.HEADSCALE, controlUrl: "https://hs.home.example" })),
    ).toBe("Headscale · hs.home.example");
  });

  it("maps states to tones and activity", () => {
    expect(stateTone(S.CONNECTED)).toBe("ok");
    expect(stateTone(S.DEGRADED)).toBe("warn");
    expect(stateTone(S.ERROR)).toBe("danger");
    expect(stateTone(S.RECONNECTING)).toBe("info");
    expect(stateTone(S.DISCONNECTED)).toBe("neutral");
    expect(isActive(S.AUTHENTICATING)).toBe(true);
    expect(isActive(S.ERROR)).toBe(false);
    expect(isLive(S.DEGRADED)).toBe(true);
    expect(isLive(S.RECONNECTING)).toBe(false);
  });

  it("summarises devices", () => {
    expect(deviceSummary(1, true)).toBe("1 device on this network, including this one.");
    expect(deviceSummary(4, false)).toBe("4 devices on this network, including this one.");
    expect(deviceSummary(0, true)).toBe("No devices reported yet.");
    expect(deviceSummary(0, false)).toBe("Connect to see devices on this network.");
  });
});

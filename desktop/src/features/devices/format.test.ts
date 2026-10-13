import { describe, expect, it } from "vitest";
import { canRetryDescribe, deviceBadges, lastSeen, sshTarget } from "./format";

const now = new Date("2026-10-07T12:00:00Z");

describe("device formatting", () => {
  it("formats last seen relative to now", () => {
    expect(lastSeen(undefined, now)).toBeNull();
    expect(lastSeen(new Date(0), now)).toBeNull();
    expect(lastSeen(new Date("2026-10-07T11:59:30Z"), now)).toBe("just now");
    expect(lastSeen(new Date("2026-10-07T11:59:00Z"), now)).toBe("1 minute ago");
    expect(lastSeen(new Date("2026-10-07T09:00:00Z"), now)).toBe("3 hours ago");
    expect(lastSeen(new Date("2026-10-01T12:00:00Z"), now)).toBe("6 days ago");
  });

  it("labels exit nodes, routers and this device", () => {
    const base = { local: false, exitNode: false, exitNodeOption: false, routes: [] };
    expect(deviceBadges(base)).toEqual([]);
    expect(deviceBadges({ ...base, exitNodeOption: true }).map((b) => b.label)).toEqual(["Exit node"]);
    expect(deviceBadges({ ...base, exitNodeOption: true, exitNode: true })).toEqual([
      { label: "Exit node in use", active: true },
    ]);
    expect(deviceBadges({ ...base, local: true, routes: ["10.0.0.0/24"] }).map((b) => b.label)).toEqual([
      "This device",
      "Router",
    ]);
  });

  it("prefers the DNS name for SSH and falls back to the first address", () => {
    expect(sshTarget("postgres.lunar.ts.net.", ["100.64.0.2"])).toBe("ssh postgres.lunar.ts.net");
    expect(sshTarget("", ["100.64.0.2"])).toBe("ssh 100.64.0.2");
    expect(sshTarget("", [])).toBeNull();
  });

  it("offers a retry only when a describe failure could succeed", () => {
    const internal = { kind: "daemon", code: "FLAVOR_ERROR_CODE_INTERNAL", message: "", retryable: false };
    const missing = { ...internal, code: "FLAVOR_ERROR_CODE_DEVICE_NOT_FOUND" };
    expect(canRetryDescribe(internal, "ready")).toBe(true);
    expect(canRetryDescribe(new Error("boom"), "ready")).toBe(true);
    expect(canRetryDescribe(missing, "ready")).toBe(false);
    expect(canRetryDescribe(internal, "unavailable")).toBe(false);
    expect(canRetryDescribe(internal, "resyncing")).toBe(false);
  });
});

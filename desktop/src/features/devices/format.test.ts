import { describe, expect, it } from "vitest";
import { lastSeen, sshTarget } from "./format";

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

  it("prefers the DNS name for SSH and falls back to the first address", () => {
    expect(sshTarget("postgres.lunar.ts.net.", ["100.64.0.2"])).toBe("ssh postgres.lunar.ts.net");
    expect(sshTarget("", ["100.64.0.2"])).toBe("ssh 100.64.0.2");
    expect(sshTarget("", [])).toBeNull();
  });
});

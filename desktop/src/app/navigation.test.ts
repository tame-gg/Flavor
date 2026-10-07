import { Capability } from "@gen/lattice/v1/common_pb";
import { describe, expect, it } from "vitest";
import { visiblePages } from "./navigation";

describe("visiblePages", () => {
  it("hides the Connection Inspector when the daemon does not support it", () => {
    expect(visiblePages([Capability.HEADSCALE]).map(([id]) => id)).toEqual(["networks", "devices", "diagnostics", "settings"]);
  });

  it("shows it once the daemon advertises the capability", () => {
    expect(visiblePages([Capability.CONNECTION_INSPECTOR]).map(([id]) => id)).toContain("inspector");
  });

  it("shows Workspaces only when supported", () => {
    expect(visiblePages([]).map(([id]) => id)).not.toContain("workspaces");
    expect(visiblePages([Capability.WORKSPACES]).map(([id]) => id)).toContain("workspaces");
  });

  it("gates the Conflict Center separately", () => {
    expect(visiblePages([Capability.CONNECTION_INSPECTOR]).map(([id]) => id)).not.toContain("conflicts");
    expect(visiblePages([Capability.CONFLICT_CENTER]).map(([id]) => id)).toContain("conflicts");
  });
});

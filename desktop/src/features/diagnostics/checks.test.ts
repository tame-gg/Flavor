import { create } from "@bufbuild/protobuf";
import { RunDiagnosticsResponseSchema } from "@gen/flavor/v1/diagnostics_pb";
import { describe, expect, it } from "vitest";
import { checkLabel, checkTone, detailParts, keyringLabel, statusLabel, systemChecks } from "./checks";

describe("checkTone", () => {
  it("maps both warning spellings and leaves anything else neutral", () => {
    expect(["ok", "warning", "warn", "error", "", "skipped"].map(checkTone)).toEqual([
      "ok",
      "warn",
      "warn",
      "error",
      "unknown",
      "unknown",
    ]);
    expect(["warn", "", "skipped"].map(statusLabel)).toEqual(["Warning", "Unknown", "skipped"]);
  });
});

describe("checkLabel", () => {
  it("names networks and system checks", () => {
    const names = new Map([["n1", "LunarLabs"]]);
    expect(checkLabel("network/n1", names)).toBe("LunarLabs");
    expect(checkLabel("network/gone", names)).toBe("Unknown network");
    expect(checkLabel("secret_store", names)).toBe("Keyring");
    expect(checkLabel("future_check", names)).toBe("future_check");
  });
});

describe("keyringLabel", () => {
  it("names the backend and warns about memory only", () => {
    expect(keyringLabel("secret-service", "available")).toBe("Secret Service, available");
    expect(keyringLabel("keychain", "locked")).toBe("macOS Keychain, locked");
    expect(keyringLabel("memory", "memory")).toMatch(/^Memory only/);
  });
});

describe("detailParts", () => {
  it("splits control server reports from the facts", () => {
    expect(
      detailParts("headscale, control hs.example, state degraded; control server reports: DERP down; login expired"),
    ).toEqual({ facts: "headscale, control hs.example, state degraded", reports: ["DERP down", "login expired"] });
    expect(detailParts("schema version 6")).toEqual({ facts: "schema version 6", reports: [] });
  });
});

describe("systemChecks", () => {
  it("keeps system checks and describes the keyring from the response", () => {
    const r = create(RunDiagnosticsResponseSchema, {
      secretStoreBackend: "secret-service",
      secretStoreState: "available",
      checks: [
        { name: "daemon", status: "ok", safeDetail: "version 1" },
        { name: "secret_store", status: "ok", safeDetail: "backend secret-service, state available" },
        { name: "network/n1", status: "warn", safeDetail: "x" },
      ],
    });
    expect(systemChecks(r).map(({ name, status, safeDetail }) => [name, status, safeDetail])).toEqual([
      ["daemon", "ok", "version 1"],
      ["secret_store", "ok", "Secret Service, available"],
    ]);
  });

  it("adds a keyring row with unknown status when the daemon sends no check for it", () => {
    const r = create(RunDiagnosticsResponseSchema, { secretStoreBackend: "memory", secretStoreState: "memory" });
    expect(systemChecks(r)).toEqual([
      { name: "secret_store", status: "", safeDetail: "Memory only. Nothing is saved between daemon restarts." },
    ]);
  });
});

import type { DiagnosticCheck, RunDiagnosticsResponse } from "@gen/flavor/v1/diagnostics_pb";

export type Tone = "ok" | "warn" | "error" | "unknown";
export type CheckRow = Pick<DiagnosticCheck, "name" | "status" | "safeDetail">;

const tones: Record<string, Tone> = { ok: "ok", warning: "warn", warn: "warn", error: "error" };
const toneLabels: Record<Tone, string> = { ok: "OK", warn: "Warning", error: "Error", unknown: "Unknown" };
const systemNames: Record<string, string> = { daemon: "Daemon", database: "Database", secret_store: "Keyring" };
const backends: Record<string, string> = {
  "secret-service": "Secret Service",
  keychain: "macOS Keychain",
  wincred: "Windows Credential Manager",
};
const healthMarker = "; control server reports: ";

export const checkTone = (status: string): Tone => tones[status] ?? "unknown";

export const statusLabel = (status: string) => (tones[status] ? toneLabels[tones[status]] : status || "Unknown");

export const isNetworkCheck = (name: string) => name.startsWith("network/");

export function checkLabel(name: string, networkNames: Map<string, string>): string {
  if (isNetworkCheck(name)) return networkNames.get(name.slice(8)) ?? "Unknown network";
  return systemNames[name] ?? name;
}

export function keyringLabel(backend: string, state: string): string {
  if (backend === "memory") return "Memory only. Nothing is saved between daemon restarts.";
  return `${backends[backend] ?? "System keyring"}, ${state}`;
}

export function detailParts(detail: string): { facts: string; reports: string[] } {
  const [facts, health] = detail.split(healthMarker);
  return { facts, reports: health ? health.split("; ") : [] };
}

export function systemChecks(r: RunDiagnosticsResponse): CheckRow[] {
  const rows: CheckRow[] = r.checks.filter((c) => !isNetworkCheck(c.name));
  if (!rows.some((c) => c.name === "secret_store")) rows.push({ name: "secret_store", status: "", safeDetail: "" });
  const keyring = keyringLabel(r.secretStoreBackend, r.secretStoreState);
  return rows.map((c) => (c.name === "secret_store" ? { ...c, safeDetail: keyring } : c));
}

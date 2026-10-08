import { useEffect, useState } from "react";
import type { RunDiagnosticsResponse } from "@gen/flavor/v1/diagnostics_pb";
import { Banner } from "../../components/ui/Banner";
import { Button } from "../../components/ui/Button";
import { runDiagnostics } from "../../lib/api/daemon";
import { useDaemon } from "../../app/sync/useDaemon";
import { useAction } from "../../app/useAction";

const tones: Record<string, string> = { ok: "badge-ok", warning: "badge-warn", error: "badge-danger" };

function checkLabel(name: string, networkNames: Map<string, string>): string {
  if (name.startsWith("network/")) return networkNames.get(name.slice(8)) ?? "Network";
  return { daemon: "Daemon", database: "Database", secret_store: "Keyring" }[name] ?? name;
}

export function DiagnosticsPage() {
  const { networks, status } = useDaemon();
  const [result, setResult] = useState<RunDiagnosticsResponse | null>(null);
  const run = useAction(runDiagnostics);
  const names = new Map([...networks.values()].map((n) => [n.id, n.displayName]));

  const refresh = async () => {
    const r = await run.run();
    if (r.ok) setResult(r.value);
  };
  useEffect(() => {
    if (status === "ready") void refresh();
  }, [status]);

  return (
    <div className="page">
      <header className="page-header">
        <div className="stack-sm">
          <h1>Diagnostics</h1>
          <p className="muted">A safe summary of the daemon and each network. It never includes keys or sign-in links.</p>
        </div>
        <Button loading={run.pending} disabled={status !== "ready"} onClick={() => void refresh()}>
          Run again
        </Button>
      </header>
      {run.error && <Banner tone="danger">{run.error}</Banner>}
      <div className="card">
        {!result ? (
          <div className="stack-sm" aria-busy="true">
            <div className="skeleton" />
            <div className="skeleton" />
            <div className="skeleton" />
          </div>
        ) : (
          <ul className="check-list">
            {result.checks.map((c) => (
              <li key={c.name}>
                <span>
                  <span className={`badge ${tones[c.status] ?? ""}`}>{c.status === "ok" ? "OK" : c.status === "warning" ? "Warning" : "Error"}</span>
                </span>
                <span className="stack-sm">
                  <strong>{checkLabel(c.name, names)}</strong>
                  <span className="muted small">{c.safeDetail}</span>
                </span>
              </li>
            ))}
          </ul>
        )}
      </div>
    </div>
  );
}

import { useEffect, useState } from "react";
import { Banner } from "../../components/ui/Banner";
import { appVersion, runDiagnostics } from "../../lib/api/daemon";
import type { CloseBehavior, LoadedSettings, Settings, Theme } from "../../lib/api/types";
import { useDaemon } from "../../app/sync/useDaemon";

type Props = { settings: LoadedSettings; error: string | null; onChange: (s: Settings) => void };

const themes: [Theme, string][] = [
  ["system", "System"],
  ["light", "Light"],
  ["dark", "Dark"],
];

const closeOptions: [CloseBehavior, string, string][] = [
  ["tray", "Keep Lattice in the tray", "Closing the window hides it. Use the tray icon to show it again."],
  ["quit_gui", "Quit the window", "Closing the window quits the desktop app."],
];

function keyringLabel(backend: string, state: string): string {
  if (backend === "memory") return "Memory only. Nothing is saved between daemon restarts.";
  return `System keyring, ${state}`;
}

export function SettingsPage({ settings, error, onChange }: Props) {
  const { info, instanceId, status } = useDaemon();
  const [version, setVersion] = useState("");
  const [keyring, setKeyring] = useState<string | null>(null);
  useEffect(() => {
    void appVersion().then(setVersion);
  }, []);
  useEffect(() => {
    if (status !== "ready") return;
    runDiagnostics()
      .then((d) => setKeyring(keyringLabel(d.secretStoreBackend, d.secretStoreState)))
      .catch(() => setKeyring(null));
  }, [status]);

  return (
    <div className="page">
      <header className="page-header">
        <h1>Settings</h1>
      </header>
      {settings.warning && <Banner tone="warn">{settings.warning}</Banner>}
      {error && <Banner tone="danger">{error}</Banner>}

      <section className="card stack" aria-labelledby="appearance">
        <h3 id="appearance">Appearance</h3>
        <div className="segmented" role="group" aria-labelledby="appearance">
          {themes.map(([value, label]) => (
            <button key={value} type="button" aria-pressed={settings.theme === value} onClick={() => onChange({ ...settings, theme: value })}>
              {label}
            </button>
          ))}
        </div>
      </section>

      <section className="card stack" aria-labelledby="closing">
        <h3 id="closing">When the window closes</h3>
        <p className="muted small">Networks stay connected either way. The Lattice daemon keeps running in the background.</p>
        {closeOptions.map(([value, label, hint]) => (
          <label key={value} className="check">
            <input
              type="radio"
              name="close"
              checked={settings.closeBehavior === value}
              onChange={() => onChange({ ...settings, closeBehavior: value })}
            />
            <span>
              {label}
              <span className="hint check-hint">{hint}</span>
            </span>
          </label>
        ))}
      </section>

      <section className="card stack" aria-labelledby="about">
        <h3 id="about">About</h3>
        <dl className="dl">
          <dt>Lattice desktop</dt>
          <dd className="mono">{version || "…"}</dd>
          <dt>Daemon</dt>
          <dd className="mono">{info ? `${info.daemonVersion}${info.buildCommit ? ` (${info.buildCommit.slice(0, 12)})` : ""}` : "Not connected"}</dd>
          <dt>Protocol</dt>
          <dd className="mono">{info ? `${info.protocolMajor}.${info.protocolMinor}` : "…"}</dd>
          <dt>Keyring</dt>
          <dd>{keyring ?? "Unknown"}</dd>
          <dt>Daemon session</dt>
          <dd className="mono">{instanceId || "…"}</dd>
        </dl>
      </section>
    </div>
  );
}

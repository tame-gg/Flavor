import { useEffect, useState } from "react";
import { Capability, NetworkConnectionState } from "@gen/lattice/v1/common_pb";
import logo from "../logo.svg";
import { Banner } from "../components/ui/Banner";
import { Button } from "../components/ui/Button";
import { ConflictsPage } from "../features/conflicts/ConflictsPage";
import { DevicesPage } from "../features/devices/DevicesPage";
import { DiagnosticsPage } from "../features/diagnostics/DiagnosticsPage";
import { InspectorPage } from "../features/inspector/InspectorPage";
import { NetworksPage } from "../features/networks/NetworksPage";
import { Welcome } from "../features/onboarding/Welcome";
import { SettingsPage } from "../features/settings/SettingsPage";
import { getSettings, setSettings, setTraySummary } from "../lib/api/daemon";
import { errorMessage } from "../lib/api/errors";
import type { LoadedSettings, Settings } from "../lib/api/types";
import { visiblePages, type Page } from "./navigation";
import { useDaemon } from "./sync/useDaemon";

export function App() {
  const { status, info, networks, devices, controller } = useDaemon();
  const [page, setPage] = useState<Page>("networks");
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [deviceNetwork, setDeviceNetwork] = useState("");
  const [deviceQuery, setDeviceQuery] = useState("");
  const [inspectorQuery, setInspectorQuery] = useState("");
  const [addOpen, setAddOpen] = useState(false);
  const [welcomeDone, setWelcomeDone] = useState(false);
  const [settings, setSettingsState] = useState<LoadedSettings>({ theme: "system", closeBehavior: "tray", warning: null });
  const [settingsError, setSettingsError] = useState<string | null>(null);

  useEffect(() => {
    void getSettings().then(setSettingsState);
  }, []);

  useEffect(() => {
    const root = document.documentElement;
    if (settings.theme === "system") delete root.dataset.theme;
    else root.dataset.theme = settings.theme;
  }, [settings.theme]);

  const connected = [...networks.values()].filter((n) => n.state === NetworkConnectionState.CONNECTED).length;
  const summary =
    status === "unavailable" || status === "incompatible"
      ? "Daemon not reachable"
      : networks.size === 0
        ? "No networks"
        : `${connected} of ${networks.size} network${networks.size === 1 ? "" : "s"} connected`;
  useEffect(() => {
    void setTraySummary(summary);
  }, [summary]);

  const changeSettings = async (next: Settings) => {
    setSettingsError(null);
    try {
      setSettingsState(await setSettings(next));
    } catch (e) {
      setSettingsError(errorMessage(e));
    }
  };

  if (status === "incompatible") {
    return (
      <Gate title="Update required">
        This version of the desktop app cannot talk to the running Lattice daemon (protocol {info?.protocolMajor}). Install
        matching versions of Lattice and latticed.
      </Gate>
    );
  }
  if (!info) {
    return status === "unavailable" ? (
      <Gate title="The Lattice daemon is not running" action={<Button onClick={() => void controller.resync()}>Try again</Button>}>
        Start it with <code className="mono">systemctl --user start latticed</code>, or run <code className="mono">latticed</code>{" "}
        in a terminal. Lattice keeps retrying in the background.
      </Gate>
    ) : (
      <Gate title="Connecting to the Lattice daemon" busy />
    );
  }
  if (networks.size === 0 && !welcomeDone) {
    return (
      <Welcome
        onAdd={() => {
          setWelcomeDone(true);
          setAddOpen(true);
        }}
        onLater={() => setWelcomeDone(true)}
      />
    );
  }

  const openNetwork = (id: string) => {
    setSelectedId(id);
    setPage("networks");
  };
  const showDevice = (networkId: string, search: string) => {
    setDeviceNetwork(networkId);
    setDeviceQuery(search);
    setPage("devices");
  };
  const inspect = (destination: string) => {
    setInspectorQuery(destination);
    setPage("inspector");
  };

  return (
    <div className="shell">
      <aside className="sidebar">
        <div className="brand">
          <img src={logo} alt="" />
          Lattice
        </div>
        <nav className="stack-sm" aria-label="Sections">
          {visiblePages(info.capabilities).map(([id, label]) => (
            <button key={id} type="button" className="nav-item" aria-current={page === id ? "page" : undefined} onClick={() => setPage(id)}>
              {label}
              {id === "networks" && <span className="nav-count">{networks.size}</span>}
              {id === "devices" && <span className="nav-count">{devices.size}</span>}
            </button>
          ))}
        </nav>
        <span className="spacer" />
        <p className="small muted">{summary}</p>
      </aside>
      <div className="topline" aria-live="polite">
        {status === "unavailable" && <Banner tone="warn">The Lattice daemon is not reachable. Showing last known state while Lattice retries.</Banner>}
        {status === "resyncing" && <Banner>Refreshing from the daemon…</Banner>}
      </div>
      <main className="main">
        {page === "networks" && (
          <NetworksPage
            selectedId={selectedId}
            onSelect={setSelectedId}
            addOpen={addOpen}
            setAddOpen={setAddOpen}
            onShowDevices={(id) => {
              setDeviceNetwork(id);
              setPage("devices");
            }}
          />
        )}
        {page === "devices" && (
          <DevicesPage networkId={deviceNetwork} onNetworkChange={setDeviceNetwork} query={deviceQuery} onQueryChange={setDeviceQuery} />
        )}
        {page === "inspector" && info.capabilities.includes(Capability.CONNECTION_INSPECTOR) && (
          <InspectorPage query={inspectorQuery} onQueryChange={setInspectorQuery} onOpenNetwork={openNetwork} onShowDevice={showDevice} />
        )}
        {page === "conflicts" && info.capabilities.includes(Capability.CONFLICT_CENTER) && (
          <ConflictsPage onInspect={inspect} onOpenNetwork={openNetwork} onShowDevice={showDevice} />
        )}
        {page === "diagnostics" && <DiagnosticsPage />}
        {page === "settings" && <SettingsPage settings={settings} error={settingsError} onChange={(s) => void changeSettings(s)} />}
      </main>
    </div>
  );
}

function Gate({ title, children, action, busy }: { title: string; children?: React.ReactNode; action?: React.ReactNode; busy?: boolean }) {
  return (
    <main className="center-screen">
      <div className="center-card" aria-busy={busy || undefined}>
        <img className="logo" src={logo} alt="" />
        <div className="row">
          {busy && <span className="spinner" aria-hidden="true" />}
          <h1>{title}</h1>
        </div>
        {children && <p className="muted">{children}</p>}
        {action && <div className="row">{action}</div>}
      </div>
    </main>
  );
}

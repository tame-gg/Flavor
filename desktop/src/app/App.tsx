import { useEffect, useState } from "react";
import { Loader2 } from "lucide-react";
import { AnimatePresence, motion } from "motion/react";
import { Capability, NetworkConnectionState } from "@gen/flavor/v1/common_pb";
import logo from "../logo.svg";
import { Button } from "@/ui/button";
import { ConflictsPage } from "../features/conflicts/ConflictsPage";
import { DevicesPage } from "../features/devices/DevicesPage";
import { DiagnosticsPage } from "../features/diagnostics/DiagnosticsPage";
import { InspectorPage } from "../features/inspector/InspectorPage";
import { NetworksPage } from "../features/networks/NetworksPage";
import { OverviewPage } from "../features/overview/OverviewPage";
import { summarize } from "../features/overview/summary";
import { Welcome } from "../features/onboarding/Welcome";
import { SettingsPage } from "../features/settings/SettingsPage";
import { WorkspacesPage } from "../features/workspaces/WorkspacesPage";
import { getSettings, setSettings, setTraySummary } from "../lib/api/daemon";
import { errorMessage } from "../lib/api/errors";
import type { LoadedSettings, Settings } from "../lib/api/types";
import { CommandPalette } from "./CommandPalette";
import { visiblePages, type Page } from "./navigation";
import { Sidebar } from "./shell/Sidebar";
import { deviceKey } from "./sync/controller";
import { useDaemon } from "./sync/useDaemon";

export function App() {
  const { status, info, networks, devices, workspaces, activeWorkspaceId, controller } = useDaemon();
  const [page, setPage] = useState<Page>("overview");
  const [paletteOpen, setPaletteOpen] = useState(false);
  const [selectedId, setSelectedId] = useState<string | null>(null);
  const [deviceNetwork, setDeviceNetwork] = useState("");
  const [deviceQuery, setDeviceQuery] = useState("");
  const [selectedDevice, setSelectedDevice] = useState<string | null>(null);
  const [inspectorQuery, setInspectorQuery] = useState("");
  const [addOpen, setAddOpen] = useState(false);
  const [welcomeDone, setWelcomeDoneState] = useState(() => localStorage.getItem("flavor.welcomeDone") === "1");
  const setWelcomeDone = (done: boolean) => {
    localStorage.setItem("flavor.welcomeDone", done ? "1" : "0");
    setWelcomeDoneState(done);
  };
  const [settings, setSettingsState] = useState<LoadedSettings>({
    theme: "system",
    closeBehavior: "tray",
    warning: null,
  });
  const [settingsError, setSettingsError] = useState<string | null>(null);

  useEffect(() => {
    void getSettings().then(setSettingsState);
  }, []);

  useEffect(() => {
    const root = document.documentElement;
    if (settings.theme === "system") delete root.dataset.theme;
    else root.dataset.theme = settings.theme;
  }, [settings.theme]);

  useEffect(() => {
    const onKey = (e: KeyboardEvent) => {
      if ((e.ctrlKey || e.metaKey) && !e.altKey && e.key.toLowerCase() === "k") {
        e.preventDefault();
        setPaletteOpen((o) => !o);
      }
    };
    window.addEventListener("keydown", onKey);
    return () => window.removeEventListener("keydown", onKey);
  }, []);

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
        This version of the desktop app cannot talk to the running Flavor daemon (protocol {info?.protocolMajor}).
        Install matching versions of Flavor and flavord.
      </Gate>
    );
  }
  if (!info) {
    return status === "unavailable" ? (
      <Gate
        title="The Flavor daemon is not running"
        action={<Button onClick={() => void controller.resync()}>Try again</Button>}
      >
        Start it with <code className="font-mono text-foreground">systemctl --user start flavord</code>, or run{" "}
        <code className="font-mono text-foreground">flavord</code> in a terminal. Flavor keeps retrying in the background.
      </Gate>
    ) : (
      <Gate title="Connecting to the Flavor daemon" busy />
    );
  }
  if (networks.size === 0 && !welcomeDone) {
    return (
      <Welcome
        onAdd={() => {
          setWelcomeDone(true);
          setPage("networks");
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
  const showDevice = (networkId: string, nodeId: string) => {
    setDeviceNetwork("");
    setDeviceQuery("");
    setSelectedDevice(deviceKey(networkId, nodeId));
    setPage("devices");
  };
  const inspect = (destination: string) => {
    setInspectorQuery(destination);
    setPage("inspector");
  };

  const alerts = summarize(networks.values(), devices.values(), 0).needsYou.length;
  const pages = visiblePages(info.capabilities);
  const canInspect = info.capabilities.includes(Capability.CONNECTION_INSPECTOR);

  return (
    <div className="flex h-full bg-background text-foreground">
      <Sidebar
        pages={pages}
        page={page}
        onPage={setPage}
        counts={{ networks: networks.size, devices: devices.size }}
        alerts={{ overview: alerts }}
        workspace={workspaces.get(activeWorkspaceId)?.name ?? null}
        status={status}
        onSearch={() => setPaletteOpen(true)}
      />
      <main className="min-w-0 flex-1 overflow-auto px-8 pt-6 pb-8">
        <AnimatePresence mode="wait" initial={false}>
          <motion.div
            key={page}
            initial={{ opacity: 0, y: 6 }}
            animate={{ opacity: 1, y: 0 }}
            exit={{ opacity: 0, y: -4 }}
            transition={{ duration: 0.16, ease: [0.2, 0, 0, 1] }}
          >
            {page === "overview" && (
              <OverviewPage
                onOpenNetwork={openNetwork}
                onConflicts={() => setPage("conflicts")}
                onAddNetwork={() => {
                  setPage("networks");
                  setAddOpen(true);
                }}
              />
            )}
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
            {page === "workspaces" && info.capabilities.includes(Capability.WORKSPACES) && (
              <WorkspacesPage onOpenNetwork={openNetwork} />
            )}
            {page === "devices" && (
              <DevicesPage
                networkId={deviceNetwork}
                onNetworkChange={setDeviceNetwork}
                query={deviceQuery}
                onQueryChange={setDeviceQuery}
                selected={selectedDevice}
                onSelect={setSelectedDevice}
                onInspect={inspect}
                onOpenNetwork={openNetwork}
                canInspect={canInspect}
              />
            )}
            {page === "inspector" && canInspect && (
              <InspectorPage
                query={inspectorQuery}
                onQueryChange={setInspectorQuery}
                onOpenNetwork={openNetwork}
                onShowDevice={showDevice}
                canPrefer={info.capabilities.includes(Capability.DESTINATION_PREFERENCES)}
              />
            )}
            {page === "conflicts" && info.capabilities.includes(Capability.CONFLICT_CENTER) && (
              <ConflictsPage
                onInspect={inspect}
                onOpenNetwork={openNetwork}
                onShowDevice={showDevice}
                canPrefer={info.capabilities.includes(Capability.DESTINATION_PREFERENCES)}
              />
            )}
            {page === "diagnostics" && <DiagnosticsPage />}
            {page === "settings" && (
              <SettingsPage settings={settings} error={settingsError} onChange={(s) => void changeSettings(s)} />
            )}
          </motion.div>
        </AnimatePresence>
      </main>
      <CommandPalette
        open={paletteOpen}
        onOpenChange={setPaletteOpen}
        pages={pages}
        canInspect={canInspect}
        onPage={setPage}
        onOpenNetwork={openNetwork}
        onShowDevice={showDevice}
        onInspect={inspect}
      />
    </div>
  );
}

function Gate({
  title,
  children,
  action,
  busy,
}: {
  title: string;
  children?: React.ReactNode;
  action?: React.ReactNode;
  busy?: boolean;
}) {
  return (
    <main className="flex h-full items-center justify-center bg-background p-6 text-foreground">
      <div className="flex w-full max-w-md flex-col gap-4 rounded-xl border bg-card p-6 shadow-sm" aria-busy={busy || undefined}>
        <img className="size-10" src={logo} alt="" />
        <h1 className="flex items-center gap-2 text-lg font-semibold">
          {busy && <Loader2 className="size-4 animate-spin text-muted-foreground" aria-hidden="true" />}
          {title}
        </h1>
        {children && <p className="text-sm text-muted-foreground">{children}</p>}
        {action && <div className="flex gap-2">{action}</div>}
      </div>
    </main>
  );
}

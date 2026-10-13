import { useMemo, useState } from "react";
import { Plus } from "lucide-react";
import { motion } from "motion/react";
import { NetworkConnectionState as S } from "@gen/flavor/v1/common_pb";
import type { Network } from "@gen/flavor/v1/network_pb";
import { useCanMutate, useDaemon } from "../../app/sync/useDaemon";
import { cn } from "@/lib/cn";
import { networkColorIndex, networkColorVar } from "@/lib/networkColor";
import { Button } from "@/ui/button";
import { Card } from "@/ui/card";
import { AddNetworkDialog } from "./AddNetworkDialog";
import { NetworkDetail } from "./NetworkDetail";
import { RemoveNetworkDialog } from "./RemoveNetworkDialog";
import { isLive, networkSource } from "./format";
import { StateBadge } from "./StateBadge";

type Props = {
  selectedId: string | null;
  onSelect: (id: string | null) => void;
  onShowDevices: (networkId: string) => void;
  addOpen: boolean;
  setAddOpen: (open: boolean) => void;
};

export function NetworksPage({ selectedId, onSelect, onShowDevices, addOpen, setAddOpen }: Props) {
  const { networks, devices } = useDaemon();
  const canMutate = useCanMutate();
  const [removing, setRemoving] = useState<Network | null>(null);
  const [notice, setNotice] = useState<{ id: string; text: string } | null>(null);
  const [tab, setTab] = useState("overview");

  const list = useMemo(
    () => [...networks.values()].sort((a, b) => a.displayName.localeCompare(b.displayName) || a.id.localeCompare(b.id)),
    [networks],
  );
  const counts = useMemo(() => {
    const m = new Map<string, number>();
    for (const d of devices.values()) {
      const id = d.id?.networkId ?? "";
      m.set(id, (m.get(id) ?? 0) + 1);
    }
    return m;
  }, [devices]);
  const colors = useMemo(() => networkColorIndex(networks.values()), [networks]);
  const selected = (selectedId && networks.get(selectedId)) || list[0] || null;
  const noticed = notice && networks.get(notice.id);
  if (noticed && isLive(noticed.state)) setNotice(null);

  return (
    <div className="mx-auto flex w-full max-w-5xl flex-col gap-6">
      <header className="flex items-start justify-between gap-4">
        <div className="flex flex-col gap-1">
          <h1 className="text-xl font-semibold">Networks</h1>
          <p className="text-sm text-muted-foreground">
            Each network runs its own isolated session with its own device identity.
          </p>
        </div>
        <Button disabled={!canMutate} onClick={() => setAddOpen(true)}>
          <Plus />
          Add network
        </Button>
      </header>

      {list.length === 0 ? (
        <Card className="items-center gap-3 px-6 py-10 text-center">
          <h2 className="text-base font-semibold">No networks yet</h2>
          <p className="max-w-md text-sm text-muted-foreground">
            Add a Tailscale account or a Headscale server. You can join several at once; each keeps its own devices and
            addresses.
          </p>
          <Button disabled={!canMutate} onClick={() => setAddOpen(true)}>
            <Plus />
            Add your first network
          </Button>
        </Card>
      ) : (
        <div className="grid items-start gap-6 lg:grid-cols-[16rem_minmax(0,1fr)]">
          <nav aria-label="Networks">
            <Card className="max-h-56 gap-0 overflow-y-auto py-0 lg:max-h-none">
              <ul>
                {list.map((n, i) => {
                  const current = selected?.id === n.id;
                  const message =
                    (n.state === S.DEGRADED || n.state === S.ERROR) && n.statusMessage ? n.statusMessage : null;
                  return (
                    <motion.li
                      key={n.id}
                      initial={{ opacity: 0, y: 4 }}
                      animate={{ opacity: 1, y: 0 }}
                      transition={{ delay: i * 0.03, duration: 0.18 }}
                      className="border-t border-border first:border-t-0"
                    >
                      <button
                        type="button"
                        aria-current={current ? "true" : undefined}
                        onClick={() => onSelect(n.id)}
                        className={cn(
                          "flex w-full items-start gap-3 px-4 py-3 text-left transition-colors outline-none hover:bg-accent/60 focus-visible:inset-ring-2 focus-visible:inset-ring-ring/60",
                          current && "bg-accent hover:bg-accent",
                        )}
                      >
                        <span
                          className="mt-1.5 size-2.5 shrink-0 rounded-[3px]"
                          style={{ background: networkColorVar(colors.get(n.id)) }}
                          aria-hidden="true"
                        />
                        <span className="flex min-w-0 flex-1 flex-col gap-1">
                          <span className="flex items-center gap-2">
                            <span className="min-w-0 flex-1 truncate text-sm font-medium">{n.displayName}</span>
                            <StateBadge state={n.state} />
                          </span>
                          <span className="hidden truncate text-xs text-muted-foreground lg:block">{networkSource(n)}</span>
                          {message && (
                            <span
                              className={cn(
                                "line-clamp-2 text-xs",
                                n.state === S.ERROR ? "text-destructive" : "text-warning",
                              )}
                            >
                              {message}
                            </span>
                          )}
                        </span>
                      </button>
                    </motion.li>
                  );
                })}
              </ul>
            </Card>
          </nav>
          {selected && (
            <NetworkDetail
              key={selected.id}
              network={selected}
              deviceCount={counts.get(selected.id) ?? 0}
              notice={notice?.id === selected.id ? notice.text : null}
              onShowDevices={() => onShowDevices(selected.id)}
              onRemove={() => setRemoving(selected)}
              tab={tab}
              onTabChange={setTab}
            />
          )}
        </div>
      )}

      <AddNetworkDialog
        open={addOpen}
        onClose={() => setAddOpen(false)}
        onAdded={(n, followUp) => {
          setAddOpen(false);
          onSelect(n.id);
          setTab("overview");
          setNotice(followUp ? { id: n.id, text: followUp } : null);
        }}
      />
      <RemoveNetworkDialog
        network={removing}
        onClose={() => setRemoving(null)}
        onRemoved={() => {
          setRemoving(null);
          onSelect(null);
        }}
      />
    </div>
  );
}

import { useEffect, useMemo, useState } from "react";
import { ArrowRight, CircleAlert, LogIn } from "lucide-react";
import { motion } from "motion/react";
import { toast } from "sonner";
import { Capability, NetworkConnectionState as S } from "@gen/flavor/v1/common_pb";
import { ConflictSeverity } from "@gen/flavor/v1/conflicts_pb";
import type { Network } from "@gen/flavor/v1/network_pb";
import { stateLabel } from "@/lib/networkState";
import { useCanMutate, useDaemon } from "../../app/sync/useDaemon";
import { connectNetwork, disconnectNetwork, listConflicts, openAuthUrl } from "../../lib/api/daemon";
import { errorMessage } from "../../lib/api/errors";
import { cn } from "@/lib/cn";
import { networkColorIndex, networkColorVar } from "@/lib/networkColor";
import { Button } from "@/ui/button";
import { Card } from "@/ui/card";
import { Switch } from "@/ui/switch";
import { summarize, type NeedsYou, type Tone } from "./summary";

const toneDot: Record<Tone, string> = {
  ok: "bg-success",
  warn: "bg-warning",
  danger: "bg-destructive",
  neutral: "bg-muted-foreground",
};

const active = new Set([S.CONNECTING, S.AUTHENTICATING, S.AWAITING_APPROVAL, S.CONNECTED, S.DEGRADED, S.RECONNECTING]);

type Props = {
  onOpenNetwork: (id: string) => void;
  onConflicts: () => void;
  onAddNetwork: () => void;
};

export function OverviewPage({ onOpenNetwork, onConflicts, onAddNetwork }: Props) {
  const { networks, devices, workspaces, activeWorkspaceId, sequence, status, info } = useDaemon();
  const [ambiguous, setAmbiguous] = useState(0);
  const canConflicts = info?.capabilities.includes(Capability.CONFLICT_CENTER) ?? false;

  useEffect(() => {
    if (status !== "ready" || !canConflicts) return;
    let cancelled = false;
    const t = setTimeout(() => {
      listConflicts()
        .then(
          (r) =>
            !cancelled && setAmbiguous(r.conflicts.filter((c) => c.severity === ConflictSeverity.AMBIGUOUS).length),
        )
        .catch(() => {});
    }, 250);
    return () => {
      cancelled = true;
      clearTimeout(t);
    };
  }, [sequence, status, canConflicts]);

  const overview = useMemo(
    () => summarize(networks.values(), devices.values(), ambiguous),
    [networks, devices, ambiguous],
  );
  const colors = useMemo(() => networkColorIndex(networks.values()), [networks]);
  const sorted = [...networks.values()].sort((a, b) => a.displayName.localeCompare(b.displayName));
  const workspace = workspaces.get(activeWorkspaceId);

  const runNeed = (item: NeedsYou) => {
    if (item.action === "conflicts") return onConflicts();
    const n = item.networkId ? networks.get(item.networkId) : undefined;
    if (item.action === "sign-in" && n?.authentication?.flowId) {
      openAuthUrl(n.id, n.authentication.flowId).catch((e) => toast.error(errorMessage(e)));
      return;
    }
    if (item.networkId) onOpenNetwork(item.networkId);
  };

  return (
    <div className="mx-auto flex w-full max-w-5xl flex-col gap-6">
      <header className="flex flex-col gap-1">
        <h1 className="flex items-center gap-2 text-xl font-semibold">
          <span className={cn("size-2.5 rounded-full", toneDot[overview.tone])} aria-hidden="true" />
          {overview.headline}
        </h1>
        <p className="font-mono text-xs text-muted-foreground">
          {overview.devicesOnline} devices online
          {ambiguous > 0 && ` · ${ambiguous} ambiguous`}
          {workspace && ` · workspace ${workspace.name}`}
        </p>
      </header>

      {overview.needsYou.length > 0 && (
        <section aria-labelledby="needs-you" className="flex flex-col gap-2">
          <h2 id="needs-you" className="text-sm font-medium text-muted-foreground">
            Needs you
          </h2>
          <Card className="gap-0 py-0">
            {overview.needsYou.map((item, i) => (
              <motion.div
                key={item.key}
                initial={{ opacity: 0, y: 4 }}
                animate={{ opacity: 1, y: 0 }}
                transition={{ delay: i * 0.04, duration: 0.18 }}
                className="flex items-center gap-3 border-t border-border px-4 py-3 first:border-t-0"
              >
                {item.networkId ? (
                  <span
                    className="size-2.5 shrink-0 rounded-[3px]"
                    style={{ background: networkColorVar(colors.get(item.networkId)) }}
                  />
                ) : (
                  <CircleAlert className="size-4 shrink-0 text-warning" />
                )}
                <p className="min-w-0 flex-1 text-sm">
                  <span className="font-medium">{item.title}</span>{" "}
                  <span className="text-muted-foreground">{item.detail}</span>
                </p>
                <Button size="sm" variant="outline" onClick={() => runNeed(item)}>
                  {item.action === "sign-in" ? (
                    <>
                      <LogIn /> Sign in
                    </>
                  ) : item.action === "conflicts" ? (
                    "Review"
                  ) : (
                    "Open"
                  )}
                </Button>
              </motion.div>
            ))}
          </Card>
        </section>
      )}

      <section aria-label="Summary" className="grid grid-cols-2 gap-3 lg:grid-cols-4">
        <Tile label="Networks connected" value={`${overview.connected} / ${overview.total}`} />
        <Tile label="Devices online" value={`${overview.devicesOnline} / ${overview.devicesTotal}`} />
        <Tile label="Ambiguous destinations" value={String(ambiguous)} tone={ambiguous > 0 ? "warn" : undefined} />
        <Tile label="Active workspace" value={workspace?.name ?? "None"} />
      </section>

      <section aria-labelledby="networks-heading" className="flex flex-col gap-2">
        <div className="flex items-center justify-between">
          <h2 id="networks-heading" className="text-sm font-medium text-muted-foreground">
            Networks
          </h2>
          <Button size="sm" variant="ghost" onClick={onAddNetwork}>
            Add network
          </Button>
        </div>
        {sorted.length === 0 ? (
          <Card className="items-center gap-3 py-10 text-center">
            <p className="text-sm text-muted-foreground">Add a Tailscale or Headscale network to get started.</p>
            <Button onClick={onAddNetwork}>Add network</Button>
          </Card>
        ) : (
          <Card className="gap-0 py-0">
            {sorted.map((n, i) => (
              <NetworkRow
                key={n.id}
                network={n}
                index={i}
                color={networkColorVar(colors.get(n.id))}
                online={[...devices.values()].filter((d) => d.id?.networkId === n.id && !d.local && d.online).length}
                total={[...devices.values()].filter((d) => d.id?.networkId === n.id && !d.local).length}
                exitNode={[...devices.values()].find((d) => d.id?.networkId === n.id && d.exitNode)?.hostname}
                onOpen={() => onOpenNetwork(n.id)}
              />
            ))}
          </Card>
        )}
      </section>
    </div>
  );
}

function Tile({ label, value, tone }: { label: string; value: string; tone?: "warn" }) {
  return (
    <Card className="gap-1 px-4 py-3">
      <span className="text-xs text-muted-foreground">{label}</span>
      <span className={cn("truncate font-mono text-lg font-medium", tone === "warn" && "text-warning")}>{value}</span>
    </Card>
  );
}

function NetworkRow({
  network: n,
  index,
  color,
  online,
  total,
  exitNode,
  onOpen,
}: {
  network: Network;
  index: number;
  color: string;
  online: number;
  total: number;
  exitNode?: string;
  onOpen: () => void;
}) {
  const canMutate = useCanMutate();
  const [pending, setPending] = useState(false);
  const isOn = active.has(n.state);
  const live = n.state === S.CONNECTED || n.state === S.DEGRADED;
  const toggle = async (on: boolean) => {
    setPending(true);
    try {
      await (on ? connectNetwork(n.id) : disconnectNetwork(n.id));
    } catch (e) {
      toast.error(`${n.displayName}: ${errorMessage(e)}`);
    } finally {
      setPending(false);
    }
  };
  const detail = live
    ? [
        `${online}/${total} online`,
        exitNode && `exit via ${exitNode}`,
        n.state === S.DEGRADED && (n.statusMessage || "degraded"),
      ]
        .filter(Boolean)
        .join(" · ")
    : n.state === S.ERROR && n.statusMessage
      ? n.statusMessage
      : stateLabel(n.state);
  return (
    <motion.div
      initial={{ opacity: 0, y: 4 }}
      animate={{ opacity: 1, y: 0 }}
      transition={{ delay: index * 0.03, duration: 0.18 }}
      className="flex items-center gap-3 border-t border-border px-4 py-3 first:border-t-0"
    >
      <span className="size-2.5 shrink-0 rounded-[3px]" style={{ background: color }} aria-hidden="true" />
      <button
        type="button"
        onClick={onOpen}
        className="group flex min-w-0 flex-1 flex-col text-left focus-visible:outline-none"
      >
        <span className="flex items-center gap-1 text-sm font-medium group-hover:underline group-focus-visible:underline">
          {n.displayName}
          <ArrowRight className="size-3 opacity-0 transition-opacity group-hover:opacity-60" />
        </span>
        <span className={cn("truncate text-xs text-muted-foreground", n.state === S.ERROR && "text-destructive")}>
          {detail}
        </span>
      </button>
      <Switch
        checked={isOn}
        disabled={!canMutate || pending || n.state === S.REMOVING}
        onCheckedChange={(v) => void toggle(v)}
        aria-label={`${isOn ? "Disconnect" : "Connect"} ${n.displayName}`}
      />
    </motion.div>
  );
}

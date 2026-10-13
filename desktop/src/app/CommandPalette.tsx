import { useEffect, useMemo, useState } from "react";
import { ArrowRight, Copy, Monitor, Plug, PlugZap, Route } from "lucide-react";
import { toast } from "sonner";
import { NetworkConnectionState as S } from "@gen/flavor/v1/common_pb";
import { CandidateStatus, ResolutionDecision, type InspectDestinationResponse } from "@gen/flavor/v1/inspector_pb";
import { connectNetwork, disconnectNetwork, inspectDestination } from "../lib/api/daemon";
import { errorMessage } from "../lib/api/errors";
import { modKey } from "@/lib/platform";
import { networkColorIndex, networkColorVar } from "@/lib/networkColor";
import {
  Command,
  CommandEmpty,
  CommandGroup,
  CommandInput,
  CommandItem,
  CommandList,
  CommandShortcut,
} from "@/ui/command";
import { Dialog, DialogContent, DialogDescription, DialogHeader, DialogTitle } from "@/ui/dialog";
import type { Page } from "./navigation";
import { deviceKey } from "./sync/controller";
import { useCanMutate, useDaemon } from "./sync/useDaemon";

type Props = {
  open: boolean;
  onOpenChange: (open: boolean) => void;
  pages: [Page, string][];
  canInspect: boolean;
  onPage: (p: Page) => void;
  onOpenNetwork: (id: string) => void;
  onShowDevice: (networkId: string, nodeId: string) => void;
  onInspect: (destination: string) => void;
};

const looksLikeDestination = (q: string) => /^[\w.:/-]+$/.test(q) && /[.:]|\d/.test(q);
const live = new Set([S.CONNECTING, S.AUTHENTICATING, S.AWAITING_APPROVAL, S.CONNECTED, S.DEGRADED, S.RECONNECTING]);

export function CommandPalette({
  open,
  onOpenChange,
  pages,
  canInspect,
  onPage,
  onOpenNetwork,
  onShowDevice,
  onInspect,
}: Props) {
  const { networks, devices } = useDaemon();
  const canMutate = useCanMutate();
  const [query, setQuery] = useState("");
  const [selected, setSelected] = useState("");
  const [inspection, setInspection] = useState<InspectDestinationResponse | null>(null);
  const colors = useMemo(() => networkColorIndex(networks.values()), [networks]);
  const q = query.trim();

  useEffect(() => {
    if (!open) setQuery("");
  }, [open]);

  useEffect(() => {
    setInspection(null);
    if (!canInspect || !looksLikeDestination(q)) return;
    let cancelled = false;
    const t = setTimeout(() => {
      inspectDestination(q)
        .then((r) => !cancelled && setInspection(r))
        .catch(() => {});
    }, 150);
    return () => {
      cancelled = true;
      clearTimeout(t);
    };
  }, [q, canInspect]);

  const run = (fn: () => void) => {
    onOpenChange(false);
    fn();
  };
  const copy = (text: string) =>
    navigator.clipboard.writeText(text).then(
      () => toast.success(`Copied ${text}`),
      () => toast.error("Could not copy"),
    );
  const toggle = (id: string, on: boolean, name: string) =>
    (on ? connectNetwork(id) : disconnectNetwork(id)).then(
      () => toast.success(`${on ? "Connecting" : "Disconnecting"} ${name}`),
      (e) => toast.error(`${name}: ${errorMessage(e)}`),
    );

  const deviceList = [...devices.values()].filter((d) => !d.local && d.id);
  const winner =
    inspection?.decision === ResolutionDecision.UNIQUE
      ? (inspection.candidates.find((c) => c.status === CandidateStatus.SELECTED) ?? inspection.candidates[0])
      : undefined;
  const inspectLabel =
    inspection?.decision === ResolutionDecision.UNIQUE && winner
      ? `${winner.network?.displayName ?? "Unknown network"} · ${winner.device?.hostname || winner.matchedValue}`
      : inspection?.decision === ResolutionDecision.AMBIGUOUS
        ? `Ambiguous: ${new Set(inspection.candidates.map((c) => c.network?.displayName)).size} networks`
        : inspection?.decision === ResolutionDecision.NO_MATCH
          ? "No network has it"
          : "Checking…";

  return (
    <Dialog open={open} onOpenChange={onOpenChange}>
      <DialogContent className="overflow-hidden p-0 sm:max-w-xl" showCloseButton={false}>
        <DialogHeader className="sr-only">
          <DialogTitle>Search Flavor</DialogTitle>
          <DialogDescription>Jump to a page, network or device, or inspect where a destination goes.</DialogDescription>
        </DialogHeader>
        <Command
          value={selected}
          onValueChange={setSelected}
          onKeyDown={(e) => {
            if (e.key === "Enter" && (e.ctrlKey || e.metaKey) && selected.startsWith("device:")) {
              e.preventDefault();
              const d = devices.get(selected.slice(7));
              if (d?.addresses[0]) void copy(d.addresses[0]);
            }
          }}
          className="[&_[cmdk-group-heading]]:px-2 [&_[cmdk-group-heading]]:text-xs [&_[cmdk-group-heading]]:font-medium [&_[cmdk-group-heading]]:text-muted-foreground [&_[cmdk-item]]:py-2"
        >
          <CommandInput
            value={query}
            onValueChange={setQuery}
            placeholder="Search, or type an address or name to inspect"
          />
          <CommandList className="max-h-96">
            <CommandEmpty>Nothing matches.</CommandEmpty>
            {canInspect && looksLikeDestination(q) && (
              <CommandGroup heading="Inspect" forceMount>
                <CommandItem value={`inspect:${q}`} forceMount onSelect={() => run(() => onInspect(q))}>
                  <Route />
                  {winner?.network ? (
                    <span
                      className="size-2.5 rounded-[3px]"
                      style={{ background: networkColorVar(colors.get(winner.network.id)) }}
                    />
                  ) : null}
                  <span className="truncate font-mono">{q}</span>
                  <span className="truncate text-muted-foreground">{inspectLabel}</span>
                  <CommandShortcut>Enter</CommandShortcut>
                </CommandItem>
              </CommandGroup>
            )}
            <CommandGroup heading="Pages">
              {pages.map(([id, label]) => (
                <CommandItem key={id} value={`page:${id} ${label}`} onSelect={() => run(() => onPage(id))}>
                  <ArrowRight />
                  {label}
                </CommandItem>
              ))}
            </CommandGroup>
            <CommandGroup heading="Networks">
              {[...networks.values()].map((n) => {
                const on = live.has(n.state);
                const color = networkColorVar(colors.get(n.id));
                return [
                  <CommandItem
                    key={`open:${n.id}`}
                    value={`network:${n.id} open ${n.displayName}`}
                    onSelect={() => run(() => onOpenNetwork(n.id))}
                  >
                    <span className="size-2.5 rounded-[3px]" style={{ background: color }} />
                    Open {n.displayName}
                  </CommandItem>,
                  <CommandItem
                    key={`toggle:${n.id}`}
                    value={`toggle:${n.id} ${on ? "disconnect" : "connect"} ${n.displayName}`}
                    disabled={!canMutate}
                    onSelect={() => run(() => void toggle(n.id, !on, n.displayName))}
                  >
                    {on ? <Plug /> : <PlugZap />}
                    {on ? "Disconnect" : "Connect"} {n.displayName}
                  </CommandItem>,
                ];
              })}
            </CommandGroup>
            <CommandGroup heading="Devices">
              {deviceList.map((d) => {
                const key = deviceKey(d.id!.networkId, d.id!.nodeId);
                const net = networks.get(d.id!.networkId);
                return (
                  <CommandItem
                    key={key}
                    value={`device:${key}`}
                    keywords={[d.hostname, d.dnsName, ...d.addresses, net?.displayName ?? ""]}
                    onSelect={() => run(() => onShowDevice(d.id!.networkId, d.id!.nodeId))}
                  >
                    <Monitor />
                    <span
                      className="size-2.5 rounded-[3px]"
                      style={{ background: networkColorVar(colors.get(d.id!.networkId)) }}
                    />
                    <span className="truncate">{d.hostname}</span>
                    <span className="truncate font-mono text-xs text-muted-foreground">{d.addresses[0]}</span>
                    {selected === `device:${key}` && (
                      <CommandShortcut className="flex items-center gap-1">
                        <Copy className="size-3" /> {modKey} Enter
                      </CommandShortcut>
                    )}
                  </CommandItem>
                );
              })}
            </CommandGroup>
          </CommandList>
        </Command>
      </DialogContent>
    </Dialog>
  );
}

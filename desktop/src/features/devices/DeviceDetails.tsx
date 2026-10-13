import { Fragment, useEffect, useMemo, useState, type ReactNode } from "react";
import { timestampDate } from "@bufbuild/protobuf/wkt";
import type { DescribeDeviceResponse } from "@gen/flavor/v1/inspector_pb";
import { ArrowRight, ChevronRight, Copy, Loader2, Route, Terminal, X } from "lucide-react";
import { toast } from "sonner";
import { stateLabel } from "@/lib/networkState";
import { useCanMutate, useDaemon } from "../../app/sync/useDaemon";
import { useAction } from "../../app/useAction";
import { clearExitNode, describeDevice, setExitNode } from "../../lib/api/daemon";
import { cn } from "@/lib/cn";
import { networkColorIndex, networkColorVar } from "@/lib/networkColor";
import { Badge } from "@/ui/badge";
import { Button } from "@/ui/button";
import { Skeleton } from "@/ui/skeleton";
import { providerName } from "../networks/format";
import { canRetryDescribe, deviceBadges, lastSeen, sshTarget, type DeviceBadge } from "./format";

type Props = {
  deviceKey: string;
  onClose: () => void;
  onInspect: (destination: string) => void;
  onOpenNetwork: (networkId: string) => void;
  canInspect: boolean;
};

const copy = (value: string) =>
  navigator.clipboard.writeText(value).then(
    () => toast.success(`Copied ${value}`),
    () => toast.error("Could not copy to the clipboard"),
  );

export function DeviceBadges({ badges }: { badges: DeviceBadge[] }) {
  return badges.map((b) => (
    <Badge
      key={b.label}
      variant={b.active ? "default" : "outline"}
      className={cn("h-5 rounded-md px-1.5 py-0 font-normal", !b.active && "text-muted-foreground")}
    >
      {b.label}
    </Badge>
  ));
}

export function DeviceDetails({ deviceKey, onClose, onInspect, onOpenNetwork, canInspect }: Props) {
  const { devices, networks, sequence, status } = useDaemon();
  const d = devices.get(deviceKey);
  const network = d?.id ? networks.get(d.id.networkId) : undefined;
  const networkId = d?.id?.networkId;
  const nodeId = d?.id?.nodeId;
  const canMutate = useCanMutate();
  const colors = useMemo(() => networkColorIndex(networks.values()), [networks]);
  const [names, setNames] = useState<DescribeDeviceResponse | null>(null);
  const [namesError, setNamesError] = useState<{ cause: unknown } | null>(null);
  const [attempt, setAttempt] = useState(0);
  const exit = useAction((stop: boolean) => (stop ? clearExitNode(networkId!) : setExitNode(networkId!, nodeId!)));

  useEffect(() => {
    if (!networkId || !nodeId) return;
    let cancelled = false;
    describeDevice(networkId, nodeId)
      .then((r) => {
        if (cancelled) return;
        setNames(r);
        setNamesError(null);
      })
      .catch((cause: unknown) => {
        if (cancelled) return;
        setNames(null);
        setNamesError({ cause });
      });
    return () => {
      cancelled = true;
    };
  }, [networkId, nodeId, sequence, attempt]);

  if (!d || !d.id) {
    return (
      <aside aria-label="Device details" className="flex min-h-0 flex-1 flex-col">
        <Header title="Device unavailable" onClose={onClose} />
        <p className="px-4 py-3 text-sm text-muted-foreground">
          This device is no longer reported. Its network may have disconnected or removed it.
        </p>
      </aside>
    );
  }

  const id = d.id;
  const name = d.hostname || d.dnsName;
  const dns = d.dnsName.replace(/\.$/, "");
  const seen = d.online ? null : lastSeen(d.lastSeen ? timestampDate(d.lastSeen) : undefined);
  const ssh = d.local ? null : sshTarget(d.dnsName, d.addresses);
  const destination = dns || d.addresses[0];
  const badges = deviceBadges(d);
  const stopping = d.exitNode;
  const namesRetry = namesError !== null && canRetryDescribe(namesError.cause, status);

  const toggleExit = async () => {
    const r = await exit.run(stopping);
    if (r.ok) toast.success(stopping ? `Stopped using ${name} as exit node` : `Using ${name} as exit node`);
    else toast.error(r.error);
  };

  return (
    <aside aria-label={`Details for ${name}`} className="flex min-h-0 flex-1 flex-col">
      <Header title={name} onClose={onClose}>
        <span className="flex items-center gap-2 text-xs text-muted-foreground">
          <span
            className={cn("size-2 shrink-0 rounded-full", d.online ? "bg-success" : "bg-muted-foreground/50")}
            aria-hidden="true"
          />
          {d.online ? "Online" : seen ? `Offline, last seen ${seen}` : "Offline"}
        </span>
      </Header>

      <div className="flex min-h-0 flex-1 flex-col overflow-y-auto">
        <div className="flex flex-col gap-3 px-4 py-3">
          {badges.length > 0 && (
            <div className="flex flex-wrap gap-1">
              <DeviceBadges badges={badges} />
            </div>
          )}
          <div className="flex items-center gap-3 rounded-md border border-border bg-muted/40 py-2 pr-1 pl-3">
            <span
              className="size-2.5 shrink-0 rounded-[3px]"
              style={{ background: networkColorVar(colors.get(id.networkId)) }}
              aria-hidden="true"
            />
            <span className="flex min-w-0 flex-1 flex-col">
              <span className="truncate text-sm font-medium">{network?.displayName ?? "Unknown network"}</span>
              {network && (
                <span className="truncate text-xs text-muted-foreground">
                  {providerName(network.provider)} · {stateLabel(network.state)}
                </span>
              )}
            </span>
            {network && (
              <Button
                variant="ghost"
                size="sm"
                aria-label={`Open ${network.displayName} network`}
                onClick={() => onOpenNetwork(network.id)}
              >
                Open
                <ArrowRight />
              </Button>
            )}
          </div>
          {((canInspect && destination) || ssh) && (
            <div className="flex flex-wrap gap-2">
              {canInspect && destination && (
                <Button size="sm" onClick={() => onInspect(destination)}>
                  <Route />
                  Inspect
                </Button>
              )}
              {ssh && (
                <Button size="sm" variant="outline" onClick={() => void copy(ssh)}>
                  <Terminal />
                  Copy SSH command
                </Button>
              )}
            </div>
          )}
        </div>

        <dl className="flex flex-col gap-3 border-t border-border px-4 py-3">
          {dns && (
            <Field label="DNS name">
              <CopyValue value={dns} label="DNS name" />
            </Field>
          )}
          {(names ? names.name : !namesError || namesRetry) && (
            <Field label="Flavor name">
              {names?.name ? (
                <CopyValue value={names.name} label="Flavor name" />
              ) : namesError ? (
                <dd className="flex items-center justify-between gap-2 text-sm text-destructive">
                  Could not load the Flavor name.
                  <Button
                    variant="ghost"
                    size="xs"
                    onClick={() => {
                      setNamesError(null);
                      setAttempt((a) => a + 1);
                    }}
                  >
                    Retry
                  </Button>
                </dd>
              ) : (
                <dd>
                  <Skeleton className="h-5 w-3/4" />
                </dd>
              )}
            </Field>
          )}
          {d.addresses.length > 0 && (
            <Field label={d.addresses.length === 1 ? "Address" : "Addresses"}>
              {d.addresses.map((a) => (
                <CopyValue key={a} value={a} label={`address ${a}`} />
              ))}
            </Field>
          )}
        </dl>

        {d.routes.length > 0 && (
          <dl className="flex flex-col gap-3 border-t border-border px-4 py-3">
            <Field label="Subnet routes">
              {d.routes.map((r) => (
                <dd key={r} className="flex min-h-6 items-center gap-2">
                  <span className="min-w-0 flex-1 font-mono text-sm wrap-anywhere">{r}</span>
                  {canInspect && (
                    <Button
                      variant="ghost"
                      size="xs"
                      aria-label={`Inspect route ${r}`}
                      onClick={() => onInspect(r.split("/")[0])}
                    >
                      Inspect
                    </Button>
                  )}
                </dd>
              ))}
            </Field>
          </dl>
        )}

        {(d.os || d.tags.length > 0) && (
          <dl className="flex flex-col gap-3 border-t border-border px-4 py-3">
            {d.os && (
              <Field label="OS">
                <dd className="text-sm">{d.os}</dd>
              </Field>
            )}
            {d.tags.length > 0 && (
              <Field label="Tags">
                <dd className="flex flex-wrap gap-1">
                  {d.tags.map((t) => (
                    <Badge key={t} variant="outline" className="rounded-md px-1.5 font-mono font-normal">
                      {t}
                    </Badge>
                  ))}
                </dd>
              </Field>
            )}
          </dl>
        )}

        {(d.exitNodeOption || d.exitNode) && !d.local && (
          <section aria-label="Exit node" className="flex flex-col gap-2 border-t border-border px-4 py-3">
            <h3 className="text-xs font-normal text-muted-foreground">Exit node</h3>
            <p className="text-sm">
              Connections through Flavor to addresses and names outside your networks{" "}
              {stopping ? "leave" : "would leave"} through this device.
            </p>
            <Button
              variant="outline"
              size="sm"
              className="self-start"
              disabled={!canMutate || exit.pending}
              onClick={() => void toggleExit()}
            >
              {exit.pending && <Loader2 className="animate-spin" />}
              {stopping ? "Stop using as exit node" : "Use as exit node"}
            </Button>
          </section>
        )}

        <details className="group border-t border-border">
          <summary className="flex cursor-pointer list-none items-center gap-1 px-4 py-3 text-xs text-muted-foreground transition-colors select-none hover:text-foreground focus-visible:ring-[3px] focus-visible:ring-ring/50 focus-visible:outline-none focus-visible:ring-inset [&::-webkit-details-marker]:hidden">
            <ChevronRight className="size-3.5 transition-transform group-open:rotate-90" aria-hidden="true" />
            Identifiers
          </summary>
          <dl className="flex flex-col gap-3 px-4 pb-4">
            <Field label="Network ID">
              <CopyValue value={id.networkId} label="network ID" />
            </Field>
            <Field label="Node ID">
              <CopyValue value={id.nodeId} label="node ID" />
            </Field>
            {names?.stableName && names.stableName !== names.name && (
              <Field label="Stable name">
                <CopyValue value={names.stableName} label="stable name" />
              </Field>
            )}
          </dl>
        </details>
      </div>
    </aside>
  );
}

function Header({ title, onClose, children }: { title: string; onClose: () => void; children?: ReactNode }) {
  return (
    <header className="flex items-start gap-3 border-b border-border py-3 pr-2 pl-4">
      <div className="flex min-w-0 flex-1 flex-col gap-1 pt-1">
        <h2 className="text-sm font-semibold wrap-anywhere">{title}</h2>
        {children}
      </div>
      <Button variant="ghost" size="icon-sm" onClick={onClose} aria-label="Close device details">
        <X />
      </Button>
    </header>
  );
}

function Field({ label, children }: { label: string; children: ReactNode }) {
  return (
    <div className="flex flex-col gap-1">
      <dt className="text-xs text-muted-foreground">{label}</dt>
      {children}
    </div>
  );
}

function CopyValue({ value, label }: { value: string; label: string }) {
  return (
    <dd className="flex min-h-6 items-center gap-2">
      <span className="min-w-0 flex-1 font-mono text-sm wrap-anywhere">
        {value.split(/(?<=\.)/).map((part, i) => (
          <Fragment key={i}>
            {i > 0 && <wbr />}
            {part}
          </Fragment>
        ))}
      </span>
      <Button variant="ghost" size="icon-xs" aria-label={`Copy ${label}`} onClick={() => void copy(value)}>
        <Copy />
      </Button>
    </dd>
  );
}

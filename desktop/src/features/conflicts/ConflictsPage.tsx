import { useEffect, useMemo, useRef, useState } from "react";
import {
  Check,
  CircleAlert,
  CircleCheck,
  Copy,
  Ellipsis,
  Info,
  LoaderCircle,
  Monitor,
  Network as NetworkIcon,
  Route,
  TriangleAlert,
  X,
} from "lucide-react";
import { motion } from "motion/react";
import { toast } from "sonner";
import {
  ConflictType,
  type Conflict,
  type ConflictMember,
  type ListConflictsResponse,
} from "@gen/flavor/v1/conflicts_pb";
import { listConflicts, setDestinationPreference } from "../../lib/api/daemon";
import { errorMessage } from "../../lib/api/errors";
import { useCanMutate, useDaemon } from "../../app/sync/useDaemon";
import { cn } from "@/lib/cn";
import { networkColorIndex, networkColorVar } from "@/lib/networkColor";
import { Alert, AlertDescription, AlertTitle } from "@/ui/alert";
import { Badge } from "@/ui/badge";
import { Button } from "@/ui/button";
import { Card } from "@/ui/card";
import { DropdownMenu, DropdownMenuContent, DropdownMenuItem, DropdownMenuTrigger } from "@/ui/dropdown-menu";
import { Select, SelectContent, SelectItem, SelectTrigger, SelectValue } from "@/ui/select";
import { Skeleton } from "@/ui/skeleton";
import { ToggleGroup, ToggleGroupItem } from "@/ui/toggle-group";
import {
  category,
  counts,
  describe,
  filterConflicts,
  groupByKind,
  kindLabel,
  typeLabel,
  type Kind,
  type SeverityFilter,
  type TypeFilter,
} from "./describe";
import { heat, overlapMatrix, shares, type Overlap } from "./overlap";

type Props = {
  onInspect: (destination: string) => void;
  onOpenNetwork: (networkId: string) => void;
  onShowDevice: (networkId: string, nodeId: string) => void;
  canPrefer: boolean;
};

type Pair = [string, string];

type Pending = { id: string; networkId: string; sent: boolean };

const kindTone: Record<Kind, string> = {
  ambiguous: "border-warning/40 bg-warning/10 [&>svg]:text-warning",
  expected: "text-muted-foreground",
  resolved: "border-success/40 bg-success/10 [&>svg]:text-success",
};

const kindIcon: Record<Kind, typeof Check> = {
  ambiguous: TriangleAlert,
  expected: Info,
  resolved: Check,
};

const heatTone = ["", "bg-foreground/5", "bg-foreground/10", "bg-foreground/20"];

const memberCols = "@2xl:grid-cols-[9rem_minmax(0,1fr)_minmax(0,1.25fr)_8rem]";

const plural = (n: number, word: string) => `${n} ${word}${n === 1 ? "" : "s"}`;

const copy = (text: string) =>
  navigator.clipboard.writeText(text).then(
    () => toast.success(`Copied ${text}`),
    () => toast.error("Could not copy"),
  );

function Swatch({ color }: { color: string }) {
  return <span className="size-2.5 shrink-0 rounded-[3px]" style={{ background: color }} aria-hidden="true" />;
}

export function ConflictsPage({ onInspect, onOpenNetwork, onShowDevice, canPrefer }: Props) {
  const { sequence, status, networks } = useDaemon();
  const [data, setData] = useState<ListConflictsResponse | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const [attempt, setAttempt] = useState(0);
  const [severity, setSeverity] = useState<SeverityFilter>("all");
  const [type, setType] = useState<TypeFilter>("all");
  const [pair, setPair] = useState<Pair | null>(null);
  const [pending, setPending] = useState<Pending | null>(null);

  useEffect(() => {
    if (status !== "ready") {
      setPending((p) => (p?.sent ? null : p));
      return;
    }
    let cancelled = false;
    const t = setTimeout(() => {
      setLoading(true);
      listConflicts()
        .then((r) => {
          if (!cancelled) {
            setData(r);
            setError(null);
          }
        })
        .catch((e) => !cancelled && setError(errorMessage(e)))
        .finally(() => {
          if (cancelled) return;
          setLoading(false);
          setPending((p) => (p?.sent ? null : p));
        });
    }, 250);
    return () => {
      cancelled = true;
      clearTimeout(t);
    };
  }, [sequence, status, attempt]);

  const refocus = useRef<string | null>(null);
  const heading = useRef<HTMLHeadingElement>(null);
  useEffect(() => {
    const id = refocus.current;
    if (!id || pending) return;
    refocus.current = null;
    if (!document.activeElement || document.activeElement === document.body) {
      document.getElementById(`conflict-${id}`)?.focus();
    }
  }, [pending]);

  const prefer = async (c: Conflict, networkId: string, name: string) => {
    setPending({ id: c.id, networkId, sent: false });
    try {
      await setDestinationPreference(c.value, networkId);
      setPending({ id: c.id, networkId, sent: true });
      refocus.current = c.id;
      setAttempt((a) => a + 1);
      toast.success(`Flavor now prefers ${name} for ${c.value}`);
    } catch (e) {
      setPending(null);
      toast.error(`Could not prefer ${name}: ${errorMessage(e)}`);
    }
  };

  const colors = useMemo(() => networkColorIndex(networks.values()), [networks]);
  const colorOf = (id: string) => networkColorVar(colors.get(id));
  const nameOf = (id: string) => networks.get(id)?.displayName ?? id;

  const all = data?.conflicts ?? [];
  const n = counts(all);
  const base = filterConflicts(all, severity, type);
  const matrix = overlapMatrix(base);
  const shown = base.filter((c) => !pair || shares(c, pair[0], pair[1]));
  const filtered = severity !== "all" || type !== "all" || pair !== null;
  const clearFilters = () => {
    setSeverity("all");
    setType("all");
    setPair(null);
  };

  const severityOptions: [SeverityFilter, string, number][] = [
    ["all", "All", all.length],
    ["ambiguous", "Ambiguous", n.ambiguous],
    ["expected", "Expected", n.expected],
    ...(canPrefer ? [["resolved", "Resolved", n.resolved] as [SeverityFilter, string, number]] : []),
  ];

  return (
    <div className="mx-auto flex w-full max-w-5xl flex-col gap-6">
      <header className="flex items-start justify-between gap-4">
        <div className="flex flex-col gap-1">
          <h1 ref={heading} tabIndex={-1} className="text-xl font-semibold outline-none">
            Conflicts
          </h1>
          <p className="text-sm text-muted-foreground">
            Addresses and names that exist more than once across your connected networks.
          </p>
        </div>
        {loading && data && (
          <span role="status" className="flex items-center gap-2 pt-1 text-xs text-muted-foreground">
            <LoaderCircle className="size-3.5 animate-spin" aria-hidden="true" />
            Updating
          </span>
        )}
      </header>

      {error && (
        <Alert variant="destructive">
          <CircleAlert />
          <AlertTitle>{data ? "Could not refresh conflicts" : "Could not load conflicts"}</AlertTitle>
          <AlertDescription>
            <p>{error}</p>
            <Button
              size="sm"
              variant="outline"
              className="mt-2 text-foreground"
              disabled={loading}
              onClick={() => {
                heading.current?.focus();
                setAttempt((a) => a + 1);
              }}
            >
              {loading && <LoaderCircle className="animate-spin" aria-hidden="true" />}
              Try again
            </Button>
          </AlertDescription>
        </Alert>
      )}

      {!data ? (
        !error && (
          <div className="flex flex-col gap-3" aria-busy="true" aria-label="Loading conflicts">
            <Skeleton className="h-8 w-72" />
            <Skeleton className="h-36 w-full rounded-xl" />
            <Skeleton className="h-36 w-full rounded-xl" />
          </div>
        )
      ) : all.length === 0 ? (
        <Card className="items-center gap-2 py-10 text-center">
          <CircleCheck className="size-5 text-success" aria-hidden="true" />
          <p className="text-sm font-medium">Nothing overlaps</p>
          <p className="text-sm text-muted-foreground">Every address and name on your connected networks is unique.</p>
        </Card>
      ) : (
        <>
          <div className="flex flex-wrap items-center gap-2">
            <ToggleGroup
              type="single"
              variant="outline"
              size="sm"
              value={severity}
              onValueChange={(v) => v && setSeverity(v as SeverityFilter)}
              aria-label="Filter by kind"
            >
              {severityOptions.map(([value, label, count]) => (
                <ToggleGroupItem key={value} value={value} className="px-2.5">
                  {label}
                  <span className="font-mono text-xs text-muted-foreground">{count}</span>
                </ToggleGroupItem>
              ))}
            </ToggleGroup>
            <Select
              value={String(type)}
              onValueChange={(v) => setType(v === "all" ? "all" : (Number(v) as ConflictType))}
            >
              <SelectTrigger size="sm" aria-label="Conflict type" className="w-36">
                <SelectValue />
              </SelectTrigger>
              <SelectContent position="popper" align="start">
                <SelectItem value="all">All types</SelectItem>
                <SelectItem value={String(ConflictType.ADDRESS_COLLISION)}>Addresses</SelectItem>
                <SelectItem value={String(ConflictType.DNS_NAME_COLLISION)}>DNS names</SelectItem>
                <SelectItem value={String(ConflictType.HOSTNAME_COLLISION)}>Device names</SelectItem>
                <SelectItem value={String(ConflictType.SUBNET_OVERLAP)}>Subnet routes</SelectItem>
              </SelectContent>
            </Select>
            {pair && (
              <Button size="sm" variant="secondary" onClick={() => setPair(null)}>
                <span className="sr-only">Clear filter:</span>
                <Swatch color={colorOf(pair[0])} />
                {nameOf(pair[0])}
                {pair[0] !== pair[1] && (
                  <>
                    <span className="text-muted-foreground">and</span>
                    <Swatch color={colorOf(pair[1])} />
                    {nameOf(pair[1])}
                  </>
                )}
                <X className="text-muted-foreground" aria-hidden="true" />
              </Button>
            )}
          </div>

          {matrix.networks.length > 1 && (
            <OverlapMatrix matrix={matrix} pair={pair} onPair={setPair} colorOf={colorOf} />
          )}

          {shown.length === 0 ? (
            <Card className="items-center gap-3 py-10 text-center">
              <p className="text-sm text-muted-foreground">No conflicts match these filters.</p>
              {filtered && (
                <Button size="sm" variant="outline" onClick={clearFilters}>
                  Clear filters
                </Button>
              )}
            </Card>
          ) : (
            groupByKind(shown).map(([kind, list]) => (
              <section key={kind} aria-labelledby={`conflicts-${kind}`} className="flex flex-col gap-2">
                <h2 id={`conflicts-${kind}`} className="flex items-baseline gap-2 text-sm font-medium text-muted-foreground">
                  {kindLabel[kind]}
                  <span className="font-mono text-xs">{list.length}</span>
                </h2>
                <ul className="flex flex-col gap-3">
                  {list.map((c, i) => (
                    <motion.li
                      key={c.id}
                      initial={{ opacity: 0, y: 4 }}
                      animate={{ opacity: 1, y: 0 }}
                      transition={{ delay: Math.min(i, 6) * 0.04, duration: 0.18 }}
                    >
                      <ConflictCard
                        conflict={c}
                        colorOf={colorOf}
                        onInspect={onInspect}
                        onOpenNetwork={onOpenNetwork}
                        onShowDevice={onShowDevice}
                        canPrefer={canPrefer}
                        pending={pending}
                        onPrefer={prefer}
                      />
                    </motion.li>
                  ))}
                </ul>
              </section>
            ))
          )}
        </>
      )}

      {data && data.notInspected.length > 0 && (
        <p className="text-xs text-muted-foreground">
          Not checked because {data.notInspected.length === 1 ? "it is" : "they are"} not connected:{" "}
          {data.notInspected.map((x) => x.displayName).join(", ")}.
        </p>
      )}
    </div>
  );
}

function OverlapMatrix({
  matrix,
  pair,
  onPair,
  colorOf,
}: {
  matrix: Overlap;
  pair: Pair | null;
  onPair: (pair: Pair | null) => void;
  colorOf: (id: string) => string;
}) {
  const nets = matrix.networks;
  return (
    <section aria-labelledby="overlap-heading" className="flex flex-col gap-2">
      <div className="flex flex-col gap-1">
        <h2 id="overlap-heading" className="text-sm font-medium text-muted-foreground">
          Overlap between networks
        </h2>
        <p id="overlap-hint" className="text-xs text-muted-foreground">
          Select a number to filter. The diagonal counts conflicts inside one network.
        </p>
      </div>
      <Card className="w-fit max-w-full p-3">
        <div className="overflow-x-auto">
          <table className="border-separate border-spacing-1 text-xs" aria-describedby="overlap-hint">
            <thead>
              <tr>
                <td />
                {nets.map((b) => (
                  <th key={b.id} scope="col" className="px-1 pb-1 text-left font-medium">
                    <span className="flex max-w-24 items-center gap-1.5" title={b.name}>
                      <Swatch color={colorOf(b.id)} />
                      <span className="truncate">{b.name}</span>
                    </span>
                  </th>
                ))}
              </tr>
            </thead>
            <tbody>
              {nets.map((a, i) => (
                <tr key={a.id}>
                  <th scope="row" className="pr-3 text-left font-medium">
                    <span className="flex max-w-32 items-center gap-1.5" title={a.name}>
                      <Swatch color={colorOf(a.id)} />
                      <span className="truncate">{a.name}</span>
                    </span>
                  </th>
                  {nets.map((b, j) => {
                    if (j > i) return <td key={b.id} />;
                    const count = matrix.counts[i][j];
                    const selected =
                      pair !== null &&
                      ((pair[0] === a.id && pair[1] === b.id) || (pair[0] === b.id && pair[1] === a.id));
                    const label =
                      i === j
                        ? `${plural(count, "conflict")} inside ${a.name}`
                        : `${plural(count, "conflict")} shared by ${a.name} and ${b.name}`;
                    return (
                      <td key={b.id} className="p-0">
                        {count === 0 ? (
                          <span className="flex h-8 w-14 items-center justify-center rounded-md font-mono text-muted-foreground">
                            0
                          </span>
                        ) : (
                          <button
                            type="button"
                            aria-pressed={selected}
                            aria-label={label}
                            title={label}
                            onClick={() => onPair(selected ? null : [a.id, b.id])}
                            className={cn(
                              "flex h-8 w-14 items-center justify-center rounded-md font-mono text-sm font-medium transition-shadow hover:ring-1 hover:ring-foreground/30 focus-visible:ring-[3px] focus-visible:ring-ring/50 focus-visible:outline-none",
                              heatTone[heat(count, matrix.max)],
                              selected && "ring-2 ring-foreground hover:ring-2 hover:ring-foreground",
                            )}
                          >
                            {count}
                          </button>
                        )}
                      </td>
                    );
                  })}
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      </Card>
    </section>
  );
}

function ConflictCard({
  conflict: c,
  colorOf,
  onInspect,
  onOpenNetwork,
  onShowDevice,
  canPrefer,
  pending,
  onPrefer,
}: {
  conflict: Conflict;
  colorOf: (id: string) => string;
  pending: Pending | null;
  onPrefer: (c: Conflict, networkId: string, name: string) => void;
} & Props) {
  const kind = category(c);
  const KindIcon = kindIcon[kind];
  const preferredName = c.members.find((m) => m.network?.id === c.preferredNetworkId)?.network?.displayName;
  const spans = new Set(c.members.map((m) => m.network?.id)).size > 1;
  const subnet = c.type === ConflictType.SUBNET_OVERLAP;
  const target = subnet ? c.sampleAddress : c.value;

  return (
    <Card className="@container gap-0 overflow-hidden py-0">
      <div className="flex items-start gap-4 px-4 py-3">
        <div className="flex min-w-0 flex-1 flex-col gap-1">
          <div className="flex flex-wrap items-center gap-x-2 gap-y-1">
            <h3
              id={`conflict-${c.id}`}
              tabIndex={-1}
              className="rounded-sm font-mono text-sm font-medium break-all outline-none focus-visible:ring-[3px] focus-visible:ring-ring/50"
            >
              {c.value}
            </h3>
            <span className="text-xs text-muted-foreground">{typeLabel[c.type]}</span>
            <Badge variant="outline" className={kindTone[kind]}>
              <KindIcon aria-hidden="true" />
              {kindLabel[kind]}
            </Badge>
          </div>
          <p className="text-sm text-muted-foreground">{describe(c)}</p>
          {preferredName && (
            <p className="text-xs text-muted-foreground">
              Flavor prefers {preferredName} for {c.value} in Flavor names (including system-wide names), forward, SOCKS
              and explain. Kernel routes for raw tailnet addresses are unchanged.
            </p>
          )}
        </div>
        {target && (
          <Button size="sm" variant="outline" aria-label={`Inspect ${target}`} onClick={() => onInspect(target)}>
            <Route aria-hidden="true" />
            {subnet ? `Inspect ${target}` : "Inspect"}
          </Button>
        )}
      </div>
      <div role="table" aria-label={`Devices sharing ${c.value}`}>
        <div
          role="row"
          className={cn(
            "grid gap-x-4 border-t border-border bg-muted/40 px-4 py-2 text-xs text-muted-foreground @max-2xl:sr-only",
            memberCols,
          )}
        >
          <span role="columnheader">Network</span>
          <span role="columnheader">Device</span>
          <span role="columnheader">{subnet ? "Route" : "Network-specific name"}</span>
          <span role="columnheader" className="sr-only">
            Actions
          </span>
        </div>
        {c.members.map((m) => (
          <MemberRow
            key={`${m.device?.id?.networkId}:${m.device?.id?.nodeId}`}
            conflict={c}
            member={m}
            subnet={subnet}
            color={colorOf(m.network?.id ?? "")}
            canPreferHere={canPrefer && spans && !subnet}
            pending={pending}
            onPrefer={(networkId, name) => onPrefer(c, networkId, name)}
            onOpenNetwork={onOpenNetwork}
            onShowDevice={onShowDevice}
          />
        ))}
      </div>
    </Card>
  );
}

function MemberRow({
  conflict: c,
  member: m,
  subnet,
  color,
  canPreferHere,
  pending,
  onPrefer,
  onOpenNetwork,
  onShowDevice,
}: {
  conflict: Conflict;
  member: ConflictMember;
  subnet: boolean;
  color: string;
  canPreferHere: boolean;
  pending: Pending | null;
  onPrefer: (networkId: string, name: string) => void;
  onOpenNetwork: (networkId: string) => void;
  onShowDevice: (networkId: string, nodeId: string) => void;
}) {
  const canMutate = useCanMutate();
  const d = m.device;
  const net = m.network;
  if (!d || !net) return null;
  const name = d.hostname || d.dnsName;
  const preferred = net.id === c.preferredNetworkId;
  return (
    <div
      role="row"
      className={cn(
        "grid grid-cols-[minmax(0,1fr)_auto] items-center gap-x-4 gap-y-1 border-t border-border px-4 py-3",
        memberCols,
      )}
    >
      <div role="cell" className="flex min-w-0 items-center gap-2">
        <Swatch color={color} />
        <span className="truncate text-sm font-medium">{net.displayName}</span>
      </div>
      <div role="cell" className="col-start-1 row-start-2 flex min-w-0 flex-col @2xl:col-start-auto @2xl:row-start-auto">
        <span className="flex min-w-0 items-center gap-2 text-sm">
          <span className="truncate">{name}</span>
          {d.local && (
            <Badge variant="outline" className="text-muted-foreground">
              This device
            </Badge>
          )}
        </span>
        <span className="truncate font-mono text-xs text-muted-foreground" title={d.addresses.join(", ")}>
          {d.addresses[0]}
          {d.addresses.length > 1 ? ` +${d.addresses.length - 1}` : ""}
        </span>
      </div>
      <div
        role="cell"
        className="col-start-1 row-start-3 flex min-w-0 items-center gap-1 @2xl:col-start-auto @2xl:row-start-auto"
      >
        <span className="mr-1 shrink-0 text-xs text-muted-foreground @2xl:hidden" aria-hidden="true">
          {subnet ? "Route" : "Network-specific name"}
        </span>
        {subnet ? (
          <span className="truncate font-mono text-xs">{m.route}</span>
        ) : m.uniqueName ? (
          <>
            <span className="truncate font-mono text-xs" title={m.uniqueName}>
              {m.uniqueName}
            </span>
            <Button
              size="icon-xs"
              variant="ghost"
              aria-label={`Copy ${m.uniqueName}`}
              onClick={() => void copy(m.uniqueName)}
            >
              <Copy aria-hidden="true" />
            </Button>
          </>
        ) : (
          <span className="text-xs text-muted-foreground">None</span>
        )}
      </div>
      <div
        role="cell"
        className="col-start-2 row-span-3 row-start-1 flex items-center justify-end gap-1 @2xl:col-start-auto @2xl:row-span-1 @2xl:row-start-auto"
      >
        {preferred ? (
          <Badge variant="outline" className={kindTone.resolved}>
            <Check aria-hidden="true" />
            Preferred
          </Badge>
        ) : (
          canPreferHere && (
            <Button
              size="sm"
              variant="outline"
              disabled={!canMutate || pending !== null}
              aria-label={`Prefer ${net.displayName} for ${c.value}`}
              onClick={() => onPrefer(net.id, net.displayName)}
            >
              {pending?.id === c.id && pending.networkId === net.id && (
                <LoaderCircle className="animate-spin" aria-hidden="true" />
              )}
              Prefer
            </Button>
          )
        )}
        <DropdownMenu>
          <DropdownMenuTrigger asChild>
            <Button size="icon-sm" variant="ghost" aria-label={`More actions for ${name} on ${net.displayName}`}>
              <Ellipsis aria-hidden="true" />
            </Button>
          </DropdownMenuTrigger>
          <DropdownMenuContent align="end">
            <DropdownMenuItem disabled={!d.id} onSelect={() => d.id && onShowDevice(net.id, d.id.nodeId)}>
              <Monitor aria-hidden="true" />
              Show {name} in Devices
            </DropdownMenuItem>
            <DropdownMenuItem onSelect={() => onOpenNetwork(net.id)}>
              <NetworkIcon aria-hidden="true" />
              Open {net.displayName}
            </DropdownMenuItem>
          </DropdownMenuContent>
        </DropdownMenu>
      </div>
    </div>
  );
}

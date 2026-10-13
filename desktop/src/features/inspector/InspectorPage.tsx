import { useEffect, useMemo, useRef, useState, type ReactNode, type RefObject } from "react";
import {
  ArrowRight,
  CircleAlert,
  CircleCheck,
  CircleSlash,
  Copy,
  Laptop,
  Loader2,
  Pin,
  RefreshCw,
  Route,
  Search,
  Split,
  TriangleAlert,
} from "lucide-react";
import { motion } from "motion/react";
import { toast } from "sonner";
import {
  CandidateStatus,
  DestinationKind,
  MatchKind,
  ResolutionDecision,
  type InspectDestinationResponse,
  type ResolutionCandidate,
} from "@gen/flavor/v1/inspector_pb";
import { stateLabel } from "@/lib/networkState";
import { deleteDestinationPreference, inspectDestination, setDestinationPreference } from "../../lib/api/daemon";
import { errorMessage } from "../../lib/api/errors";
import { useCanMutate, useDaemon } from "../../app/sync/useDaemon";
import { useAction } from "../../app/useAction";
import { providerName } from "../networks/format";
import { cn } from "@/lib/cn";
import { networkColorIndex, networkColorVar } from "@/lib/networkColor";
import { Alert } from "@/ui/alert";
import { Badge } from "@/ui/badge";
import { Button } from "@/ui/button";
import { Card } from "@/ui/card";
import { Input } from "@/ui/input";
import { Label } from "@/ui/label";
import { Skeleton } from "@/ui/skeleton";
import { candidateLabel, candidatePath, explain, matchLabel, preferenceNote } from "./explain";
import { exampleDestinations } from "./suggest";

type Props = {
  query: string;
  onQueryChange: (q: string) => void;
  onOpenNetwork: (networkId: string) => void;
  onShowDevice: (networkId: string, nodeId: string) => void;
  canPrefer: boolean;
};

const softDisabled = "aria-disabled:pointer-events-none aria-disabled:opacity-50";

const decisionBadge: Record<ResolutionDecision, [string, string, ReactNode]> = {
  [ResolutionDecision.UNSPECIFIED]: ["", "", null],
  [ResolutionDecision.UNIQUE]: ["Unique", "bg-success/15 text-success", <CircleCheck />],
  [ResolutionDecision.AMBIGUOUS]: ["Needs network context", "bg-warning/15 text-warning", <Split />],
  [ResolutionDecision.NO_MATCH]: ["No match", "bg-muted text-muted-foreground", <CircleSlash />],
};

const statusTone: Record<CandidateStatus, string> = {
  [CandidateStatus.UNSPECIFIED]: "bg-muted text-muted-foreground",
  [CandidateStatus.SELECTED]: "bg-success/15 text-success",
  [CandidateStatus.TIED]: "bg-warning/15 text-warning",
  [CandidateStatus.OUTRANKED]: "bg-muted text-muted-foreground",
};

const copy = (text: string) =>
  navigator.clipboard.writeText(text).then(
    () => toast.success(`Copied ${text}`),
    () => toast.error("Could not copy"),
  );

const Chip = ({ color, className }: { color: string; className?: string }) => (
  <span className={cn("size-2.5 shrink-0 rounded-[3px]", className)} style={{ background: color }} aria-hidden="true" />
);

const Spin = ({ busy, icon }: { busy: boolean; icon?: ReactNode }) =>
  busy ? <Loader2 className="animate-spin" aria-hidden="true" /> : (icon ?? null);

export function InspectorPage({ query, onQueryChange, onOpenNetwork, onShowDevice, canPrefer }: Props) {
  const { sequence, status, preferences, networks, devices } = useDaemon();
  const canMutate = useCanMutate();
  const [pending, setPending] = useState<string | null>(null);
  const [result, setResult] = useState<InspectDestinationResponse | null>(null);
  const [lastQuery, setLastQuery] = useState("");
  const [announcement, setAnnouncement] = useState("");
  const [refocus, setRefocus] = useState<{ to: RefObject<HTMLElement | null> } | null>(null);
  const run = useAction(inspectDestination);
  const input = useRef<HTMLInputElement>(null);
  const heading = useRef<HTMLHeadingElement>(null);
  const savedHeading = useRef<HTMLHeadingElement>(null);
  const seq = useRef(0);
  const emitted = useRef(query);
  const colors = useMemo(() => networkColorIndex(networks.values()), [networks]);
  const examples = useMemo(() => exampleDestinations(devices.values()), [devices]);
  const colorOf = (id?: string) => networkColorVar(id ? colors.get(id) : undefined);

  const busy = run.pending || pending !== null;

  const submit = async (q: string) => {
    const t = q.trim();
    if (!t) return;
    const id = ++seq.current;
    setLastQuery(t);
    setAnnouncement(`Inspecting ${t}`);
    const r = await run.run(t);
    if (id !== seq.current) return;
    if (r.ok) setResult(r.value);
    setAnnouncement(r.ok ? `${t}: ${explain(r.value).title}` : `Could not inspect ${t}`);
    setRefocus({ to: r.ok ? heading : input });
  };

  const change = (q: string) => {
    emitted.current = q;
    onQueryChange(q);
  };

  const inspect = (q: string) => {
    if (!busy && status === "ready") void submit(q);
  };

  const pick = (q: string) => {
    if (busy || status !== "ready") return;
    change(q);
    void submit(q);
  };

  useEffect(() => {
    if (query && status === "ready" && !result) void submit(query);
    else input.current?.focus();
  }, []);

  useEffect(() => {
    if (query === emitted.current) return;
    emitted.current = query;
    if (status === "ready") void submit(query);
  }, [query]);

  useEffect(() => {
    const active = document.activeElement;
    if (refocus && (!active || active === document.body)) refocus.to.current?.focus();
  }, [refocus]);

  const stale = result !== null && sequence > result.snapshotSequence;
  const summary = result && explain(result);
  const note = result && preferenceNote(result);
  const spansNetworks = result !== null && new Set(result.candidates.map((c) => c.network?.id)).size > 1;
  const preferredId = result?.preference?.network?.id;
  const saved = [...preferences.values()].sort((a, b) => a.destination.localeCompare(b.destination));
  const paths =
    result?.decision === ResolutionDecision.UNIQUE
      ? result.candidates.filter((c) => c.status === CandidateStatus.SELECTED)
      : result?.decision === ResolutionDecision.AMBIGUOUS
        ? result.candidates.filter((c) => c.status === CandidateStatus.TIED)
        : [];

  const changePreference = async (key: string, fn: () => Promise<unknown>, done: string) => {
    if (busy) return false;
    const before = seq.current;
    setPending(key);
    try {
      await fn();
      toast.success(done);
      if (result && seq.current === before) await submit(result.query);
      return true;
    } catch (e) {
      toast.error(errorMessage(e));
      return false;
    } finally {
      setPending(null);
    }
  };
  const prefer = (networkId: string, name: string) =>
    result &&
    changePreference(
      `prefer:${networkId}`,
      () => setDestinationPreference(result.normalized, networkId),
      `Flavor now uses ${name} for ${result.normalized}`,
    );
  const unprefer = (destination: string) =>
    changePreference(
      `remove:${destination}`,
      () => deleteDestinationPreference(destination),
      `Removed your preference for ${destination}`,
    );

  const [decisionText, decisionTone, decisionIcon] =
    decisionBadge[result?.decision ?? ResolutionDecision.UNSPECIFIED] ?? decisionBadge[ResolutionDecision.UNSPECIFIED];

  return (
    <div className="mx-auto flex w-full max-w-5xl flex-col gap-6">
      <header className="flex flex-col gap-1">
        <h1 className="text-xl font-semibold">Connection inspector</h1>
        <p className="text-sm text-muted-foreground">
          See which of your networks a destination belongs to, and why.
        </p>
      </header>
      <p role="status" className="sr-only">
        {announcement}
      </p>

      <form
        className="flex flex-col gap-2"
        onSubmit={(e) => {
          e.preventDefault();
          inspect(query);
        }}
      >
        <Label htmlFor="inspect-destination">What do you want to reach?</Label>
        <div className="flex gap-2">
          <div className="relative flex-1">
            <Search
              className="pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2 text-muted-foreground"
              aria-hidden="true"
            />
            <Input
              id="inspect-destination"
              ref={input}
              className="h-11 bg-card pl-9 font-mono text-base md:text-base"
              value={query}
              onChange={(e) => change(e.target.value)}
              placeholder="100.64.0.1, db or db.lab.ts.net:5432"
              spellCheck={false}
              autoComplete="off"
            />
          </div>
          <Button
            type="submit"
            className={cn("h-11 px-6", softDisabled)}
            disabled={status !== "ready" || !query.trim()}
            aria-disabled={busy || undefined}
            aria-busy={run.pending || undefined}
          >
            <Spin busy={run.pending} />
            Inspect
          </Button>
        </div>
      </form>

      {run.error && (
        <Alert variant="destructive" className="flex items-center gap-3 [&>svg]:translate-y-0">
          <CircleAlert />
          <div className="flex min-w-0 flex-1 flex-col gap-1">
            <p>
              <span className="font-medium">Could not inspect {lastQuery}.</span> {run.error}
            </p>
            {result && (
              <p className="text-muted-foreground">The result below is from an earlier inspection of {result.normalized}.</p>
            )}
          </div>
          <Button
            size="sm"
            variant="outline"
            className={cn("text-foreground", softDisabled)}
            aria-disabled={busy || undefined}
            onClick={() => inspect(lastQuery)}
          >
            Try again
          </Button>
        </Alert>
      )}

      {run.pending && !result && <ResultSkeleton query={lastQuery} />}

      {result && summary && (
        <motion.div
          key={result.normalized}
          initial={{ opacity: 0, y: 4 }}
          animate={{ opacity: run.pending || run.error ? 0.6 : 1, y: 0 }}
          transition={{ duration: 0.18 }}
          className="flex flex-col gap-6"
          aria-busy={run.pending || undefined}
        >
          {stale && (
            <Alert className="flex items-center gap-3 [&>svg]:translate-y-0 [&>svg]:text-warning">
              <RefreshCw />
              <p className="min-w-0 flex-1">Your networks changed since this result was produced.</p>
              <Button
                size="sm"
                variant="outline"
                className={softDisabled}
                aria-disabled={busy || undefined}
                onClick={() => inspect(result.query)}
              >
                <Spin busy={run.pending} />
                Inspect again
              </Button>
            </Alert>
          )}

          <Card
            className={cn("gap-0 overflow-hidden py-0", result.decision === ResolutionDecision.AMBIGUOUS && "border-warning/50")}
            aria-labelledby="decision-title"
            role="region"
          >
            <div className="flex flex-col gap-3 p-4">
              <div className="flex flex-wrap items-center gap-2">
                <Badge className={cn("gap-1", decisionTone)}>
                  {decisionIcon}
                  {decisionText}
                </Badge>
                <span className="font-mono text-xs text-muted-foreground">
                  {result.kind === DestinationKind.ADDRESS ? "address" : "name"} {result.normalized}
                  {result.port ? `, port ${result.port}` : ""}
                </span>
              </div>
              <div className="flex flex-col gap-1">
                <h2
                  id="decision-title"
                  ref={heading}
                  tabIndex={-1}
                  className="rounded-sm text-base font-semibold outline-none focus-visible:ring-[3px] focus-visible:ring-ring/50"
                >
                  {summary.title}
                </h2>
                <p className="text-sm text-muted-foreground">{summary.detail}</p>
              </div>
              {note && (
                <p className="flex items-start gap-2 text-sm">
                  <TriangleAlert className="mt-0.5 size-4 shrink-0 text-warning" aria-hidden="true" />
                  {note}
                </p>
              )}
            </div>

            {paths.length > 0 && (
              <div className="@container flex flex-col gap-3 border-t bg-muted/40 p-4">
                <h3 className="text-xs font-medium text-muted-foreground">
                  {paths.length === 1
                    ? "Path"
                    : `${paths.length} possible paths. Flavor will not pick one on its own.`}
                </h3>
                <div className="flex flex-col items-start gap-2 @2xl:flex-row @2xl:items-center">
                  <HopBox caption="From">
                    <Laptop className="size-4 text-muted-foreground" aria-hidden="true" />
                    This computer
                  </HopBox>
                  <div className="grid min-w-0 grid-cols-[repeat(3,auto)] items-center gap-2">
                    {paths.map((c, i) => (
                      <ol
                        key={`${c.network?.id}:${c.device?.id?.nodeId}:${i}`}
                        className="col-span-full grid grid-cols-subgrid items-center"
                        aria-label={`Path from this computer through ${c.network?.displayName}`}
                      >
                        <Hop lead caption="Network" className={cn(paths.length > 1 && "border-warning/50")}>
                          <Chip color={colorOf(c.network?.id)} />
                          {c.network?.displayName}
                        </Hop>
                        {candidatePath(result, c).map((h) => (
                          <Hop key={`${h.caption}:${h.label}`} caption={h.caption} className={cn(h.address && "font-mono")}>
                            {h.label}
                          </Hop>
                        ))}
                      </ol>
                    ))}
                  </div>
                </div>
              </div>
            )}

            {result.systemRoute && (
              <p className="flex items-start gap-3 border-t px-4 py-3 text-sm">
                <Route className="mt-0.5 size-4 shrink-0 text-muted-foreground" aria-hidden="true" />
                <span>
                  Programs on this computer reach it over TCP and UDP through the route{" "}
                  <span className="font-mono">{result.systemRoute}</span>.{" "}
                  <span className="text-muted-foreground">Ping does not go through it.</span>
                </span>
              </p>
            )}

            {result.preference && canPrefer && (
              <div className="flex flex-wrap items-center gap-3 border-t px-4 py-3">
                <Pin className="size-4 shrink-0 text-muted-foreground" aria-hidden="true" />
                <p className="min-w-0 flex-1 text-sm">
                  <span className="inline-flex items-center gap-2 font-medium">
                    <Chip color={colorOf(result.preference.network?.id)} />
                    Flavor preference: {result.preference.network?.displayName}
                  </span>{" "}
                  <span className="text-muted-foreground">
                    Used by Flavor's decisions only. System routing is not changed.
                  </span>
                </p>
                <Button
                  size="sm"
                  variant="outline"
                  className={softDisabled}
                  disabled={!canMutate}
                  aria-disabled={busy || undefined}
                  aria-busy={pending === `remove:${result.preference.destination}` || undefined}
                  onClick={() => void unprefer(result.preference!.destination)}
                >
                  <Spin busy={pending === `remove:${result.preference.destination}`} />
                  Remove preference
                </Button>
              </div>
            )}

            {(result.port > 0 ||
              (result.notInspected.length > 0 && result.decision !== ResolutionDecision.NO_MATCH)) && (
              <div className="flex flex-col gap-1 border-t px-4 py-3 text-xs text-muted-foreground">
                {result.notInspected.length > 0 && result.decision !== ResolutionDecision.NO_MATCH && (
                  <p>
                    Not checked because {result.notInspected.length === 1 ? "it is" : "they are"} not connected:{" "}
                    {result.notInspected.map((n) => n.displayName).join(", ")}.
                  </p>
                )}
                {result.port > 0 && <p>Flavor notes the port but does not probe services.</p>}
              </div>
            )}
          </Card>

          {result.candidates.length > 0 && (
            <section aria-labelledby="candidates-title" className="flex flex-col gap-2">
              <div className="flex flex-col gap-1">
                <h2 id="candidates-title" className="text-sm font-medium text-muted-foreground">
                  {result.candidates.length} candidate{result.candidates.length === 1 ? "" : "s"}
                </h2>
                {canPrefer && spansNetworks && result.decision === ResolutionDecision.AMBIGUOUS && (
                  <p className="text-xs text-muted-foreground">
                    Prefer a network to have Flavor choose it whenever you use{" "}
                    <span className="font-mono">{result.normalized}</span>.
                  </p>
                )}
              </div>
              <Card className="gap-0 py-0">
                <ul>
                  {result.candidates.map((c, i) => {
                    const preferKey = `prefer:${c.network?.id}`;
                    return (
                      <CandidateRow
                        key={`${c.network?.id}:${c.device?.id?.nodeId}:${c.match}:${c.matchedValue}`}
                        index={i}
                        candidate={c}
                        color={colorOf(c.network?.id)}
                        label={candidateLabel(result, c.status, c.match)}
                        onOpenNetwork={onOpenNetwork}
                        onShowDevice={onShowDevice}
                        preferring={pending === preferKey}
                        preferDisabled={busy}
                        onPrefer={
                          canPrefer && spansNetworks && c.network && c.network.id !== preferredId && canMutate
                            ? () => void prefer(c.network!.id, c.network!.displayName)
                            : undefined
                        }
                      />
                    );
                  })}
                </ul>
              </Card>
            </section>
          )}
        </motion.div>
      )}

      {!result && !run.pending && !run.error && (
        <Card className="gap-0 py-0">
          <div className="flex flex-col gap-1 p-4">
            <h2 className="text-sm font-medium">
              {examples.length > 0 ? "Try a destination from your networks" : "Inspect an address or a name"}
            </h2>
            <p className="text-sm text-muted-foreground">
              Flavor checks every connected network for a device with that address, full DNS name or device name. The
              same address on two networks is normal; the inspector shows each one so you can tell them apart.
            </p>
          </div>
          {examples.length > 0 && (
            <ul>
              {examples.map((e, i) => (
                <motion.li
                  key={e.value}
                  initial={{ opacity: 0, y: 4 }}
                  animate={{ opacity: 1, y: 0 }}
                  transition={{ delay: i * 0.04, duration: 0.18 }}
                  className="border-t"
                >
                  <button
                    type="button"
                    disabled={status !== "ready"}
                    onClick={() => pick(e.value)}
                    className="group flex w-full items-center gap-3 px-4 py-3 text-left transition-colors hover:bg-accent focus-visible:bg-accent focus-visible:ring-[3px] focus-visible:ring-ring/50 focus-visible:outline-none focus-visible:ring-inset disabled:opacity-50"
                  >
                    <span className="flex min-w-0 flex-1 flex-col gap-1">
                      <span className="truncate font-mono text-sm">{e.value}</span>
                      <span className="flex flex-wrap items-center gap-x-3 gap-y-1 text-xs text-muted-foreground">
                        <span>{e.hint}</span>
                        {e.networkIds.map((id) => (
                          <span key={id} className="inline-flex items-center gap-1.5">
                            <Chip color={colorOf(id)} />
                            {networks.get(id)?.displayName}
                          </span>
                        ))}
                      </span>
                    </span>
                    <ArrowRight
                      className="size-4 shrink-0 text-muted-foreground transition-transform group-hover:translate-x-0.5"
                      aria-hidden="true"
                    />
                  </button>
                </motion.li>
              ))}
            </ul>
          )}
        </Card>
      )}

      {canPrefer && saved.length > 0 && (
        <section aria-labelledby="saved-preferences" className="flex flex-col gap-2">
          <div className="flex flex-col gap-1">
            <h2
              id="saved-preferences"
              ref={savedHeading}
              tabIndex={-1}
              className="rounded-sm text-sm font-medium text-muted-foreground outline-none focus-visible:ring-[3px] focus-visible:ring-ring/50"
            >
              Your preferences
            </h2>
            <p className="text-xs text-muted-foreground">
              When a destination exists on several networks, Flavor uses these to decide. They do not change system
              routing.
            </p>
          </div>
          <Card className="gap-0 py-0">
            <ul aria-label="Destination preferences">
              {saved.map((p) => {
                const network = networks.get(p.networkId);
                const key = `remove:${p.destination}`;
                return (
                  <li key={p.destination} className="flex items-center gap-3 border-t px-4 py-2 first:border-t-0">
                    <span className="min-w-0 flex-1 truncate font-mono text-sm">{p.destination}</span>
                    <span className="flex min-w-0 items-center gap-2 text-sm">
                      {network && <Chip color={colorOf(network.id)} />}
                      <span className={cn("truncate", !network && "text-muted-foreground")}>
                        {network?.displayName ?? "Removed network"}
                      </span>
                    </span>
                    <span className="flex shrink-0 gap-1">
                      <Button
                        size="sm"
                        variant="ghost"
                        className={softDisabled}
                        disabled={status !== "ready"}
                        aria-disabled={busy || undefined}
                        aria-label={`Inspect ${p.destination}`}
                        onClick={() => pick(p.destination)}
                      >
                        Inspect
                      </Button>
                      <Button
                        size="sm"
                        variant="ghost"
                        className={softDisabled}
                        disabled={!canMutate}
                        aria-disabled={busy || undefined}
                        aria-busy={pending === key || undefined}
                        aria-label={`Remove preference for ${p.destination}`}
                        onClick={async () => {
                          if (await unprefer(p.destination))
                            (saved.length > 1 ? savedHeading : input).current?.focus();
                        }}
                      >
                        <Spin busy={pending === key} />
                        Remove
                      </Button>
                    </span>
                  </li>
                );
              })}
            </ul>
          </Card>
        </section>
      )}
    </div>
  );
}

function ResultSkeleton({ query }: { query: string }) {
  return (
    <Card className="gap-0 py-0" role="status">
      <span className="sr-only">Inspecting {query}</span>
      <div className="flex flex-col gap-3 p-4">
        <Skeleton className="h-5 w-24 rounded-full" />
        <Skeleton className="h-5 w-2/3" />
        <Skeleton className="h-4 w-full" />
      </div>
      <div className="flex flex-wrap gap-2 border-t px-4 py-4">
        {[0, 1, 2, 3].map((i) => (
          <Skeleton key={i} className="h-12 w-28" />
        ))}
      </div>
    </Card>
  );
}

function Hop({
  caption,
  lead,
  className,
  children,
}: {
  caption: string;
  lead?: boolean;
  className?: string;
  children: ReactNode;
}) {
  return (
    <li className="flex min-w-0 items-center gap-2">
      <ArrowRight
        className={cn("size-4 shrink-0 text-muted-foreground", lead && "@max-2xl:hidden")}
        aria-hidden="true"
      />
      <HopBox caption={caption} className={cn("flex-1", className)}>
        {children}
      </HopBox>
    </li>
  );
}

function HopBox({ caption, className, children }: { caption: string; className?: string; children: ReactNode }) {
  return (
    <div className={cn("flex min-w-0 flex-col rounded-md border bg-card px-3 py-2", className)}>
      <span className="font-sans text-xs text-muted-foreground">{caption}</span>
      <span className="flex min-w-0 items-center gap-2 text-sm font-medium wrap-anywhere">{children}</span>
    </div>
  );
}

function CandidateRow({
  index,
  candidate: c,
  color,
  label,
  onOpenNetwork,
  onShowDevice,
  onPrefer,
  preferring,
  preferDisabled,
}: {
  index: number;
  candidate: ResolutionCandidate;
  color: string;
  label: string;
  onOpenNetwork: (id: string) => void;
  onShowDevice: (networkId: string, nodeId: string) => void;
  onPrefer?: () => void;
  preferring: boolean;
  preferDisabled: boolean;
}) {
  const d = c.device;
  const n = c.network;
  if (!d || !n) return null;
  const outranked = c.status === CandidateStatus.OUTRANKED;
  const recordOnly = c.match === MatchKind.DNS_RECORD && !d.hostname && !d.dnsName;
  const name = d.hostname || d.dnsName || (recordOnly ? c.matchedValue : "");
  const address =
    c.match === MatchKind.DEVICE_ADDRESS || c.match === MatchKind.DNS_RECORD ? c.matchedValue : d.addresses[0];
  const copyAddress = c.match === MatchKind.SUBNET_ROUTE ? "Copy router address" : "Copy address";
  return (
    <motion.li
      initial={{ opacity: 0, y: 4 }}
      animate={{ opacity: 1, y: 0 }}
      transition={{ delay: index * 0.04, duration: 0.18 }}
      className="flex flex-col gap-3 border-t px-4 py-4 first:border-t-0"
    >
      <div className={cn("flex flex-wrap items-center gap-x-3 gap-y-1", outranked && "text-muted-foreground")}>
        <Chip color={color} className={cn(outranked && "opacity-50")} />
        <span className="text-sm font-medium">{n.displayName}</span>
        <span className="text-xs text-muted-foreground">
          {providerName(n.provider)} · {stateLabel(n.state)}
        </span>
        {label && <Badge className={cn("ml-auto", statusTone[c.status])}>{label}</Badge>}
      </div>
      <dl
        className={cn(
          "grid grid-cols-[minmax(6rem,auto)_1fr] gap-x-4 gap-y-1 text-sm",
          outranked && "text-muted-foreground",
        )}
      >
        <dt className="text-muted-foreground">Device</dt>
        <dd className="flex min-w-0 flex-wrap items-center gap-2">
          <span className="truncate">{name}</span>
          {!recordOnly && (
            <span className="inline-flex items-center gap-1 text-xs text-muted-foreground">
              <span
                className={cn("size-1.5 rounded-full", d.online ? "bg-success" : "bg-muted-foreground")}
                aria-hidden="true"
              />
              {d.online ? "Online" : "Offline"}
            </span>
          )}
          {d.local && <Badge variant="outline">This device</Badge>}
        </dd>
        <dt className="text-muted-foreground">Matched</dt>
        <dd className="min-w-0">
          {matchLabel[c.match]} <span className="font-mono">{c.matchedValue}</span>
        </dd>
        {d.dnsName && (
          <>
            <dt className="text-muted-foreground">DNS name</dt>
            <dd className="min-w-0 truncate font-mono">{d.dnsName}</dd>
          </>
        )}
        <dt className="text-muted-foreground">Addresses</dt>
        <dd className="min-w-0 font-mono">{d.addresses.join(", ")}</dd>
        {c.qualifiedName && <CopyField label="Flavor name" value={c.qualifiedName} network={n.displayName} />}
        {c.stableName && c.stableName !== c.qualifiedName && (
          <CopyField label="Stable name" value={c.stableName} network={n.displayName} />
        )}
      </dl>
      <div className="flex flex-wrap items-center gap-2">
        <div className="-ml-2.5 flex flex-wrap gap-1">
          {address && (
            <Button
              size="sm"
              variant="ghost"
              aria-label={`${copyAddress} on ${n.displayName}`}
              onClick={() => void copy(address)}
            >
              <Copy />
              {copyAddress}
            </Button>
          )}
          {d.dnsName && (
            <Button
              size="sm"
              variant="ghost"
              aria-label={`Copy DNS name on ${n.displayName}`}
              onClick={() => void copy(d.dnsName)}
            >
              <Copy />
              Copy DNS name
            </Button>
          )}
        </div>
        <div className="ml-auto flex flex-wrap gap-2">
          {onPrefer && (
            <Button
              size="sm"
              variant="outline"
              className={softDisabled}
              aria-disabled={preferDisabled || undefined}
              aria-busy={preferring || undefined}
              onClick={onPrefer}
            >
              <Spin busy={preferring} icon={<Pin />} />
              Prefer {n.displayName}
            </Button>
          )}
          {d.id?.nodeId && (
            <Button
              size="sm"
              variant="outline"
              aria-label={`Show device ${name} on ${n.displayName}`}
              onClick={() => onShowDevice(n.id, d.id!.nodeId)}
            >
              Show device
            </Button>
          )}
          <Button
            size="sm"
            variant="outline"
            aria-label={`Open network ${n.displayName}`}
            onClick={() => onOpenNetwork(n.id)}
          >
            Open network
          </Button>
        </div>
      </div>
    </motion.li>
  );
}

function CopyField({ label, value, network }: { label: string; value: string; network: string }) {
  return (
    <>
      <dt className="text-muted-foreground">{label}</dt>
      <dd className="flex min-w-0 items-center gap-1">
        <span className="truncate font-mono" title={value}>
          {value}
        </span>
        <Button
          size="icon-xs"
          variant="ghost"
          className="-my-1"
          aria-label={`Copy ${label.toLowerCase()} on ${network}`}
          onClick={() => void copy(value)}
        >
          <Copy />
        </Button>
      </dd>
    </>
  );
}

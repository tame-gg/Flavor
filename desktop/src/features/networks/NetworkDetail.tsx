import { useEffect, useId, useRef, useState, type ReactNode } from "react";
import {
  ArrowRight,
  ChevronDown,
  CircleAlert,
  Clock,
  ExternalLink,
  KeyRound,
  Loader2,
  LogIn,
  type LucideIcon,
  Monitor,
  Plug,
  RotateCw,
  Trash2,
  TriangleAlert,
  Unplug,
  WifiOff,
} from "lucide-react";
import { motion } from "motion/react";
import { toast } from "sonner";
import { NetworkConnectionState as S, ProviderType } from "@gen/flavor/v1/common_pb";
import type { NetworkDNS } from "@gen/flavor/v1/dns_pb";
import { SubnetRouteUse, type SubnetRoute } from "@gen/flavor/v1/routes_pb";
import type { Network } from "@gen/flavor/v1/network_pb";
import { authDestination } from "../../lib/api/authUrl";
import {
  chooseSubnetRoutes,
  connectNetwork,
  getRoutes,
  offerExitNode,
  disconnectNetwork,
  enrollNetwork,
  listDNS,
  listSubnetRoutes,
  openAuthUrl,
  updateNetwork,
} from "../../lib/api/daemon";
import { useCanMutate, useDaemon } from "../../app/sync/useDaemon";
import { useAction } from "../../app/useAction";
import { errorMessage } from "../../lib/api/errors";
import { cn } from "@/lib/cn";
import { networkColorIndex, networkColorVar } from "@/lib/networkColor";
import { Alert, AlertDescription, AlertTitle } from "@/ui/alert";
import { Button } from "@/ui/button";
import { Card } from "@/ui/card";
import { Input } from "@/ui/input";
import { Label } from "@/ui/label";
import { Skeleton } from "@/ui/skeleton";
import { Switch } from "@/ui/switch";
import { Tabs, TabsContent, TabsList, TabsTrigger } from "@/ui/tabs";
import {
  controlHost,
  deviceSummary,
  isActive,
  isLive,
  networkSource,
  providerName,
  type Tone,
} from "./format";
import { StateBadge, ToneBadge } from "./StateBadge";

const row = "flex items-center gap-3 border-t border-border px-4 py-3";
const warnAlert = "border-warning/40 [&>svg]:text-warning";

const fail = (title: string, e: unknown) => {
  toast.error(title, { description: errorMessage(e) });
};

type Props = {
  network: Network;
  deviceCount: number;
  notice: string | null;
  onShowDevices: () => void;
  onRemove: () => void;
  tab?: string;
  onTabChange?: (tab: string) => void;
};

export function NetworkDetail({ network: n, deviceCount, notice, onShowDevices, onRemove, tab, onTabChange }: Props) {
  const canMutate = useCanMutate();
  const { status, networks } = useDaemon();
  const active = isActive(n.state);
  const live = isLive(n.state);
  const offline = status === "unavailable";
  const color = networkColorVar(networkColorIndex(networks.values()).get(n.id));
  const toggle = useAction(() =>
    (active ? disconnectNetwork(n.id) : connectNetwork(n.id)).catch((e) =>
      fail(`Could not ${active ? "disconnect" : "connect"} ${n.displayName}`, e),
    ),
  );
  const prompt = n.authentication;

  return (
    <motion.section
      aria-labelledby="network-title"
      initial={{ opacity: 0, y: 4 }}
      animate={{ opacity: 1, y: 0 }}
      transition={{ duration: 0.16 }}
      className="flex min-w-0 flex-col gap-4"
    >
      <header className="grid grid-cols-[auto_minmax(0,1fr)_auto] items-center gap-x-3 gap-y-1">
        <span className="size-2.5 rounded-[3px]" style={{ background: color }} aria-hidden="true" />
        <div className="flex min-w-0 items-center gap-2">
          <h2 id="network-title" className="truncate text-base font-semibold">
            {n.displayName}
          </h2>
          <StateBadge state={n.state} />
        </div>
        <Button
          size="sm"
          variant={active ? "outline" : "default"}
          disabled={!canMutate || toggle.pending || n.state === S.REMOVING}
          onClick={() => void toggle.run()}
        >
          {toggle.pending ? <Loader2 className="animate-spin" /> : active ? <Unplug /> : <Plug />}
          {active ? "Disconnect" : "Connect"}
        </Button>
        <p className="col-start-2 truncate text-xs text-muted-foreground">{networkSource(n)}</p>
      </header>

      {status === "unavailable" && (
        <Alert className={warnAlert}>
          <WifiOff />
          <AlertTitle>Showing last known state</AlertTitle>
          <AlertDescription>The Flavor daemon is not reachable.</AlertDescription>
        </Alert>
      )}
      {notice && (
        <Alert className={warnAlert}>
          <TriangleAlert />
          <AlertTitle>Added, but not connected</AlertTitle>
          <AlertDescription>{notice}</AlertDescription>
        </Alert>
      )}

      <Tabs value={tab} onValueChange={onTabChange} defaultValue="overview" className="gap-4">
        <TabsList variant="line" className="h-9 w-full justify-start rounded-none border-b border-border p-0">
          <TabsTrigger value="overview" className="flex-none px-3">
            Overview
          </TabsTrigger>
          <TabsTrigger value="routes" className="flex-none px-3">
            Routes
          </TabsTrigger>
          <TabsTrigger value="dns" className="flex-none px-3">
            DNS
          </TabsTrigger>
          <TabsTrigger value="settings" className="flex-none px-3">
            Settings
          </TabsTrigger>
        </TabsList>

        <TabsContent value="overview" className="flex flex-col gap-4">
          <StateNote network={n} />
          {prompt?.kind === "authentication" && <SignInPanel network={n} flowId={prompt.flowId} url={prompt.authUrl} />}
          {prompt?.kind === "approval" && (
            <Callout icon={Clock} iconClass="text-warning" title="Waiting for approval">
              An administrator must approve this device in the {providerName(n.provider)} admin console. Flavor connects
              automatically once it is approved.
            </Callout>
          )}
          {(n.state === S.AUTHENTICATING || n.state === S.DISCONNECTED || n.state === S.ERROR) && (
            <EnrollPanel network={n} />
          )}
          <Callout
            icon={Monitor}
            title="Devices"
            action={
              deviceCount > 0 && (
                <Button size="sm" variant="outline" onClick={onShowDevices}>
                  View devices
                  <ArrowRight />
                </Button>
              )
            }
          >
            {deviceSummary(deviceCount, active)}
          </Callout>
          <Card className="gap-0 py-0">
            <h3 className="px-4 py-3 text-sm font-medium">Details</h3>
            <dl>
              <Detail term="Provider">{providerName(n.provider)}</Detail>
              <Detail term="Control server">{controlHost(n)}</Detail>
              <Detail term="Device name" mono>
                {n.nodeHostname}
              </Detail>
              <Detail term="Network ID" mono>
                {n.id}
              </Detail>
            </dl>
          </Card>
        </TabsContent>

        <TabsContent value="routes" className="flex flex-col gap-4">
          {!live ? (
            <NotLive>
              Connect {n.displayName} to use its subnet routes or offer this computer as an exit node.
            </NotLive>
          ) : offline ? (
            <NotLive>Subnet routes and the exit node are unavailable while the Flavor daemon is not reachable.</NotLive>
          ) : (
            <>
              <SubnetRoutesCard key={`routes:${n.id}`} networkId={n.id} />
              <ExitOfferCard key={`exit:${n.id}`} networkId={n.id} />
            </>
          )}
        </TabsContent>

        <TabsContent value="dns" className="flex flex-col gap-4">
          {!live ? (
            <NotLive>Connect {n.displayName} to see the DNS records and resolvers it publishes.</NotLive>
          ) : offline ? (
            <NotLive>DNS is unavailable while the Flavor daemon is not reachable.</NotLive>
          ) : (
            <DNSCard key={`dns:${n.id}`} networkId={n.id} />
          )}
        </TabsContent>

        <TabsContent value="settings" className="flex flex-col gap-4">
          <Preferences network={n} />
          <Callout
            icon={Trash2}
            title="Remove network"
            action={
              <Button
                size="sm"
                variant="outline"
                className="text-destructive hover:text-destructive"
                disabled={!canMutate}
                onClick={onRemove}
              >
                Remove…
              </Button>
            }
          >
            Disconnects {n.displayName} and removes it from Flavor. You choose whether this device keeps its identity.
          </Callout>
        </TabsContent>
      </Tabs>
    </motion.section>
  );
}

function StateNote({ network: n }: { network: Network }) {
  if (n.state === S.CONNECTING || n.state === S.RECONNECTING) {
    return (
      <p className="flex items-center gap-2 text-sm text-muted-foreground" role="status">
        <Loader2 className="size-4 shrink-0 animate-spin" aria-hidden="true" />
        {n.state === S.CONNECTING
          ? "Starting the network session and contacting the control server."
          : "Connection to the control server was lost. Flavor is reconnecting."}
      </p>
    );
  }
  if (n.state === S.DEGRADED) {
    return (
      <Alert className={warnAlert}>
        <TriangleAlert />
        <AlertTitle>Connected, but the control server reports a problem</AlertTitle>
        {n.statusMessage && <AlertDescription>{n.statusMessage}</AlertDescription>}
      </Alert>
    );
  }
  if (n.state === S.ERROR) {
    return (
      <Alert variant="destructive">
        <CircleAlert />
        <AlertTitle>The network session stopped because of an error</AlertTitle>
        <AlertDescription>
          {n.statusMessage && <p>{n.statusMessage}</p>}
          <p>Connect to try again.</p>
        </AlertDescription>
      </Alert>
    );
  }
  return null;
}

function Callout({
  icon: Icon,
  iconClass,
  title,
  action,
  children,
}: {
  icon: LucideIcon;
  iconClass?: string;
  title: ReactNode;
  action?: ReactNode;
  children?: ReactNode;
}) {
  return (
    <Card className="flex-row items-center gap-3 px-4 py-3">
      <Icon className={cn("size-4 shrink-0 self-start text-muted-foreground mt-0.5", iconClass)} aria-hidden="true" />
      <div className="flex min-w-0 flex-1 flex-col gap-1">
        <h3 className="text-sm font-medium">{title}</h3>
        {children && <div className="text-sm text-muted-foreground">{children}</div>}
      </div>
      {action}
    </Card>
  );
}

function Panel({ title, description, children }: { title: string; description: string; children: ReactNode }) {
  return (
    <Card className="gap-0 py-0">
      <div className="flex flex-col gap-1 px-4 py-3">
        <h3 className="text-sm font-medium">{title}</h3>
        <p className="text-xs text-muted-foreground">{description}</p>
      </div>
      {children}
    </Card>
  );
}

function Detail({ term, mono, children }: { term: string; mono?: boolean; children: ReactNode }) {
  return (
    <div className="flex items-baseline gap-4 border-t border-border px-4 py-3 text-sm">
      <dt className="w-32 shrink-0 text-muted-foreground">{term}</dt>
      <dd className={cn("min-w-0 break-all", mono && "font-mono text-xs")}>{children}</dd>
    </div>
  );
}

function NotLive({ children }: { children: ReactNode }) {
  return (
    <Card className="items-center px-6 py-10 text-center">
      <p className="max-w-sm text-sm text-muted-foreground">{children}</p>
    </Card>
  );
}

function Loading({ label }: { label: string }) {
  return (
    <div role="status" aria-label={label}>
      {[0, 1].map((i) => (
        <div key={i} className={row}>
          <Skeleton className="h-4 w-8 rounded-full" />
          <Skeleton className={cn("h-4", i === 0 ? "w-40" : "w-28")} />
        </div>
      ))}
    </div>
  );
}

function LoadError({ message, onRetry }: { message: string; onRetry: () => void }) {
  return (
    <div className={row} role="alert">
      <CircleAlert className="size-4 shrink-0 text-destructive" aria-hidden="true" />
      <p className="min-w-0 flex-1 text-sm text-destructive">{message}</p>
      <Button size="sm" variant="outline" onClick={onRetry}>
        <RotateCw />
        Try again
      </Button>
    </div>
  );
}

const routeUse: Partial<Record<SubnetRouteUse, [string, Tone]>> = {
  [SubnetRouteUse.INSTALLED]: ["In use", "ok"],
  [SubnetRouteUse.PENDING]: ["Waiting", "info"],
  [SubnetRouteUse.REFUSED]: ["Refused", "danger"],
  [SubnetRouteUse.NOT_ADVERTISED]: ["No device advertises it now", "warn"],
};

function SubnetRoutesCard({ networkId }: { networkId: string }) {
  const { sequence, status } = useDaemon();
  const canMutate = useCanMutate();
  const id = useId();
  const [routes, setRoutes] = useState<SubnetRoute[] | null>(null);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [reload, setReload] = useState(0);
  const generation = useRef(0);
  const toggle = useAction((prefix: string, on: boolean) => {
    generation.current++;
    return chooseSubnetRoutes(networkId, on ? [prefix] : [], on ? [] : [prefix]).then(
      (r) => {
        generation.current++;
        setRoutes(r.routes);
      },
      (e) => fail(`Could not turn ${on ? "on" : "off"} ${prefix}`, e),
    );
  });

  useEffect(() => {
    if (status !== "ready") return;
    let cancelled = false;
    const started = generation.current;
    listSubnetRoutes(networkId)
      .then((r) => {
        if (cancelled || started !== generation.current) return;
        setRoutes(r.routes);
        setLoadError(null);
      })
      .catch((e) => !cancelled && setLoadError(errorMessage(e)));
    return () => {
      cancelled = true;
    };
  }, [networkId, sequence, status, reload]);

  return (
    <Panel
      title="Subnet routes"
      description="Turn a route on to reach its addresses from every program on this computer. This needs system-wide networking through flavor-netd."
    >
      {loadError && !routes ? (
        <LoadError message={loadError} onRetry={() => setReload((v) => v + 1)} />
      ) : !routes ? (
        <Loading label="Loading subnet routes" />
      ) : routes.length === 0 ? (
        <p className={cn(row, "text-sm text-muted-foreground")}>No device on this network shares a subnet route.</p>
      ) : (
        routes.map((r, i) => {
          const used = r.use !== SubnetRouteUse.NOT_USED;
          const via = r.routers.map((d) => (d.online ? d.hostname : `${d.hostname} (offline)`)).join(", ");
          const use = routeUse[r.use];
          return (
            <div key={r.prefix} className={cn(row, "items-start")}>
              <Switch
                id={`${id}-${i}`}
                className="mt-0.5"
                checked={used}
                disabled={!canMutate || toggle.pending}
                onCheckedChange={(on) => void toggle.run(r.prefix, on)}
                aria-label={`Use ${r.prefix}`}
              />
              <div className="flex min-w-0 flex-1 flex-col gap-1">
                <div className="flex flex-wrap items-center gap-2">
                  <Label htmlFor={`${id}-${i}`} className="font-mono text-sm font-normal">
                    {r.prefix}
                  </Label>
                  {use && <ToneBadge tone={use[1]}>{use[0]}</ToneBadge>}
                </div>
                {via && <p className="text-xs text-muted-foreground">via {via}</p>}
                {r.detail && <p className="text-xs text-muted-foreground">{r.detail}</p>}
                {r.overlaps.length > 0 && (
                  <p className="text-xs text-muted-foreground">
                    Overlaps a route on {r.overlaps.map((o) => o.displayName).join(", ")}.
                  </p>
                )}
              </div>
            </div>
          );
        })
      )}
    </Panel>
  );
}

function ExitOfferCard({ networkId }: { networkId: string }) {
  const { sequence, status } = useDaemon();
  const canMutate = useCanMutate();
  const id = useId();
  const [state, setState] = useState<{ offered: boolean; approved: boolean } | null>(null);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [reload, setReload] = useState(0);
  const generation = useRef(0);
  const offer = useAction((on: boolean) => {
    generation.current++;
    return offerExitNode(networkId, on).then(
      (r) => {
        generation.current++;
        setState({ offered: r.offered, approved: r.approved });
      },
      (e) =>
        fail(on ? "Could not offer this computer as an exit node" : "Could not stop offering this computer as an exit node", e),
    );
  });

  useEffect(() => {
    if (status !== "ready") return;
    let cancelled = false;
    const started = generation.current;
    getRoutes(networkId)
      .then((r) => {
        if (cancelled || started !== generation.current) return;
        setState({ offered: r.exitNodeOffered, approved: r.exitNodeApproved });
        setLoadError(null);
      })
      .catch((e) => !cancelled && setLoadError(errorMessage(e)));
    return () => {
      cancelled = true;
    };
  }, [networkId, sequence, status, reload]);

  return (
    <Panel
      title="Exit node"
      description="Devices on this network that pick it send their internet traffic through this computer. Flavor forwards TCP to public internet addresses only, never to your local network or this computer. UDP, such as games and calls, does not go through."
    >
      {loadError && !state ? (
        <LoadError message={loadError} onRetry={() => setReload((v) => v + 1)} />
      ) : !state ? (
        <Loading label="Loading exit node status" />
      ) : (
        <div className={row}>
          <Switch
            id={id}
            checked={state.offered}
            disabled={!canMutate || offer.pending}
            onCheckedChange={(on) => void offer.run(on)}
          />
          <Label htmlFor={id} className="font-normal">
            Offer this computer as an exit node
          </Label>
          {state.offered && (
            <ToneBadge tone={state.approved ? "ok" : "warn"}>{state.approved ? "Approved" : "Needs approval"}</ToneBadge>
          )}
        </div>
      )}
    </Panel>
  );
}

function DNSCard({ networkId }: { networkId: string }) {
  const { sequence, status } = useDaemon();
  const [dns, setDNS] = useState<NetworkDNS | null | undefined>(undefined);
  const [loadError, setLoadError] = useState<string | null>(null);
  const [reload, setReload] = useState(0);

  useEffect(() => {
    if (status !== "ready") return;
    let cancelled = false;
    listDNS(networkId)
      .then((r) => {
        if (cancelled) return;
        setDNS(r.networks[0] ?? null);
        setLoadError(null);
      })
      .catch((e) => !cancelled && setLoadError(errorMessage(e)));
    return () => {
      cancelled = true;
    };
  }, [networkId, sequence, status, reload]);

  const empty = !dns || (dns.records.length === 0 && dns.routes.length === 0 && dns.searchDomains.length === 0);
  return (
    <Panel title="DNS" description="Records, split DNS and search domains this network publishes.">
      {loadError && dns === undefined ? (
        <LoadError message={loadError} onRetry={() => setReload((v) => v + 1)} />
      ) : dns === undefined ? (
        <Loading label="Loading DNS" />
      ) : empty ? (
        <p className={cn(row, "text-sm text-muted-foreground")}>
          This network publishes no DNS records, split DNS or search domains.
        </p>
      ) : (
        <dl>
          {dns.records.map((r) => (
            <DNSRow key={`record:${r.name}`} term={r.name} mono>
              {r.addresses.join(", ")}
            </DNSRow>
          ))}
          {dns.routes.map((r) => (
            <DNSRow key={`route:${r.suffix}`} term={`*.${r.suffix}`} mono>
              <span className="font-sans">Asks </span>
              {r.resolvers.join(", ")}
            </DNSRow>
          ))}
          {dns.searchDomains.length > 0 && (
            <DNSRow term="Search domains">{dns.searchDomains.join(", ")}</DNSRow>
          )}
        </dl>
      )}
    </Panel>
  );
}

function DNSRow({ term, mono, children }: { term: string; mono?: boolean; children: ReactNode }) {
  return (
    <div className="flex items-baseline gap-4 border-t border-border px-4 py-3">
      <dt className={cn("min-w-0 flex-1 break-all", mono ? "font-mono text-xs" : "text-sm text-muted-foreground")}>
        {term}
      </dt>
      <dd className="min-w-0 text-right font-mono text-xs break-all text-muted-foreground">{children}</dd>
    </div>
  );
}

function SignInPanel({ network, flowId, url }: { network: Network; flowId: string; url: string }) {
  const canMutate = useCanMutate();
  const open = useAction(() =>
    openAuthUrl(network.id, flowId).catch((e) => fail("Could not open the sign-in page", e)),
  );
  const destination = authDestination(url);
  return (
    <Callout
      icon={LogIn}
      iconClass="text-warning"
      title={`Sign in to ${network.displayName}`}
      action={
        destination && (
          <Button size="sm" disabled={!canMutate || open.pending} onClick={() => void open.run()}>
            {open.pending ? <Loader2 className="animate-spin" /> : <ExternalLink />}
            Open sign-in page
          </Button>
        )
      }
    >
      {destination ? (
        <>
          Opens your browser at <span className="font-mono text-xs text-foreground">{destination}</span>. Finish
          signing in there and Flavor continues automatically.
        </>
      ) : (
        <span className="text-destructive">
          The control server sent a sign-in link that is not a web address, so Flavor will not open it.
        </span>
      )}
    </Callout>
  );
}

function EnrollPanel({ network }: { network: Network }) {
  const canMutate = useCanMutate();
  const id = useId();
  const [key, setKey] = useState("");
  const enroll = useAction((k: string) => enrollNetwork(network.id, k));
  const where =
    network.provider === ProviderType.HEADSCALE ? "headscale preauthkeys create" : "the Tailscale admin console";
  return (
    <details className="group rounded-xl border border-border bg-card shadow-sm">
      <summary className="flex cursor-pointer list-none items-center gap-3 rounded-xl px-4 py-3 text-sm font-medium focus-visible:ring-[3px] focus-visible:ring-ring/50 focus-visible:outline-none [&::-webkit-details-marker]:hidden">
        <KeyRound className="size-4 shrink-0 text-muted-foreground" aria-hidden="true" />
        <span className="flex-1">Join with a pre-auth key</span>
        <ChevronDown
          className="size-4 shrink-0 text-muted-foreground transition-transform group-open:rotate-180"
          aria-hidden="true"
        />
      </summary>
      <form
        className="flex flex-col gap-2 border-t border-border px-4 py-3"
        onSubmit={(e) => {
          e.preventDefault();
          const k = key.trim();
          setKey("");
          if (k) void enroll.run(k);
        }}
      >
        <Label htmlFor={id}>Pre-auth key</Label>
        <div className="flex gap-2">
          <Input
            id={id}
            type="password"
            value={key}
            onChange={(e) => setKey(e.target.value)}
            autoComplete="off"
            spellCheck={false}
            aria-invalid={enroll.error ? true : undefined}
            aria-describedby={`${id}-hint${enroll.error ? ` ${id}-error` : ""}`}
          />
          <Button type="submit" variant="outline" disabled={!canMutate || enroll.pending || key.trim() === ""}>
            {enroll.pending && <Loader2 className="animate-spin" />}
            Join network
          </Button>
        </div>
        <p id={`${id}-hint`} className="text-xs text-muted-foreground">
          Create one with <span className="font-mono">{where}</span>. It is used once and never stored.
        </p>
        {enroll.error && (
          <p id={`${id}-error`} role="alert" className="text-xs text-destructive">
            {enroll.error}
          </p>
        )}
      </form>
    </details>
  );
}

function Preferences({ network: n }: { network: Network }) {
  const canMutate = useCanMutate();
  const id = useId();
  const [name, setName] = useState(n.displayName);
  const rename = useAction((v: string) => updateNetwork(n.id, { displayName: v }));
  const auto = useAction((v: boolean) =>
    updateNetwork(n.id, { autoConnect: v }).catch((e) => fail("Could not change automatic connection", e)),
  );
  const trimmed = name.trim();
  return (
    <Card className="gap-0 py-0">
      <form
        className="flex flex-col gap-2 px-4 py-3"
        onSubmit={(e) => {
          e.preventDefault();
          if (trimmed && trimmed !== n.displayName)
            void rename.run(trimmed).then((r) => r.ok && toast.success(`Renamed to ${trimmed}`));
        }}
      >
        <Label htmlFor={`${id}-name`}>Name</Label>
        <div className="flex gap-2">
          <Input
            id={`${id}-name`}
            value={name}
            onChange={(e) => setName(e.target.value)}
            maxLength={128}
            aria-invalid={rename.error ? true : undefined}
            aria-describedby={rename.error ? `${id}-error` : undefined}
          />
          <Button
            type="submit"
            variant="outline"
            disabled={!canMutate || rename.pending || !trimmed || trimmed === n.displayName}
          >
            {rename.pending && <Loader2 className="animate-spin" />}
            Rename
          </Button>
        </div>
        {rename.error && (
          <p id={`${id}-error`} role="alert" className="text-xs text-destructive">
            {rename.error}
          </p>
        )}
      </form>
      <div className={row}>
        <Label htmlFor={`${id}-auto`} className="flex-1 font-normal">
          Connect automatically when Flavor starts
        </Label>
        <Switch
          id={`${id}-auto`}
          checked={n.autoConnect}
          disabled={!canMutate || auto.pending}
          onCheckedChange={(v) => void auto.run(v)}
        />
      </div>
    </Card>
  );
}

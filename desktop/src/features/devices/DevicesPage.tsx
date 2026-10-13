import { useEffect, useMemo, useRef, useState, useSyncExternalStore, type KeyboardEvent } from "react";
import { timestampDate } from "@bufbuild/protobuf/wkt";
import { useVirtualizer } from "@tanstack/react-virtual";
import { CloudOff, Search } from "lucide-react";
import { motion } from "motion/react";
import { useDaemon } from "../../app/sync/useDaemon";
import { cn } from "@/lib/cn";
import { networkColorIndex, networkColorVar } from "@/lib/networkColor";
import { Button } from "@/ui/button";
import { Card } from "@/ui/card";
import { Input } from "@/ui/input";
import { Sheet, SheetContent, SheetTitle } from "@/ui/sheet";
import { Skeleton } from "@/ui/skeleton";
import { ToggleGroup, ToggleGroupItem } from "@/ui/toggle-group";
import { DeviceBadges, DeviceDetails } from "./DeviceDetails";
import { countByNetwork, deviceRows } from "./filter";
import { deviceBadges, lastSeen } from "./format";
import { moveCursor } from "./nav";

type Props = {
  networkId: string;
  onNetworkChange: (id: string) => void;
  query: string;
  onQueryChange: (q: string) => void;
  selected: string | null;
  onSelect: (key: string | null) => void;
  onInspect: (destination: string) => void;
  onOpenNetwork: (networkId: string) => void;
  canInspect: boolean;
};

const ROW = 56;
const columns =
  "grid grid-cols-[minmax(0,1fr)_8rem_6rem] items-center gap-4 @2xl:grid-cols-[minmax(0,1fr)_9rem_6rem_7rem]";
const wideQuery = "(min-width: 1080px)";
const subscribeWide = (onChange: () => void) => {
  const m = window.matchMedia(wideQuery);
  m.addEventListener("change", onChange);
  return () => m.removeEventListener("change", onChange);
};
const isWide = () => window.matchMedia(wideQuery).matches;

export function DevicesPage({
  networkId,
  onNetworkChange,
  query,
  onQueryChange,
  selected,
  onSelect,
  onInspect,
  onOpenNetwork,
  canInspect,
}: Props) {
  const { devices, networks, status } = useDaemon();
  const wide = useSyncExternalStore(subscribeWide, isWide);
  const matching = useMemo(() => deviceRows(devices, networks, "", query), [devices, networks, query]);
  const rows = useMemo(
    () => (networkId ? matching.filter((r) => r.device.id?.networkId === networkId) : matching),
    [matching, networkId],
  );
  const counts = useMemo(() => countByNetwork(matching), [matching]);
  const colors = useMemo(() => networkColorIndex(networks.values()), [networks]);
  const sortedNetworks = [...networks.values()].sort((a, b) => a.displayName.localeCompare(b.displayName));

  const scroller = useRef<HTMLDivElement>(null);
  const pendingFocus = useRef<number | null>(null);
  const [cursor, setCursor] = useState(0);
  const virtual = useVirtualizer({
    count: rows.length,
    getScrollElement: () => scroller.current,
    estimateSize: () => ROW,
    overscan: 12,
    getItemKey: (i) => rows[i].key,
  });
  const items = virtual.getVirtualItems();
  const active = Math.min(cursor, rows.length - 1);
  const range = virtual.range;
  const shown = !!range && active >= range.startIndex && active <= range.endIndex;
  const tabbable = shown ? active : (range?.startIndex ?? items[0]?.index ?? -1);

  const rowElement = (i: number) => scroller.current?.querySelector<HTMLElement>(`[data-index="${i}"]`);
  const focusRow = (i: number) => {
    setCursor(i);
    virtual.scrollToIndex(i);
    const el = rowElement(i);
    if (el) el.focus({ preventScroll: true });
    else pendingFocus.current = i;
  };

  useEffect(() => {
    if (pendingFocus.current === null) return;
    const el = rowElement(pendingFocus.current);
    if (!el) return;
    pendingFocus.current = null;
    el.focus({ preventScroll: true });
  });

  useEffect(() => {
    if (!selected) return;
    const i = rows.findIndex((r) => r.key === selected);
    if (i < 0) return;
    setCursor(i);
    virtual.scrollToIndex(i);
  }, [selected]);

  const close = () => {
    const i = rows.findIndex((r) => r.key === selected);
    onSelect(null);
    if (wide && i >= 0) focusRow(i);
  };

  const onListKey = (e: KeyboardEvent<HTMLDivElement>) => {
    if (e.key === "Escape" && selected) {
      e.preventDefault();
      onSelect(null);
      return;
    }
    const from = Number((e.target as HTMLElement).dataset.index ?? active);
    const next = moveCursor(e.key, from, rows.length);
    if (next === null) return;
    e.preventDefault();
    focusRow(next);
  };

  const loading = devices.size === 0 && (status === "starting" || status === "resyncing");
  const search = query.trim();
  const filtered = search !== "" || networkId !== "";
  const filterName = networks.get(networkId)?.displayName ?? "This network";
  const details = selected && (
    <DeviceDetails
      key={selected}
      deviceKey={selected}
      onClose={close}
      onInspect={onInspect}
      onOpenNetwork={onOpenNetwork}
      canInspect={canInspect}
    />
  );
  const selectedName = selected ? devices.get(selected)?.hostname || "Device" : "";
  const countLabel = loading
    ? "Loading devices"
    : rows.length === devices.size
      ? `${devices.size} ${devices.size === 1 ? "device" : "devices"}`
      : `${rows.length} of ${devices.size} devices`;

  return (
    <div className="mx-auto flex h-[calc(100dvh_-_var(--space-5)_-_var(--space-6))] min-h-96 w-full max-w-5xl flex-col gap-6">
      <header className="flex flex-col gap-1">
        <h1 className="text-xl font-semibold">Devices</h1>
        <p className="text-sm text-muted-foreground">
          Every device on every connected network. The same address on two networks means two different machines.
        </p>
      </header>

      <div className="flex flex-col gap-3">
        <div className="relative">
          <Search
            className="pointer-events-none absolute top-1/2 left-3 size-4 -translate-y-1/2 text-muted-foreground"
            aria-hidden="true"
          />
          <Input
            type="search"
            aria-label="Search devices"
            className="pl-9"
            style={{ paddingRight: `calc(${countLabel.length}ch + 1.5rem)` }}
            placeholder="Name, address or tag. Try is:online, os:linux, tag:db"
            value={query}
            onChange={(e) => onQueryChange(e.target.value)}
            spellCheck={false}
            autoComplete="off"
          />
          <span
            role="status"
            className="pointer-events-none absolute top-1/2 right-3 -translate-y-1/2 text-xs text-muted-foreground"
          >
            {countLabel}
          </span>
        </div>
        <ToggleGroup
          type="single"
          variant="outline"
          size="sm"
          spacing={2}
          aria-label="Filter by network"
          className="w-full flex-wrap"
          value={networkId || "all"}
          onValueChange={(v) => onNetworkChange(v === "all" ? "" : v)}
        >
          <ToggleGroupItem value="all" className={chip}>
            All networks
            <Count value={matching.length} />
          </ToggleGroupItem>
          {sortedNetworks.map((n) => (
            <ToggleGroupItem key={n.id} value={n.id} className={chip}>
              <span
                className="size-2.5 shrink-0 rounded-[3px]"
                style={{ background: networkColorVar(colors.get(n.id)) }}
                aria-hidden="true"
              />
              {n.displayName}
              <Count value={counts.get(n.id) ?? 0} />
            </ToggleGroupItem>
          ))}
        </ToggleGroup>
        {status === "unavailable" && (
          <p className="flex items-center gap-2 text-xs text-muted-foreground">
            <CloudOff className="size-4 shrink-0" aria-hidden="true" />
            flavord is not reachable. Showing the last known devices.
          </p>
        )}
      </div>

      <div className="flex min-h-0 flex-1 gap-4">
        <Card className="@container min-w-0 flex-1 gap-0 overflow-hidden py-0">
          <div
            className={cn(
              columns,
              "h-9 shrink-0 overflow-y-hidden border-b border-border bg-muted/50 px-4 text-xs font-medium text-muted-foreground [scrollbar-gutter:stable]",
            )}
            aria-hidden="true"
          >
            <span>Device</span>
            <span>Address</span>
            <span className="hidden @2xl:block">OS</span>
            <span>Status</span>
          </div>
          {loading ? (
            <div className="flex flex-col overflow-y-hidden [scrollbar-gutter:stable]" aria-busy="true">
              {Array.from({ length: 6 }, (_, i) => (
                <div key={i} className={cn(columns, "h-14 border-t border-border px-4 first:border-t-0")}>
                  <span className="flex flex-col gap-1.5">
                    <Skeleton className="h-4 w-36" />
                    <Skeleton className="h-3 w-48" />
                  </span>
                  <Skeleton className="h-4 w-24" />
                  <Skeleton className="hidden h-4 w-12 @2xl:block" />
                  <Skeleton className="h-4 w-16" />
                </div>
              ))}
            </div>
          ) : rows.length === 0 ? (
            <div className="flex flex-1 flex-col items-center justify-center gap-3 px-6 py-12 text-center">
              <div className="flex flex-col gap-1">
                <h2 className="text-sm font-medium">{filtered ? "No matching devices" : "No devices yet"}</h2>
                <p className="text-sm text-muted-foreground">
                  {!filtered
                    ? "Devices appear here once a network is connected."
                    : search && networkId
                      ? `No device on ${filterName} matches “${search}”.`
                      : search
                        ? `No device matches “${search}”.`
                        : `${filterName} has no devices to show.`}
                </p>
              </div>
              {filtered && (
                <Button
                  variant="outline"
                  size="sm"
                  onClick={() => {
                    onQueryChange("");
                    onNetworkChange("");
                  }}
                >
                  {search && networkId ? "Clear search and filter" : search ? "Clear search" : "Show all networks"}
                </Button>
              )}
            </div>
          ) : (
            <div
              ref={scroller}
              role="group"
              aria-label="Devices"
              className="min-h-0 flex-1 overflow-y-auto [scrollbar-gutter:stable]"
              onKeyDown={onListKey}
            >
              <div className="relative w-full" style={{ height: virtual.getTotalSize() }}>
                {items.map((item) => {
                  const { key, device: d, networkName } = rows[item.index];
                  const seen = d.online ? null : lastSeen(d.lastSeen ? timestampDate(d.lastSeen) : undefined);
                  const dns = d.dnsName.replace(/\.$/, "");
                  return (
                    <button
                      key={item.key}
                      type="button"
                      data-index={item.index}
                      tabIndex={item.index === tabbable ? 0 : -1}
                      aria-current={selected === key ? "true" : undefined}
                      onClick={() => onSelect(key)}
                      onFocus={() => setCursor(item.index)}
                      className={cn(
                        columns,
                        "absolute inset-x-0 top-0 h-14 px-4 text-left transition-colors hover:bg-accent/60 focus-visible:ring-2 focus-visible:ring-ring focus-visible:outline-none focus-visible:ring-inset aria-[current=true]:bg-accent",
                        item.index > 0 && "border-t border-border",
                      )}
                      style={{ transform: `translateY(${item.start}px)` }}
                    >
                      <span
                        className="absolute inset-y-3 left-1.5 w-[3px] rounded-full"
                        style={{ background: networkColorVar(colors.get(d.id?.networkId ?? "")) }}
                        aria-hidden="true"
                      />
                      <span className="flex min-w-0 flex-col gap-0.5">
                        <span className="flex h-5 min-w-0 flex-wrap items-center gap-x-2 overflow-hidden">
                          <span className="max-w-full truncate text-sm font-medium">{d.hostname || dns}</span>
                          <DeviceBadges badges={deviceBadges(d)} />
                        </span>
                        <span className="truncate font-mono text-xs text-muted-foreground">{dns || " "}</span>
                      </span>
                      <span className="flex min-w-0 flex-col gap-0.5">
                        <span className="truncate font-mono text-sm" title={d.addresses.join(", ")}>
                          {d.addresses[0] ?? "No address"}
                          {d.addresses.length > 1 && (
                            <span className="text-muted-foreground"> +{d.addresses.length - 1}</span>
                          )}
                        </span>
                        <span className="truncate text-xs text-muted-foreground">{networkName || "Unknown network"}</span>
                      </span>
                      <span className="hidden truncate text-sm text-muted-foreground @2xl:block">{d.os || "Unknown"}</span>
                      <span className="flex min-w-0 flex-col gap-0.5">
                        <span className="flex items-center gap-2 text-sm">
                          <span
                            className={cn(
                              "size-2 shrink-0 rounded-full",
                              d.online ? "bg-success" : "bg-muted-foreground/50",
                            )}
                            aria-hidden="true"
                          />
                          {d.online ? "Online" : "Offline"}
                        </span>
                        {seen && <span className="truncate text-xs text-muted-foreground">{seen}</span>}
                      </span>
                    </button>
                  );
                })}
              </div>
            </div>
          )}
        </Card>

        {wide && details && (
          <motion.div
            initial={{ opacity: 0, x: 6 }}
            animate={{ opacity: 1, x: 0 }}
            transition={{ duration: 0.16, ease: [0.2, 0, 0, 1] }}
            className="flex w-80 shrink-0"
            onKeyDown={(e) => {
              if (e.key === "Escape") close();
            }}
          >
            <Card className="min-w-0 flex-1 gap-0 overflow-hidden py-0">{details}</Card>
          </motion.div>
        )}
      </div>

      <Sheet open={!wide && !!selected} onOpenChange={(open) => !open && close()}>
        <SheetContent
          showCloseButton={false}
          aria-describedby={undefined}
          className="w-full gap-0 data-[state=closed]:duration-150 data-[state=open]:duration-200 sm:max-w-sm"
          onCloseAutoFocus={(e) => {
            e.preventDefault();
            if (active >= 0) focusRow(active);
          }}
        >
          <SheetTitle className="sr-only">{selectedName}</SheetTitle>
          {!wide && details}
        </SheetContent>
      </Sheet>
    </div>
  );
}

const chip = "h-8 gap-2 px-3 font-normal data-[state=on]:border-foreground/30 data-[state=on]:font-medium";

function Count({ value }: { value: number }) {
  return <span className="font-mono text-xs text-muted-foreground">{value}</span>;
}

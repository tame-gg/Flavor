import { useMemo, useRef, useState } from "react";
import { useVirtualizer } from "@tanstack/react-virtual";
import { useDaemon } from "../../app/sync/useDaemon";
import { deviceRows } from "./filter";

type Props = { networkId: string; onNetworkChange: (id: string) => void };

export function DevicesPage({ networkId, onNetworkChange }: Props) {
  const { devices, networks, status } = useDaemon();
  const [query, setQuery] = useState("");
  const rows = useMemo(() => deviceRows(devices, networks, networkId, query), [devices, networks, networkId, query]);
  const scroller = useRef<HTMLDivElement>(null);
  const virtual = useVirtualizer({
    count: rows.length,
    getScrollElement: () => scroller.current,
    estimateSize: () => 48,
    overscan: 12,
    getItemKey: (i) => rows[i].key,
  });
  const sortedNetworks = [...networks.values()].sort((a, b) => a.displayName.localeCompare(b.displayName));

  return (
    <div className="page">
      <header className="page-header">
        <div className="stack-sm">
          <h1>Devices</h1>
          <p className="muted">
            Devices are listed per network. The same address on two networks means two different machines.
          </p>
        </div>
      </header>
      <div className="row">
        <label className="sr-only" htmlFor="device-search">
          Search devices
        </label>
        <input
          id="device-search"
          className="input grow"
          type="search"
          placeholder="Search by name, address or network"
          value={query}
          onChange={(e) => setQuery(e.target.value)}
        />
        <label className="sr-only" htmlFor="device-network">
          Network
        </label>
        <select id="device-network" className="select" value={networkId} onChange={(e) => onNetworkChange(e.target.value)}>
          <option value="">All networks</option>
          {sortedNetworks.map((n) => (
            <option key={n.id} value={n.id}>
              {n.displayName}
            </option>
          ))}
        </select>
      </div>
      {status === "unavailable" && <p className="muted small">Showing last known devices.</p>}
      {rows.length === 0 ? (
        <div className="card empty">
          <h2>{devices.size === 0 ? "No devices yet" : "No matching devices"}</h2>
          <p className="muted">
            {devices.size === 0 ? "Devices appear here once a network is connected." : "Try a different search or network."}
          </p>
        </div>
      ) : (
        <div className="table" role="table" aria-label="Devices" aria-rowcount={rows.length + 1}>
          <div className="table-head" role="row">
            <span role="columnheader">Device</span>
            <span role="columnheader" className="hide-narrow">
              Network
            </span>
            <span role="columnheader">Addresses</span>
            <span role="columnheader">Status</span>
          </div>
          <div ref={scroller} className="table-scroll">
            <div style={{ height: virtual.getTotalSize(), position: "relative" }}>
              {virtual.getVirtualItems().map((item) => {
                const { device, networkName } = rows[item.index];
                return (
                  <div
                    key={item.key}
                    className="table-row"
                    role="row"
                    aria-rowindex={item.index + 2}
                    style={{ position: "absolute", top: 0, left: 0, right: 0, transform: `translateY(${item.start}px)` }}
                  >
                    <span role="cell" className="cell">
                      <span className="list-title">{device.hostname || device.dnsName}</span>
                    </span>
                    <span role="cell" className="cell hide-narrow">
                      {networkName}
                    </span>
                    <span role="cell" className="cell mono" title={device.addresses.join(", ")}>
                      {device.addresses.join(", ")}
                    </span>
                    <span role="cell">
                      <span className={`badge ${device.online ? "badge-ok" : ""}`}>{device.online ? "Online" : "Offline"}</span>
                    </span>
                  </div>
                );
              })}
            </div>
          </div>
        </div>
      )}
    </div>
  );
}

import { useMemo, useRef } from "react";
import { useVirtualizer } from "@tanstack/react-virtual";
import { useDaemon } from "../../app/sync/useDaemon";
import { providerName } from "../networks/format";
import { DeviceDetails } from "./DeviceDetails";
import { deviceRows } from "./filter";

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

export function DevicesPage({ networkId, onNetworkChange, query, onQueryChange, selected, onSelect, onInspect, onOpenNetwork, canInspect }: Props) {
  const { devices, networks, status } = useDaemon();
  const rows = useMemo(() => deviceRows(devices, networks, networkId, query), [devices, networks, networkId, query]);
  const scroller = useRef<HTMLDivElement>(null);
  const virtual = useVirtualizer({
    count: rows.length,
    getScrollElement: () => scroller.current,
    estimateSize: () => 56,
    overscan: 12,
    getItemKey: (i) => rows[i].key,
  });
  const sortedNetworks = [...networks.values()].sort((a, b) => a.displayName.localeCompare(b.displayName));

  return (
    <div className="page page-wide">
      <header className="page-header">
        <div className="stack-sm">
          <h1>Devices</h1>
          <p className="muted">Every device on every connected network. The same address on two networks means two different machines.</p>
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
          placeholder="Name, address, network, OS or tag. Try is:online, os:linux, tag:db"
          value={query}
          onChange={(e) => onQueryChange(e.target.value)}
          spellCheck={false}
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
      <div className={selected ? "explorer explorer-open" : "explorer"}>
        {rows.length === 0 ? (
          <div className="card empty">
            <h2>{devices.size === 0 ? "No devices yet" : "No matching devices"}</h2>
            <p className="muted">
              {devices.size === 0 ? "Devices appear here once a network is connected." : "Try a different search or network."}
            </p>
          </div>
        ) : (
          <div className="table" role="grid" aria-label="Devices" aria-rowcount={rows.length + 1}>
            <div className="table-head" role="row">
              <span role="columnheader">Device</span>
              <span role="columnheader">Network</span>
              <span role="columnheader" className="hide-narrow">
                Addresses
              </span>
              <span role="columnheader">Status</span>
            </div>
            <div ref={scroller} className="table-scroll">
              <div style={{ height: virtual.getTotalSize(), position: "relative" }}>
                {virtual.getVirtualItems().map((item) => {
                  const { key, device, network, networkName } = rows[item.index];
                  return (
                    <div
                      key={item.key}
                      className="table-row device-row"
                      role="row"
                      tabIndex={0}
                      aria-selected={selected === key}
                      aria-rowindex={item.index + 2}
                      onClick={() => onSelect(key)}
                      onKeyDown={(e) => {
                        if (e.key === "Enter" || e.key === " ") {
                          e.preventDefault();
                          onSelect(key);
                        }
                      }}
                      style={{ position: "absolute", top: 0, left: 0, right: 0, transform: `translateY(${item.start}px)` }}
                    >
                      <span role="gridcell" className="cell two-line">
                        <span className="row name-line">
                          <span className="list-title">{device.hostname || device.dnsName}</span>
                          {device.local && <span className="badge">This device</span>}
                        </span>
                        <span className="muted small cell">
                          {[device.os, device.dnsName.replace(/\.$/, "")].filter(Boolean).join(" · ") || " "}
                        </span>
                      </span>
                      <span role="gridcell" className="cell two-line">
                        <strong className="cell">{networkName}</strong>
                        <span className="muted small">{network ? providerName(network.provider) : ""}</span>
                      </span>
                      <span role="gridcell" className="cell mono hide-narrow" title={device.addresses.join(", ")}>
                        {device.addresses[0]}
                        {device.addresses.length > 1 && <span className="muted"> +{device.addresses.length - 1}</span>}
                      </span>
                      <span role="gridcell">
                        <span className={`badge ${device.online ? "badge-ok" : ""}`}>{device.online ? "Online" : "Offline"}</span>
                      </span>
                    </div>
                  );
                })}
              </div>
            </div>
          </div>
        )}
        {selected && (
          <DeviceDetails
            deviceKey={selected}
            onClose={() => onSelect(null)}
            onInspect={onInspect}
            onOpenNetwork={onOpenNetwork}
            canInspect={canInspect}
          />
        )}
      </div>
    </div>
  );
}

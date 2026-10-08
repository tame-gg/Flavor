import { useEffect, useState } from "react";
import { timestampDate } from "@bufbuild/protobuf/wkt";
import type { DescribeDeviceResponse } from "@gen/flavor/v1/inspector_pb";
import { Button } from "../../components/ui/Button";
import { CopyButton } from "../../components/ui/CopyButton";
import { StatusBadge } from "../../components/ui/StatusBadge";
import { useDaemon } from "../../app/sync/useDaemon";
import { describeDevice } from "../../lib/api/daemon";
import { providerName } from "../networks/format";
import { lastSeen, sshTarget } from "./format";

type Props = {
  deviceKey: string;
  onClose: () => void;
  onInspect: (destination: string) => void;
  onOpenNetwork: (networkId: string) => void;
  canInspect: boolean;
};

export function DeviceDetails({ deviceKey, onClose, onInspect, onOpenNetwork, canInspect }: Props) {
  const { devices, networks, sequence } = useDaemon();
  const d = devices.get(deviceKey);
  const network = d?.id ? networks.get(d.id.networkId) : undefined;
  const [names, setNames] = useState<DescribeDeviceResponse | null>(null);
  const networkId = d?.id?.networkId;
  const nodeId = d?.id?.nodeId;
  useEffect(() => {
    if (!networkId || !nodeId) return;
    let cancelled = false;
    describeDevice(networkId, nodeId)
      .then((r) => !cancelled && setNames(r))
      .catch(() => !cancelled && setNames(null));
    return () => {
      cancelled = true;
    };
  }, [networkId, nodeId, sequence]);

  if (!d || !d.id) {
    return (
      <aside className="card details" aria-label="Device details">
        <div className="row">
          <h2 className="grow">Device unavailable</h2>
          <Button variant="ghost" onClick={onClose} aria-label="Close device details">
            Close
          </Button>
        </div>
        <p className="muted">This device is no longer reported. Its network may have disconnected or removed it.</p>
      </aside>
    );
  }

  const name = d.hostname || d.dnsName;
  const dns = d.dnsName.replace(/\.$/, "");
  const seen = !d.online ? lastSeen(d.lastSeen ? timestampDate(d.lastSeen) : undefined) : null;
  const ssh = d.local ? null : sshTarget(d.dnsName, d.addresses);
  const destination = dns || d.addresses[0];

  return (
    <aside className="card details stack" aria-label={`Details for ${name}`}>
      <div className="row">
        <h2 className="grow details-title">{name}</h2>
        <Button variant="ghost" onClick={onClose} aria-label="Close device details">
          Close
        </Button>
      </div>
      <div className="row">
        <span className={`badge ${d.online ? "badge-ok" : ""}`}>{d.online ? "Online" : "Offline"}</span>
        {d.local && <span className="badge badge-info">This device</span>}
        {seen && <span className="muted small">Last seen {seen}</span>}
      </div>

      <div className="details-network">
        <div className="stack-sm grow">
          <span className="hint">Network</span>
          <strong>{network?.displayName ?? "Unknown network"}</strong>
          <span className="muted small">{network ? providerName(network.provider) : ""}</span>
        </div>
        {network && <StatusBadge state={network.state} />}
      </div>

      <dl className="dl">
        {dns && (
          <>
            <dt>DNS name</dt>
            <dd className="row">
              <span className="mono grow">{dns}</span>
              <CopyButton value={dns} label="Copy" />
            </dd>
          </>
        )}
        {d.addresses.map((a, i) => (
          <div key={a} className="dl-row">
            <dt>{i === 0 ? "Addresses" : ""}</dt>
            <dd className="row">
              <span className="mono grow">{a}</span>
              <CopyButton value={a} label="Copy" />
            </dd>
          </div>
        ))}
        {d.routes.map((r, i) => (
          <div key={r} className="dl-row">
            <dt>{i === 0 ? "Routes" : ""}</dt>
            <dd className="row">
              <span className="mono grow">{r}</span>
              {canInspect && (
                <Button variant="ghost" aria-label={`Inspect route ${r}`} onClick={() => onInspect(r.split("/")[0])}>
                  Inspect
                </Button>
              )}
            </dd>
          </div>
        ))}
        {d.os && (
          <>
            <dt>OS</dt>
            <dd>{d.os}</dd>
          </>
        )}
        {d.tags.length > 0 && (
          <>
            <dt>Tags</dt>
            <dd className="row">
              {d.tags.map((t) => (
                <span key={t} className="badge tag">
                  {t}
                </span>
              ))}
            </dd>
          </>
        )}
      </dl>

      <div className="row">
        {canInspect && destination && (
          <Button variant="primary" onClick={() => onInspect(destination)}>
            Inspect
          </Button>
        )}
        {network && <Button onClick={() => onOpenNetwork(network.id)}>Open network</Button>}
        {ssh && <CopyButton value={ssh} label="Copy SSH command" />}
      </div>

      <details>
        <summary className="field-label">Identifiers</summary>
        <dl className="dl details-body">
          <dt>Network ID</dt>
          <dd className="mono">{d.id.networkId}</dd>
          <dt>Node ID</dt>
          <dd className="mono">{d.id.nodeId}</dd>
          {names?.name && (
            <>
              <dt>Flavor name</dt>
              <dd className="mono">{names.name}</dd>
            </>
          )}
          {names?.stableName && names.stableName !== names.name && (
            <>
              <dt>Stable name</dt>
              <dd className="mono">{names.stableName}</dd>
            </>
          )}
        </dl>
      </details>
    </aside>
  );
}

import { useEffect, useState } from "react";
import { ConflictSeverity, ConflictType, type Conflict, type ListConflictsResponse } from "@gen/lattice/v1/conflicts_pb";
import { Banner } from "../../components/ui/Banner";
import { Button } from "../../components/ui/Button";
import { CopyButton } from "../../components/ui/CopyButton";
import { listConflicts, setDestinationPreference } from "../../lib/api/daemon";
import { errorMessage } from "../../lib/api/errors";
import { useCanMutate, useDaemon } from "../../app/sync/useDaemon";
import { category, counts, describe, filterConflicts, typeLabel, type SeverityFilter, type TypeFilter } from "./describe";

type Props = {
  onInspect: (destination: string) => void;
  onOpenNetwork: (networkId: string) => void;
  onShowDevice: (networkId: string, nodeId: string) => void;
  canPrefer: boolean;
};

export function ConflictsPage({ onInspect, onOpenNetwork, onShowDevice, canPrefer }: Props) {
  const { sequence, status } = useDaemon();
  const [data, setData] = useState<ListConflictsResponse | null>(null);
  const [error, setError] = useState<string | null>(null);
  const [loading, setLoading] = useState(false);
  const [severity, setSeverity] = useState<SeverityFilter>("all");
  const [type, setType] = useState<TypeFilter>("all");

  useEffect(() => {
    if (status !== "ready") return;
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
        .finally(() => !cancelled && setLoading(false));
    }, 250);
    return () => {
      cancelled = true;
      clearTimeout(t);
    };
  }, [sequence, status]);

  const all = data?.conflicts ?? [];
  const shown = filterConflicts(all, severity, type);
  const n = counts(all);

  return (
    <div className="page">
      <header className="page-header">
        <div className="stack-sm">
          <h1>Conflicts</h1>
          <p className="muted">Addresses and names that exist more than once across your connected networks.</p>
        </div>
        {loading && <span className="spinner muted" aria-label="Updating" />}
      </header>

      {error && <Banner tone="danger">{error}</Banner>}

      {!data ? (
        !error && (
          <div className="card stack-sm" aria-busy="true">
            <div className="skeleton" />
            <div className="skeleton" />
          </div>
        )
      ) : (
        <>
          <div className="row">
            <div className="segmented" role="group" aria-label="Filter by kind">
              {(
                [
                  ["all", `All ${all.length}`],
                  ["ambiguous", `Ambiguous ${n.ambiguous}`],
                  ["expected", `Expected ${n.expected}`],
                  ...(canPrefer ? ([["resolved", `Resolved ${n.resolved}`]] as const) : []),
                ] as const
              ).map(([value, label]) => (
                <button key={value} type="button" aria-pressed={severity === value} onClick={() => setSeverity(value)}>
                  {label}
                </button>
              ))}
            </div>
            <span className="spacer" />
            <label className="sr-only" htmlFor="conflict-type">
              Conflict type
            </label>
            <select
              id="conflict-type"
              className="select"
              value={type}
              onChange={(e) => setType(e.target.value === "all" ? "all" : (Number(e.target.value) as ConflictType))}
            >
              <option value="all">All types</option>
              <option value={ConflictType.ADDRESS_COLLISION}>Addresses</option>
              <option value={ConflictType.DNS_NAME_COLLISION}>DNS names</option>
              <option value={ConflictType.HOSTNAME_COLLISION}>Device names</option>
            </select>
          </div>

          {all.length === 0 ? (
            <div className="card empty">
              <h2>Nothing overlaps</h2>
              <p className="muted">Every address and name on your connected networks is unique.</p>
            </div>
          ) : shown.length === 0 ? (
            <div className="card empty">
              <h2>No matching conflicts</h2>
              <p className="muted">Try a different filter.</p>
            </div>
          ) : (
            <ul className="list candidate-list">
              {shown.map((c) => (
                <ConflictCard
                  key={c.id}
                  conflict={c}
                  onInspect={onInspect}
                  onOpenNetwork={onOpenNetwork}
                  onShowDevice={onShowDevice}
                  canPrefer={canPrefer}
                />
              ))}
            </ul>
          )}

          {data.notInspected.length > 0 && (
            <p className="muted small">
              Not checked because {data.notInspected.length === 1 ? "it is" : "they are"} not connected:{" "}
              {data.notInspected.map((x) => x.displayName).join(", ")}.
            </p>
          )}
        </>
      )}
    </div>
  );
}

function ConflictCard({ conflict: c, onInspect, onOpenNetwork, onShowDevice, canPrefer }: { conflict: Conflict } & Props) {
  const ambiguous = c.severity === ConflictSeverity.AMBIGUOUS;
  const kind = category(c);
  const canMutate = useCanMutate();
  const [prefError, setPrefError] = useState<string | null>(null);
  const preferredName = c.members.find((m) => m.network?.id === c.preferredNetworkId)?.network?.displayName;
  const prefer = async (networkId: string) => {
    setPrefError(null);
    try {
      await setDestinationPreference(c.value, networkId);
    } catch (e) {
      setPrefError(errorMessage(e));
    }
  };
  const spans = new Set(c.members.map((m) => m.network?.id)).size > 1;
  return (
    <li className="card candidate">
      <div className="row">
        <strong className="mono network-name">{c.value}</strong>
        <span className="muted small">{typeLabel[c.type]}</span>
        {kind === "resolved" ? (
          <span className="badge badge-ok">Resolved by preference</span>
        ) : (
          <span className={`badge ${ambiguous ? "badge-warn" : "badge-info"}`}>{ambiguous ? "Ambiguous" : "Expected overlap"}</span>
        )}
        <span className="spacer" />
        <Button aria-label={`Inspect ${c.value}`} onClick={() => onInspect(c.value)}>
          Inspect
        </Button>
      </div>
      <p className="muted">{describe(c)}</p>
      {preferredName && <p className="muted small">Lattice prefers {preferredName} for {c.value}. System routing is not changed.</p>}
      {prefError && <p className="error-text" role="alert">{prefError}</p>}
      <div className="member-table" role="table" aria-label={`Devices sharing ${c.value}`}>
        <div className="member-row member-head" role="row">
          <span role="columnheader">Network</span>
          <span role="columnheader">Device</span>
          <span role="columnheader">Network-specific name</span>
          <span role="columnheader" className="sr-only">
            Actions
          </span>
        </div>
        {c.members.map((m) => {
          const d = m.device;
          const net = m.network;
          if (!d || !net) return null;
          const name = d.hostname || d.dnsName;
          return (
            <div key={`${d.id?.networkId}:${d.id?.nodeId}`} className="member-row" role="row">
              <span role="cell" className="cell">
                <strong>{net.displayName}</strong> {net.id === c.preferredNetworkId && <span className="badge badge-ok">Preferred</span>}
              </span>
              <span role="cell" className="cell">
                {name} {d.local && <span className="badge">This device</span>}
                <span className="mono muted small member-addr" title={d.addresses.join(", ")}>
                  {d.addresses[0]}
                  {d.addresses.length > 1 ? ` +${d.addresses.length - 1}` : ""}
                </span>
              </span>
              <span role="cell" className="cell row">
                {m.uniqueName ? (
                  <>
                    <span className="mono small cell">{m.uniqueName}</span>
                    <CopyButton value={m.uniqueName} label="Copy" />
                  </>
                ) : (
                  <span className="muted small">None</span>
                )}
              </span>
              <span role="cell" className="row member-actions">
                {canPrefer && spans && net.id !== c.preferredNetworkId && (
                  <Button variant="ghost" disabled={!canMutate} aria-label={`Prefer ${net.displayName} for ${c.value}`} onClick={() => void prefer(net.id)}>
                    Prefer
                  </Button>
                )}
                <Button variant="ghost" aria-label={`Show ${name} on ${net.displayName} in Devices`} onClick={() => d.id && onShowDevice(net.id, d.id.nodeId)}>
                  Device
                </Button>
                <Button variant="ghost" aria-label={`Open ${net.displayName}`} onClick={() => onOpenNetwork(net.id)}>
                  Network
                </Button>
              </span>
            </div>
          );
        })}
      </div>
    </li>
  );
}

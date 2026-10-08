import { useEffect, useRef, useState } from "react";
import {
  CandidateStatus,
  DestinationKind,
  MatchKind,
  ResolutionDecision,
  type InspectDestinationResponse,
  type ResolutionCandidate,
} from "@gen/flavor/v1/inspector_pb";
import { Banner } from "../../components/ui/Banner";
import { Button } from "../../components/ui/Button";
import { CopyButton } from "../../components/ui/CopyButton";
import { StatusBadge } from "../../components/ui/StatusBadge";
import { deleteDestinationPreference, inspectDestination, setDestinationPreference } from "../../lib/api/daemon";
import { errorMessage } from "../../lib/api/errors";
import { useCanMutate, useDaemon } from "../../app/sync/useDaemon";
import { useAction } from "../../app/useAction";
import { providerName } from "../networks/format";
import { candidateLabel, explain, matchLabel, preferenceNote } from "./explain";

type Props = {
  query: string;
  onQueryChange: (q: string) => void;
  onOpenNetwork: (networkId: string) => void;
  onShowDevice: (networkId: string, nodeId: string) => void;
  canPrefer: boolean;
};

const decisionTone: Record<ResolutionDecision, string> = {
  [ResolutionDecision.UNSPECIFIED]: "",
  [ResolutionDecision.UNIQUE]: "badge-ok",
  [ResolutionDecision.AMBIGUOUS]: "badge-info",
  [ResolutionDecision.NO_MATCH]: "",
};

const decisionLabel: Record<ResolutionDecision, string> = {
  [ResolutionDecision.UNSPECIFIED]: "",
  [ResolutionDecision.UNIQUE]: "Unique",
  [ResolutionDecision.AMBIGUOUS]: "Needs network context",
  [ResolutionDecision.NO_MATCH]: "No match",
};

export function InspectorPage({ query, onQueryChange, onOpenNetwork, onShowDevice, canPrefer }: Props) {
  const { sequence, status, preferences, networks } = useDaemon();
  const canMutate = useCanMutate();
  const [prefError, setPrefError] = useState<string | null>(null);
  const [prefPending, setPrefPending] = useState(false);
  const [result, setResult] = useState<InspectDestinationResponse | null>(null);
  const run = useAction(inspectDestination);
  const input = useRef<HTMLInputElement>(null);

  const submit = async (q: string) => {
    if (!q.trim()) return;
    const r = await run.run(q.trim());
    if (r.ok) setResult(r.value);
  };

  useEffect(() => {
    if (query && status === "ready" && !result) void submit(query);
    else input.current?.focus();
  }, []);

  const stale = result !== null && sequence > result.snapshotSequence;
  const summary = result && explain(result);
  const note = result && preferenceNote(result);
  const spansNetworks = result !== null && new Set(result.candidates.map((c) => c.network?.id)).size > 1;
  const preferredId = result?.preference?.network?.id;

  const changePreference = async (fn: () => Promise<unknown>) => {
    setPrefError(null);
    setPrefPending(true);
    try {
      await fn();
      if (result) await submit(result.query);
    } catch (e) {
      setPrefError(errorMessage(e));
    } finally {
      setPrefPending(false);
    }
  };
  const prefer = (networkId: string) => result && changePreference(() => setDestinationPreference(result.normalized, networkId));
  const unprefer = (destination: string) => changePreference(() => deleteDestinationPreference(destination));
  const saved = [...preferences.values()].sort((a, b) => a.destination.localeCompare(b.destination));

  return (
    <div className="page">
      <header className="page-header">
        <div className="stack-sm">
          <h1>Connection Inspector</h1>
          <p className="muted">See which of your networks a destination belongs to, and why.</p>
        </div>
      </header>

      <form
        className="row row-end inspect-form"
        onSubmit={(e) => {
          e.preventDefault();
          void submit(query);
        }}
      >
        <div className="field grow">
          <label htmlFor="inspect-destination">What do you want to reach?</label>
          <input
            id="inspect-destination"
            ref={input}
            className="input mono-input"
            value={query}
            onChange={(e) => onQueryChange(e.target.value)}
            placeholder="100.64.0.1, prod-api or postgres.example.ts.net:5432"
            spellCheck={false}
            autoComplete="off"
          />
        </div>
        <Button type="submit" variant="primary" loading={run.pending} disabled={status !== "ready" || !query.trim()}>
          Inspect
        </Button>
      </form>

      {run.error && <Banner tone="danger">{run.error}</Banner>}

      {result && summary && (
        <>
          {stale && (
            <Banner tone="warn">
              <span className="grow">Your networks changed since this result was produced.</span>
              <Button onClick={() => void submit(result.query)} loading={run.pending}>
                Inspect again
              </Button>
            </Banner>
          )}
          <section className="card stack-sm" aria-labelledby="decision-title">
            <div className="row">
              <span className={`badge ${decisionTone[result.decision]}`}>{decisionLabel[result.decision]}</span>
              <span className="muted small mono">
                {result.kind === DestinationKind.ADDRESS ? "address" : "name"} {result.normalized}
                {result.port ? `, port ${result.port}` : ""}
              </span>
            </div>
            <h2 id="decision-title">{summary.title}</h2>
            <p className="muted">{summary.detail}</p>
            {note && <p className="muted">{note}</p>}
            {result.preference && canPrefer && (
              <div className="row">
                <span className="badge">Flavor preference: {result.preference.network?.displayName}</span>
                <span className="hint">Used by Flavor's decisions only. System routing is not changed.</span>
                <span className="spacer" />
                <Button variant="ghost" loading={prefPending} disabled={!canMutate} onClick={() => void unprefer(result.preference!.destination)}>
                  Remove preference
                </Button>
              </div>
            )}
            {prefError && <p className="error-text" role="alert">{prefError}</p>}
            {result.port > 0 && <p className="hint">Flavor notes the port but does not probe services.</p>}
          </section>

          {result.candidates.length > 0 && (
            <section className="stack-sm" aria-labelledby="candidates-title">
              <h3 id="candidates-title">
                {result.candidates.length} candidate{result.candidates.length === 1 ? "" : "s"}
              </h3>
              {canPrefer && spansNetworks && result.decision === ResolutionDecision.AMBIGUOUS && (
                <p className="muted small">Prefer a network to have Flavor choose it whenever you use {result.normalized}.</p>
              )}
              <ul className="list candidate-list">
                {result.candidates.map((c) => (
                  <CandidateRow
                    key={`${c.device?.id?.networkId}:${c.device?.id?.nodeId}`}
                    candidate={c}
                    label={candidateLabel(result, c.status, c.match)}
                    onOpenNetwork={onOpenNetwork}
                    onShowDevice={onShowDevice}
                    onPrefer={
                      canPrefer && spansNetworks && c.network && c.network.id !== preferredId && canMutate && !prefPending
                        ? () => void prefer(c.network!.id)
                        : undefined
                    }
                  />
                ))}
              </ul>
            </section>
          )}

          {result.notInspected.length > 0 && result.decision !== ResolutionDecision.NO_MATCH && (
            <p className="muted small">
              Not checked because {result.notInspected.length === 1 ? "it is" : "they are"} not connected:{" "}
              {result.notInspected.map((n) => n.displayName).join(", ")}.
            </p>
          )}
        </>
      )}

      {canPrefer && saved.length > 0 && (
        <section className="stack-sm" aria-labelledby="saved-preferences">
          <h3 id="saved-preferences">Your preferences</h3>
          <p className="muted small">When a destination exists on several networks, Flavor uses these to decide. They do not change system routing.</p>
          <div className="member-table" role="table" aria-label="Destination preferences">
            {saved.map((p) => (
              <div key={p.destination} className="member-row pref-row" role="row">
                <span role="cell" className="mono cell">
                  {p.destination}
                </span>
                <span role="cell" className="cell">
                  <strong>{networks.get(p.networkId)?.displayName ?? "Removed network"}</strong>
                </span>
                <span role="cell" className="row member-actions">
                  <Button variant="ghost" onClick={() => {
                    onQueryChange(p.destination);
                    void submit(p.destination);
                  }}>
                    Inspect
                  </Button>
                  <Button variant="ghost" disabled={!canMutate || prefPending} aria-label={`Remove preference for ${p.destination}`} onClick={() => void unprefer(p.destination)}>
                    Remove
                  </Button>
                </span>
              </div>
            ))}
          </div>
        </section>
      )}

      {!result && !run.pending && !run.error && (
        <div className="card stack-sm">
          <h3>Try an address or a name</h3>
          <p className="muted">
            Flavor checks every connected network for devices with that address, full DNS name or device name. The same
            address on two networks is normal; the inspector shows each one so you can tell them apart.
          </p>
        </div>
      )}
    </div>
  );
}

function CandidateRow({
  candidate: c,
  label,
  onOpenNetwork,
  onShowDevice,
  onPrefer,
}: {
  candidate: ResolutionCandidate;
  label: string;
  onOpenNetwork: (id: string) => void;
  onShowDevice: (networkId: string, nodeId: string) => void;
  onPrefer?: () => void;
}) {
  const d = c.device;
  const n = c.network;
  if (!d || !n) return null;
  const name = d.hostname || d.dnsName;
  const address = c.match === MatchKind.DEVICE_ADDRESS ? c.matchedValue : d.addresses[0];
  return (
    <li className={`card candidate ${c.status === CandidateStatus.OUTRANKED ? "candidate-outranked" : ""}`}>
      <div className="row">
        <strong className="network-name">{n.displayName}</strong>
        <span className="muted small">{providerName(n.provider)}</span>
        <StatusBadge state={n.state} />
        <span className="spacer" />
        <span className="small muted">{label}</span>
      </div>
      <dl className="dl">
        <dt>Device</dt>
        <dd>
          {name} <span className={`badge ${d.online ? "badge-ok" : ""}`}>{d.online ? "Online" : "Offline"}</span>{" "}
          {d.local && <span className="badge">This device</span>}
        </dd>
        <dt>Matched</dt>
        <dd>
          {matchLabel[c.match]} <span className="mono">{c.matchedValue}</span>
        </dd>
        {d.dnsName && (
          <>
            <dt>DNS name</dt>
            <dd className="mono">{d.dnsName}</dd>
          </>
        )}
        <dt>Addresses</dt>
        <dd className="mono">{d.addresses.join(", ")}</dd>
        {c.qualifiedName && (
          <>
            <dt>Flavor name</dt>
            <dd className="row">
              <span className="mono grow">{c.qualifiedName}</span>
              <CopyButton value={c.qualifiedName} label="Copy" />
            </dd>
          </>
        )}
        {c.stableName && c.stableName !== c.qualifiedName && (
          <>
            <dt>Stable name</dt>
            <dd className="row">
              <span className="mono grow">{c.stableName}</span>
              <CopyButton value={c.stableName} label="Copy" />
            </dd>
          </>
        )}
      </dl>
      <div className="row">
        {address && <CopyButton value={address} label={c.match === MatchKind.SUBNET_ROUTE ? "Copy router address" : "Copy address"} />}
        {d.dnsName && <CopyButton value={d.dnsName} label="Copy DNS name" />}
        <span className="spacer" />
        {onPrefer && (
          <Button onClick={onPrefer}>
            Prefer {n.displayName}
          </Button>
        )}
        {d.id && <Button onClick={() => onShowDevice(n.id, d.id!.nodeId)}>Show device</Button>}
        <Button onClick={() => onOpenNetwork(n.id)}>Open network</Button>
      </div>
    </li>
  );
}

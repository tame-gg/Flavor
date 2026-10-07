import { useEffect, useRef, useState } from "react";
import {
  CandidateStatus,
  DestinationKind,
  MatchKind,
  ResolutionDecision,
  type InspectDestinationResponse,
  type ResolutionCandidate,
} from "@gen/lattice/v1/inspector_pb";
import { Banner } from "../../components/ui/Banner";
import { Button } from "../../components/ui/Button";
import { CopyButton } from "../../components/ui/CopyButton";
import { StatusBadge } from "../../components/ui/StatusBadge";
import { inspectDestination } from "../../lib/api/daemon";
import { useDaemon } from "../../app/sync/useDaemon";
import { useAction } from "../../app/useAction";
import { providerName } from "../networks/format";
import { explain, matchLabel, statusLabel } from "./explain";

type Props = {
  query: string;
  onQueryChange: (q: string) => void;
  onOpenNetwork: (networkId: string) => void;
  onShowDevice: (networkId: string, search: string) => void;
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

export function InspectorPage({ query, onQueryChange, onOpenNetwork, onShowDevice }: Props) {
  const { sequence, status } = useDaemon();
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
            {result.port > 0 && <p className="hint">Lattice notes the port but does not probe services.</p>}
          </section>

          {result.candidates.length > 0 && (
            <section className="stack-sm" aria-labelledby="candidates-title">
              <h3 id="candidates-title">
                {result.candidates.length} candidate{result.candidates.length === 1 ? "" : "s"}
              </h3>
              <ul className="list candidate-list">
                {result.candidates.map((c) => (
                  <CandidateRow
                    key={`${c.device?.id?.networkId}:${c.device?.id?.nodeId}`}
                    candidate={c}
                    onOpenNetwork={onOpenNetwork}
                    onShowDevice={onShowDevice}
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

      {!result && !run.pending && !run.error && (
        <div className="card stack-sm">
          <h3>Try an address or a name</h3>
          <p className="muted">
            Lattice checks every connected network for devices with that address, full DNS name or device name. The same
            address on two networks is normal; the inspector shows each one so you can tell them apart.
          </p>
        </div>
      )}
    </div>
  );
}

function CandidateRow({
  candidate: c,
  onOpenNetwork,
  onShowDevice,
}: {
  candidate: ResolutionCandidate;
  onOpenNetwork: (id: string) => void;
  onShowDevice: (networkId: string, search: string) => void;
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
        <span className="small muted">{statusLabel[c.status]}</span>
      </div>
      <dl className="dl">
        <dt>Device</dt>
        <dd>
          {name} <span className={`badge ${d.online ? "badge-ok" : ""}`}>{d.online ? "Online" : "Offline"}</span>
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
      </dl>
      <div className="row">
        {address && <CopyButton value={address} label="Copy address" />}
        {d.dnsName && <CopyButton value={d.dnsName} label="Copy DNS name" />}
        <span className="spacer" />
        <Button onClick={() => onShowDevice(n.id, name)}>Show device</Button>
        <Button onClick={() => onOpenNetwork(n.id)}>Open network</Button>
      </div>
    </li>
  );
}

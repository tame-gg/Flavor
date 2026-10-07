import { useState } from "react";
import { NetworkConnectionState as S, ProviderType } from "@gen/lattice/v1/common_pb";
import type { Network } from "@gen/lattice/v1/network_pb";
import { Banner } from "../../components/ui/Banner";
import { Button } from "../../components/ui/Button";
import { Field } from "../../components/ui/Field";
import { StatusBadge } from "../../components/ui/StatusBadge";
import { authDestination } from "../../lib/api/authUrl";
import { connectNetwork, disconnectNetwork, enrollNetwork, openAuthUrl, updateNetwork } from "../../lib/api/daemon";
import { useCanMutate, useDaemon } from "../../app/sync/useDaemon";
import { useAction } from "../../app/useAction";
import { controlHost, providerName } from "./format";

const active = new Set([S.CONNECTING, S.AUTHENTICATING, S.AWAITING_APPROVAL, S.CONNECTED, S.DEGRADED, S.RECONNECTING]);

const progress: Partial<Record<S, string>> = {
  [S.CONNECTING]: "Starting the network session and contacting the control server.",
  [S.RECONNECTING]: "Connection to the control server was lost. Lattice is reconnecting.",
  [S.DEGRADED]: "Connected, but the control server reports a problem with this device.",
  [S.ERROR]: "The network session stopped because of an error. Connect to try again.",
};

type Props = {
  network: Network;
  deviceCount: number;
  notice: string | null;
  onShowDevices: () => void;
  onRemove: () => void;
};

export function NetworkDetail({ network: n, deviceCount, notice, onShowDevices, onRemove }: Props) {
  const canMutate = useCanMutate();
  const { status } = useDaemon();
  const isActive = active.has(n.state);
  const toggle = useAction(() => (isActive ? disconnectNetwork(n.id) : connectNetwork(n.id)));
  const prompt = n.authentication;

  return (
    <section className="stack" aria-labelledby="network-title">
      <div className="row">
        <h2 id="network-title">{n.displayName}</h2>
        <StatusBadge state={n.state} />
        <span className="spacer" />
        <Button
          variant={isActive ? "secondary" : "primary"}
          loading={toggle.pending}
          disabled={!canMutate || n.state === S.REMOVING}
          title={canMutate ? undefined : "Waiting for the Lattice daemon"}
          onClick={() => void toggle.run()}
        >
          {isActive ? "Disconnect" : "Connect"}
        </Button>
        <Button variant="danger" disabled={!canMutate} onClick={onRemove}>
          Remove…
        </Button>
      </div>

      {status === "unavailable" && <Banner tone="warn">Showing last known state. The Lattice daemon is not reachable.</Banner>}
      {notice && <Banner tone="warn">{notice}</Banner>}
      {toggle.error && <Banner tone="danger">{toggle.error}</Banner>}
      {progress[n.state] && <p className="muted">{progress[n.state]}</p>}

      {prompt?.kind === "authentication" && <SignInPanel network={n} flowId={prompt.flowId} url={prompt.authUrl} />}
      {prompt?.kind === "approval" && (
        <div className="card stack-sm">
          <h3>Waiting for approval</h3>
          <p className="muted">
            An administrator must approve this device in the {providerName(n.provider)} admin console. Lattice connects
            automatically once it is approved.
          </p>
        </div>
      )}

      {(n.state === S.AUTHENTICATING || n.state === S.DISCONNECTED || n.state === S.ERROR) && <EnrollPanel network={n} />}

      <div className="card stack">
        <h3>Devices</h3>
        <div className="row">
          <p className="muted">
            {deviceCount === 0
              ? isActive
                ? "No devices reported yet."
                : "Connect to see devices on this network."
              : `${deviceCount} device${deviceCount === 1 ? "" : "s"} on this network, including this one.`}
          </p>
          <span className="spacer" />
          {deviceCount > 0 && <Button onClick={onShowDevices}>View devices</Button>}
        </div>
      </div>

      <Preferences network={n} />

      <details className="card">
        <summary className="field-label">Details</summary>
        <dl className="dl details-body">
          <dt>Provider</dt>
          <dd>{providerName(n.provider)}</dd>
          <dt>Control server</dt>
          <dd>{controlHost(n)}</dd>
          <dt>Device name</dt>
          <dd className="mono">{n.nodeHostname}</dd>
          <dt>Network ID</dt>
          <dd className="mono">{n.id}</dd>
        </dl>
      </details>
    </section>
  );
}

function SignInPanel({ network, flowId, url }: { network: Network; flowId: string; url: string }) {
  const canMutate = useCanMutate();
  const open = useAction(() => openAuthUrl(network.id, flowId));
  const destination = authDestination(url);
  return (
    <div className="card stack-sm">
      <h3>Sign in to {network.displayName}</h3>
      {destination ? (
        <>
          <p className="muted">
            Opens your browser at <strong className="mono">{destination}</strong>. Finish signing in there and Lattice
            continues automatically.
          </p>
          <div className="row">
            <Button variant="primary" loading={open.pending} disabled={!canMutate} onClick={() => void open.run()}>
              Open sign-in page
            </Button>
          </div>
        </>
      ) : (
        <p className="error-text">The control server sent a sign-in link that is not a web address, so Lattice will not open it.</p>
      )}
      {open.error && <p className="error-text" role="alert">{open.error}</p>}
    </div>
  );
}

function EnrollPanel({ network }: { network: Network }) {
  const canMutate = useCanMutate();
  const [key, setKey] = useState("");
  const enroll = useAction((k: string) => enrollNetwork(network.id, k));
  const where = network.provider === ProviderType.HEADSCALE ? "headscale preauthkeys create" : "the Tailscale admin console";
  return (
    <details className="card">
      <summary className="field-label">Join with a pre-auth key</summary>
      <form
        className="stack-sm details-body"
        onSubmit={(e) => {
          e.preventDefault();
          const k = key.trim();
          setKey("");
          if (k) void enroll.run(k);
        }}
      >
        <Field
          label="Pre-auth key"
          type="password"
          value={key}
          onChange={(e) => setKey(e.target.value)}
          autoComplete="off"
          spellCheck={false}
          hint={`Create one with ${where}. It is used once and never stored.`}
          error={enroll.error}
        />
        <div className="row">
          <Button type="submit" loading={enroll.pending} disabled={!canMutate || key.trim() === ""}>
            Join network
          </Button>
        </div>
      </form>
    </details>
  );
}

function Preferences({ network: n }: { network: Network }) {
  const canMutate = useCanMutate();
  const [name, setName] = useState(n.displayName);
  const rename = useAction((v: string) => updateNetwork(n.id, { displayName: v }));
  const auto = useAction((v: boolean) => updateNetwork(n.id, { autoConnect: v }));
  const trimmed = name.trim();
  return (
    <div className="card stack">
      <h3>Preferences</h3>
      <form
        className="row row-end"
        onSubmit={(e) => {
          e.preventDefault();
          if (trimmed && trimmed !== n.displayName) void rename.run(trimmed);
        }}
      >
        <div className="grow">
          <Field label="Name" value={name} onChange={(e) => setName(e.target.value)} maxLength={128} error={rename.error} />
        </div>
        <Button type="submit" loading={rename.pending} disabled={!canMutate || !trimmed || trimmed === n.displayName}>
          Rename
        </Button>
      </form>
      <label className="check">
        <input
          type="checkbox"
          checked={n.autoConnect}
          disabled={!canMutate || auto.pending}
          onChange={(e) => void auto.run(e.target.checked)}
        />
        Connect automatically when Lattice starts
      </label>
      {auto.error && <p className="error-text" role="alert">{auto.error}</p>}
    </div>
  );
}

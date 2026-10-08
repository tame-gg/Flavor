import { useState } from "react";
import type { Network } from "@gen/flavor/v1/network_pb";
import { Button } from "../../components/ui/Button";
import { Dialog } from "../../components/ui/Dialog";
import { Field } from "../../components/ui/Field";
import { addNetwork, connectNetwork, enrollNetwork, type Provider } from "../../lib/api/daemon";
import { errorMessage } from "../../lib/api/errors";
import { useCanMutate } from "../../app/sync/useDaemon";
import { useAction } from "../../app/useAction";

type Props = {
  open: boolean;
  onClose: () => void;
  onAdded: (network: Network, followUpError: string | null) => void;
};

export function AddNetworkDialog({ open, onClose, onAdded }: Props) {
  const canMutate = useCanMutate();
  const [provider, setProvider] = useState<Provider>("tailscale");
  const [name, setName] = useState("");
  const [controlUrl, setControlUrl] = useState("");
  const [preAuthKey, setPreAuthKey] = useState("");
  const [autoConnect, setAutoConnect] = useState(true);
  const [connectNow, setConnectNow] = useState(true);
  const add = useAction(addNetwork);

  const reset = () => {
    setProvider("tailscale");
    setName("");
    setControlUrl("");
    setPreAuthKey("");
    setAutoConnect(true);
    setConnectNow(true);
    add.clearError();
  };
  const close = () => {
    reset();
    onClose();
  };

  const submit = async () => {
    const key = preAuthKey.trim();
    setPreAuthKey("");
    const res = await add.run({
      displayName: name.trim(),
      provider,
      controlUrl: provider === "headscale" ? controlUrl.trim() : "",
      autoConnect,
    });
    if (!res.ok) return;
    let followUp: string | null = null;
    try {
      if (key) await enrollNetwork(res.value.id, key);
      else if (connectNow) await connectNetwork(res.value.id);
    } catch (e) {
      followUp = errorMessage(e);
    }
    reset();
    onAdded(res.value, followUp);
  };

  const valid = name.trim() !== "" && (provider === "tailscale" || controlUrl.trim() !== "");

  return (
    <Dialog
      open={open}
      title="Add network"
      onClose={close}
      onSubmit={() => valid && canMutate && void submit()}
      actions={
        <>
          <Button onClick={close}>Cancel</Button>
          <Button type="submit" variant="primary" loading={add.pending} disabled={!valid || !canMutate}>
            Add network
          </Button>
        </>
      }
    >
      <div className="field">
        <span className="field-label" id="provider-label">
          Provider
        </span>
        <div className="segmented" role="group" aria-labelledby="provider-label">
          {(["tailscale", "headscale"] as const).map((p) => (
            <button key={p} type="button" aria-pressed={provider === p} onClick={() => setProvider(p)}>
              {p === "tailscale" ? "Tailscale" : "Headscale"}
            </button>
          ))}
        </div>
      </div>
      <Field label="Name" value={name} onChange={(e) => setName(e.target.value)} maxLength={128} placeholder="Home lab" required autoFocus />
      {provider === "headscale" && (
        <Field
          label="Control server"
          value={controlUrl}
          onChange={(e) => setControlUrl(e.target.value)}
          placeholder="https://headscale.example.com"
          inputMode="url"
          spellCheck={false}
          required
        />
      )}
      <Field
        label="Pre-auth key (optional)"
        type="password"
        value={preAuthKey}
        onChange={(e) => setPreAuthKey(e.target.value)}
        autoComplete="off"
        spellCheck={false}
        hint="Used once to join, then discarded. Leave empty to sign in with your browser."
      />
      <label className="check">
        <input type="checkbox" checked={autoConnect} onChange={(e) => setAutoConnect(e.target.checked)} />
        Connect automatically when Flavor starts
      </label>
      {!preAuthKey && (
        <label className="check">
          <input type="checkbox" checked={connectNow} onChange={(e) => setConnectNow(e.target.checked)} />
          Connect now
        </label>
      )}
      {add.error && <p className="error-text" role="alert">{add.error}</p>}
    </Dialog>
  );
}

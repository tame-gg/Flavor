import { useState } from "react";
import type { Network } from "@gen/flavor/v1/network_pb";
import { Button } from "../../components/ui/Button";
import { Dialog } from "../../components/ui/Dialog";
import { deleteNetworkIdentity, removeNetwork } from "../../lib/api/daemon";
import { useCanMutate } from "../../app/sync/useDaemon";
import { useAction } from "../../app/useAction";

type Props = { network: Network | null; onClose: () => void; onRemoved: () => void };

export function RemoveNetworkDialog({ network, onClose, onRemoved }: Props) {
  const canMutate = useCanMutate();
  const [deleteIdentity, setDeleteIdentity] = useState(false);
  const remove = useAction((id: string, hard: boolean) => (hard ? deleteNetworkIdentity(id) : removeNetwork(id)));
  const close = () => {
    setDeleteIdentity(false);
    remove.clearError();
    onClose();
  };
  const submit = async () => {
    if (!network) return;
    const res = await remove.run(network.id, deleteIdentity);
    if (res.ok) {
      setDeleteIdentity(false);
      onRemoved();
    }
  };
  return (
    <Dialog
      open={network !== null}
      title={`Remove ${network?.displayName ?? "network"}?`}
      onClose={close}
      onSubmit={() => canMutate && void submit()}
      actions={
        <>
          <Button onClick={close} disabled={remove.pending}>
            Cancel
          </Button>
          <Button type="submit" variant="danger-solid" loading={remove.pending} disabled={!canMutate}>
            {deleteIdentity ? "Remove and delete identity" : "Remove network"}
          </Button>
        </>
      }
    >
      <p>
        Flavor disconnects this network and removes it from the app. This device's identity stays on disk, so adding
        the network again can reuse it without signing in.
      </p>
      <label className="check">
        <input type="checkbox" checked={deleteIdentity} onChange={(e) => setDeleteIdentity(e.target.checked)} />
        <span>
          Also delete this device's local identity
          <span className="hint check-hint">
            You will need to sign in again and this device will join as a new machine. Flavor never deletes the machine
            from the control server; an administrator can remove it there.
          </span>
        </span>
      </label>
      {remove.error && <p className="error-text" role="alert">{remove.error}</p>}
    </Dialog>
  );
}

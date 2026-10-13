import { useId, useState } from "react";
import { Loader2 } from "lucide-react";
import { toast } from "sonner";
import type { Network } from "@gen/flavor/v1/network_pb";
import { deleteNetworkIdentity, removeNetwork } from "../../lib/api/daemon";
import { useCanMutate } from "../../app/sync/useDaemon";
import { useAction } from "../../app/useAction";
import { Button } from "@/ui/button";
import { Checkbox } from "@/ui/checkbox";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/ui/dialog";
import { Label } from "@/ui/label";

type Props = { network: Network | null; onClose: () => void; onRemoved: () => void };

const useRemove = () =>
  useAction((networkId: string, hard: boolean) => (hard ? deleteNetworkIdentity(networkId) : removeNetwork(networkId)));

export function RemoveNetworkDialog({ network, onClose, onRemoved }: Props) {
  const [last, setLast] = useState(network);
  if (network && network !== last) setLast(network);
  const shown = network ?? last;
  const remove = useRemove();
  const close = () => {
    remove.clearError();
    onClose();
  };
  return (
    <Dialog open={network !== null} onOpenChange={(open) => !open && !remove.pending && close()}>
      <DialogContent className="sm:max-w-md" showCloseButton={!remove.pending}>
        {shown && <RemoveForm network={shown} remove={remove} onClose={close} onRemoved={onRemoved} />}
      </DialogContent>
    </Dialog>
  );
}

type FormProps = {
  network: Network;
  remove: ReturnType<typeof useRemove>;
  onClose: () => void;
  onRemoved: () => void;
};

function RemoveForm({ network, remove, onClose, onRemoved }: FormProps) {
  const canMutate = useCanMutate();
  const id = useId();
  const [deleteIdentity, setDeleteIdentity] = useState(false);
  const submit = async () => {
    const res = await remove.run(network.id, deleteIdentity);
    if (!res.ok) return;
    toast.success(
      deleteIdentity ? `Removed ${network.displayName} and deleted its identity` : `Removed ${network.displayName}`,
    );
    setDeleteIdentity(false);
    onRemoved();
  };
  return (
    <form
      className="flex flex-col gap-4"
      onSubmit={(e) => {
        e.preventDefault();
        if (canMutate && !remove.pending) void submit();
      }}
    >
      <DialogHeader>
        <DialogTitle>Remove {network.displayName}?</DialogTitle>
        <DialogDescription>
          Flavor disconnects this network and removes it from the app. This device's identity stays on disk, so adding
          the network again can reuse it without signing in.
        </DialogDescription>
      </DialogHeader>
      <div className="flex items-start gap-3 rounded-lg border border-border p-3">
        <Checkbox
          id={id}
          className="mt-0.5"
          checked={deleteIdentity}
          onCheckedChange={(v) => setDeleteIdentity(v === true)}
          aria-describedby={`${id}-hint`}
        />
        <div className="flex flex-col gap-1">
          <Label htmlFor={id} className="leading-5">
            Also delete this device's local identity
          </Label>
          <p id={`${id}-hint`} className="text-xs text-muted-foreground">
            You will need to sign in again and this device will join as a new machine. Flavor never deletes the machine
            from the control server; an administrator can remove it there.
          </p>
        </div>
      </div>
      {remove.error && (
        <p role="alert" className="text-sm text-destructive">
          {remove.error}
        </p>
      )}
      <DialogFooter>
        <Button type="button" variant="outline" onClick={onClose} disabled={remove.pending}>
          Cancel
        </Button>
        <Button type="submit" variant="destructive" disabled={!canMutate || remove.pending}>
          {remove.pending && <Loader2 className="animate-spin" />}
          {deleteIdentity ? "Remove and delete identity" : "Remove network"}
        </Button>
      </DialogFooter>
    </form>
  );
}

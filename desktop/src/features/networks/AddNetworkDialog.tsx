import { useId, useState } from "react";
import { ArrowLeft, ArrowRight, Loader2 } from "lucide-react";
import { motion } from "motion/react";
import { toast } from "sonner";
import type { Network } from "@gen/flavor/v1/network_pb";
import { addNetwork, connectNetwork, enrollNetwork, type Provider } from "../../lib/api/daemon";
import { errorMessage } from "../../lib/api/errors";
import { useCanMutate } from "../../app/sync/useDaemon";
import { useAction } from "../../app/useAction";
import { Button } from "@/ui/button";
import { Checkbox } from "@/ui/checkbox";
import { Dialog, DialogContent, DialogDescription, DialogFooter, DialogHeader, DialogTitle } from "@/ui/dialog";
import { Input } from "@/ui/input";
import { Label } from "@/ui/label";
import { RadioGroup, RadioGroupItem } from "@/ui/radio-group";

type Props = {
  open: boolean;
  onClose: () => void;
  onAdded: (network: Network, followUpError: string | null) => void;
};

const providers: [Provider, string, string][] = [
  ["tailscale", "Tailscale", "Sign in with a Tailscale account. Uses Tailscale's own control server."],
  ["headscale", "Headscale", "A self-hosted control server. You need its address."],
];

const useAdd = () =>
  useAction(async (input: Parameters<typeof addNetwork>[0], key: string, connectNow: boolean) => {
    const network = await addNetwork(input);
    let followUp: string | null = null;
    try {
      if (key) await enrollNetwork(network.id, key);
      else if (connectNow) await connectNetwork(network.id);
    } catch (e) {
      followUp = errorMessage(e);
    }
    return { network, followUp };
  });

export function AddNetworkDialog({ open, onClose, onAdded }: Props) {
  const add = useAdd();
  const close = () => {
    add.clearError();
    onClose();
  };
  return (
    <Dialog open={open} onOpenChange={(o) => !o && !add.pending && close()}>
      <DialogContent className="sm:max-w-md" showCloseButton={!add.pending}>
        <AddNetworkForm add={add} onClose={close} onAdded={onAdded} />
      </DialogContent>
    </Dialog>
  );
}

type FormProps = Omit<Props, "open"> & { add: ReturnType<typeof useAdd> };

function AddNetworkForm({ add, onClose, onAdded }: FormProps) {
  const canMutate = useCanMutate();
  const id = useId();
  const [step, setStep] = useState<1 | 2>(1);
  const [provider, setProvider] = useState<Provider>("tailscale");
  const [name, setName] = useState("");
  const [controlUrl, setControlUrl] = useState("");
  const [preAuthKey, setPreAuthKey] = useState("");
  const [autoConnect, setAutoConnect] = useState(true);
  const [connectNow, setConnectNow] = useState(true);

  const hostReady = provider === "tailscale" || controlUrl.trim() !== "";
  const valid = name.trim() !== "" && hostReady;
  const providerLabel = provider === "tailscale" ? "Tailscale" : "Headscale";

  const submit = async () => {
    const key = preAuthKey.trim();
    setPreAuthKey("");
    const res = await add.run(
      {
        displayName: name.trim(),
        provider,
        controlUrl: provider === "headscale" ? controlUrl.trim() : "",
        autoConnect,
      },
      key,
      connectNow,
    );
    if (!res.ok) return;
    toast.success(`Added ${res.value.network.displayName}`);
    onAdded(res.value.network, res.value.followUp);
  };

  return (
    <form
      className="flex flex-col gap-6"
      onSubmit={(e) => {
        e.preventDefault();
        if (step === 1) {
          if (hostReady) setStep(2);
        } else if (valid && canMutate && !add.pending) void submit();
      }}
    >
      <DialogHeader>
        <DialogTitle>Add network</DialogTitle>
        <DialogDescription>
          {step === 1
            ? "Step 1 of 2: choose where the network is hosted."
            : `Step 2 of 2: name this ${providerLabel} network and choose how it connects.`}
        </DialogDescription>
      </DialogHeader>

      <motion.div
        key={step}
        initial={{ opacity: 0, x: step === 1 ? -6 : 6 }}
        animate={{ opacity: 1, x: 0 }}
        transition={{ duration: 0.16 }}
        className="flex flex-col gap-4"
      >
        {step === 1 ? (
          <>
            <RadioGroup
              value={provider}
              onValueChange={(v) => setProvider(v as Provider)}
              aria-label="Provider"
              className="gap-2"
            >
              {providers.map(([value, title, description]) => (
                <Label
                  key={value}
                  htmlFor={`${id}-${value}`}
                  className="cursor-pointer items-start gap-3 rounded-lg border border-border p-3 font-normal transition-colors hover:bg-accent/60 has-[[data-state=checked]]:border-primary has-[[data-state=checked]]:bg-primary/5"
                >
                  <RadioGroupItem
                    id={`${id}-${value}`}
                    value={value}
                    className="mt-0.5"
                    autoFocus={provider === value}
                  />
                  <span className="flex flex-col gap-1 leading-normal">
                    <span className="text-sm font-medium">{title}</span>
                    <span className="text-xs text-muted-foreground">{description}</span>
                  </span>
                </Label>
              ))}
            </RadioGroup>
            {provider === "headscale" && (
              <div className="flex flex-col gap-2">
                <Label htmlFor={`${id}-url`}>Control server</Label>
                <Input
                  id={`${id}-url`}
                  value={controlUrl}
                  onChange={(e) => setControlUrl(e.target.value)}
                  placeholder="https://headscale.example.com"
                  inputMode="url"
                  spellCheck={false}
                  required
                />
              </div>
            )}
          </>
        ) : (
          <>
            <div className="flex flex-col gap-2">
              <Label htmlFor={`${id}-name`}>Name</Label>
              <Input
                id={`${id}-name`}
                value={name}
                onChange={(e) => setName(e.target.value)}
                maxLength={128}
                placeholder="Home lab"
                required
                autoFocus
              />
            </div>
            <div className="flex flex-col gap-2">
              <Label htmlFor={`${id}-key`}>
                Pre-auth key <span className="font-normal text-muted-foreground">(optional)</span>
              </Label>
              <Input
                id={`${id}-key`}
                type="password"
                value={preAuthKey}
                onChange={(e) => setPreAuthKey(e.target.value)}
                autoComplete="off"
                spellCheck={false}
                aria-describedby={`${id}-key-hint`}
              />
              <p id={`${id}-key-hint`} className="text-xs text-muted-foreground">
                Used once to join, then discarded. Leave empty to sign in with your browser.
              </p>
            </div>
            <div className="flex flex-col gap-3">
              <div className="flex items-center gap-2">
                <Checkbox
                  id={`${id}-auto`}
                  checked={autoConnect}
                  onCheckedChange={(v) => setAutoConnect(v === true)}
                />
                <Label htmlFor={`${id}-auto`} className="font-normal">
                  Connect automatically when Flavor starts
                </Label>
              </div>
              {!preAuthKey && (
                <div className="flex items-center gap-2">
                  <Checkbox
                    id={`${id}-now`}
                    checked={connectNow}
                    onCheckedChange={(v) => setConnectNow(v === true)}
                  />
                  <Label htmlFor={`${id}-now`} className="font-normal">
                    Connect now
                  </Label>
                </div>
              )}
            </div>
            {add.error && (
              <p role="alert" className="text-sm text-destructive">
                {add.error}
              </p>
            )}
          </>
        )}
      </motion.div>

      <DialogFooter>
        {step === 1 ? (
          <Button key="cancel" type="button" variant="outline" onClick={onClose}>
            Cancel
          </Button>
        ) : (
          <Button key="back" type="button" variant="outline" disabled={add.pending} onClick={() => setStep(1)}>
            <ArrowLeft />
            Back
          </Button>
        )}
        {step === 1 ? (
          <Button key="continue" type="submit" disabled={!hostReady}>
            Continue
            <ArrowRight />
          </Button>
        ) : (
          <Button key="add" type="submit" disabled={!valid || !canMutate || add.pending}>
            {add.pending && <Loader2 className="animate-spin" />}
            Add network
          </Button>
        )}
      </DialogFooter>
    </form>
  );
}

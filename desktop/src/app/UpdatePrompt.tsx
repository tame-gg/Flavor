import { useEffect } from "react";
import { toast } from "sonner";
import { installUpdate, onUpdate, pendingUpdate, type UpdateOffer } from "../lib/api/daemon";
import { errorMessage } from "../lib/api/errors";

const id = "flavor-update";

const buttons = (version: string) => ({
  action: { label: "Restart to update", onClick: () => void install(version) },
  cancel: { label: "Later", onClick: () => {} },
});

function offer({ version, flavordUpdate }: UpdateOffer) {
  toast(`Flavor ${version} is available`, {
    id,
    duration: Infinity,
    description: flavordUpdate ?? undefined,
    ...buttons(version),
  });
}

async function install(version: string) {
  toast.loading(`Installing Flavor ${version}…`, {
    id,
    duration: Infinity,
    description: undefined,
    action: undefined,
    cancel: undefined,
  });
  try {
    await installUpdate();
  } catch (e) {
    toast.error(errorMessage(e), { id, duration: Infinity, ...buttons(version) });
  }
}

// The desktop shell checks for updates on launch and every six hours in release builds.
// This only offers the one it found; installing restarts the app.
export function UpdatePrompt() {
  useEffect(() => {
    let live = true;
    let off: (() => void) | undefined;
    void onUpdate(offer).then((unlisten) => (live ? (off = unlisten) : unlisten()));
    void pendingUpdate().then((pending) => pending && offer(pending));
    return () => {
      live = false;
      off?.();
    };
  }, []);
  return null;
}

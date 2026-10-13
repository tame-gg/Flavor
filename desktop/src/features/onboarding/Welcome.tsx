import { Plus } from "lucide-react";
import { motion } from "motion/react";
import logo from "../../logo.svg";
import { Button } from "@/ui/button";

export function Welcome({ onAdd, onLater }: { onAdd: () => void; onLater: () => void }) {
  return (
    <main className="grid h-full place-items-center bg-background p-6 text-foreground">
      <motion.div
        initial={{ opacity: 0, y: 6 }}
        animate={{ opacity: 1, y: 0 }}
        transition={{ duration: 0.2, ease: [0.2, 0, 0, 1] }}
        className="flex w-full max-w-md flex-col items-center gap-6 text-center"
      >
        <img src={logo} alt="" className="size-12" />
        <div className="flex flex-col gap-2">
          <h1 className="text-xl font-semibold">Welcome to Flavor</h1>
          <p className="text-sm text-muted-foreground">
            Join several Tailscale and Headscale networks at once, each with its own session and device identity, so
            overlapping addresses never get mixed up.
          </p>
        </div>
        <div className="flex flex-wrap justify-center gap-2">
          <Button onClick={onAdd}>
            <Plus aria-hidden="true" />
            Add network
          </Button>
          <Button variant="ghost" onClick={onLater}>
            Set up later
          </Button>
        </div>
        <p className="text-xs text-muted-foreground">Nothing on this computer changes until you add a network.</p>
      </motion.div>
    </main>
  );
}

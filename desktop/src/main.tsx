import "@fontsource-variable/inter";
import "@fontsource-variable/jetbrains-mono";
import "./index.css";
import { MotionConfig } from "motion/react";
import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { App } from "./app/App";
import { DaemonSyncController } from "./app/sync/controller";
import { DaemonContext } from "./app/sync/useDaemon";
import { getSnapshot, onStreamMessage, watchEvents } from "./lib/api/daemon";
import { Toaster } from "./ui/sonner";

if (import.meta.env.VITE_FAKE_DAEMON === "1") {
  await import("./dev/fakeDaemon");
}

const controller = new DaemonSyncController({ getSnapshot, watchEvents, onStreamMessage });
void controller.start();

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <MotionConfig reducedMotion="user">
      <DaemonContext.Provider value={controller}>
        <App />
        <Toaster position="bottom-right" />
      </DaemonContext.Provider>
    </MotionConfig>
  </StrictMode>,
);

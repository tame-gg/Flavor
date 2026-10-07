import "@fontsource-variable/inter";
import "@fontsource-variable/jetbrains-mono";
import "./styles.css";
import { StrictMode } from "react";
import { createRoot } from "react-dom/client";
import { App } from "./app/App";
import { DaemonSyncController } from "./app/sync/controller";
import { DaemonContext } from "./app/sync/useDaemon";
import { getSnapshot, onStreamMessage, watchEvents } from "./lib/api/daemon";

const controller = new DaemonSyncController({ getSnapshot, watchEvents, onStreamMessage });
void controller.start();

createRoot(document.getElementById("root")!).render(
  <StrictMode>
    <DaemonContext.Provider value={controller}>
      <App />
    </DaemonContext.Provider>
  </StrictMode>,
);

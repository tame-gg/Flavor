import { createContext, useContext, useSyncExternalStore } from "react";
import type { DaemonSyncController, SyncState } from "./controller";

export const DaemonContext = createContext<DaemonSyncController | null>(null);

export function useDaemon(): SyncState & { controller: DaemonSyncController } {
  const controller = useContext(DaemonContext);
  if (!controller) throw new Error("DaemonContext missing");
  const state = useSyncExternalStore(controller.subscribe, controller.getState);
  return { ...state, controller };
}

export function useCanMutate(): boolean {
  return useDaemon().status === "ready";
}

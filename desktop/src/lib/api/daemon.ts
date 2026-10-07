import { fromJson, type JsonValue } from "@bufbuild/protobuf";
import { invoke } from "@tauri-apps/api/core";
import { listen } from "@tauri-apps/api/event";
import { GetStateSnapshotResponseSchema, type GetStateSnapshotResponse } from "@gen/lattice/v1/daemon_pb";
import { RunDiagnosticsResponseSchema, type RunDiagnosticsResponse } from "@gen/lattice/v1/diagnostics_pb";
import { ListConflictsResponseSchema, type ListConflictsResponse } from "@gen/lattice/v1/conflicts_pb";
import { DaemonEventSchema } from "@gen/lattice/v1/events_pb";
import {
  DescribeDeviceResponseSchema,
  InspectDestinationResponseSchema,
  type DescribeDeviceResponse,
  type InspectDestinationResponse,
} from "@gen/lattice/v1/inspector_pb";
import { NetworkSchema, type Network } from "@gen/lattice/v1/network_pb";
import {
  ActivateWorkspaceResponseSchema,
  WorkspaceSchema,
  type ActivateWorkspaceResponse,
  type Workspace,
} from "@gen/lattice/v1/workspaces_pb";
import type { LoadedSettings, RawStreamMessage, Settings, StreamMessage } from "./types";

const options = { ignoreUnknownFields: true };

export type Provider = "tailscale" | "headscale";

export async function getSnapshot(): Promise<GetStateSnapshotResponse> {
  return fromJson(GetStateSnapshotResponseSchema, await invoke<JsonValue>("get_snapshot"), options);
}

export async function watchEvents(watchId: number, instanceId: string, afterSequence: bigint): Promise<void> {
  await invoke("watch_events", { watchId, instanceId, afterSequence: afterSequence.toString() });
}

export function onStreamMessage(handler: (m: StreamMessage) => void): Promise<() => void> {
  return listen<RawStreamMessage>("lattice://daemon-event", ({ payload }) => {
    if (payload.kind === "event") {
      handler({ ...payload, event: fromJson(DaemonEventSchema, payload.event, options) });
    } else {
      handler(payload);
    }
  });
}

export async function addNetwork(input: {
  displayName: string;
  provider: Provider;
  controlUrl: string;
  autoConnect: boolean;
}): Promise<Network> {
  return fromJson(NetworkSchema, await invoke<JsonValue>("add_network", input), options);
}

export async function updateNetwork(networkId: string, patch: { displayName?: string; autoConnect?: boolean }): Promise<Network> {
  return fromJson(NetworkSchema, await invoke<JsonValue>("update_network", { networkId, ...patch }), options);
}

export const connectNetwork = (networkId: string) => invoke<void>("connect_network", { networkId });
export const disconnectNetwork = (networkId: string) => invoke<void>("disconnect_network", { networkId });
export const enrollNetwork = (networkId: string, preAuthKey: string) =>
  invoke<void>("enroll_network", { networkId, preAuthKey });
export const removeNetwork = (networkId: string) => invoke<void>("remove_network", { networkId });
export const deleteNetworkIdentity = (networkId: string) => invoke<void>("delete_network_identity", { networkId });
export const openAuthUrl = (networkId: string, flowId: string) => invoke<void>("open_auth_url", { networkId, flowId });

export async function runDiagnostics(): Promise<RunDiagnosticsResponse> {
  return fromJson(RunDiagnosticsResponseSchema, await invoke<JsonValue>("run_diagnostics"), options);
}

export async function inspectDestination(destination: string): Promise<InspectDestinationResponse> {
  return fromJson(InspectDestinationResponseSchema, await invoke<JsonValue>("inspect_destination", { destination }), options);
}

export async function listConflicts(): Promise<ListConflictsResponse> {
  return fromJson(ListConflictsResponseSchema, await invoke<JsonValue>("list_conflicts"), options);
}

export async function createWorkspace(input: { name: string; description: string; networkIds: string[] }): Promise<Workspace> {
  return fromJson(WorkspaceSchema, await invoke<JsonValue>("create_workspace", input), options);
}

export async function updateWorkspace(
  workspaceId: string,
  patch: { name?: string; description?: string; networkIds?: string[] },
): Promise<Workspace> {
  return fromJson(WorkspaceSchema, await invoke<JsonValue>("update_workspace", { workspaceId, ...patch }), options);
}

export const deleteWorkspace = (workspaceId: string) => invoke<void>("delete_workspace", { workspaceId });
export const deactivateWorkspace = () => invoke<void>("deactivate_workspace");

export async function activateWorkspace(workspaceId: string, disconnectOthers: boolean): Promise<ActivateWorkspaceResponse> {
  return fromJson(ActivateWorkspaceResponseSchema, await invoke<JsonValue>("activate_workspace", { workspaceId, disconnectOthers }), options);
}

export const setDestinationPreference = (destination: string, networkId: string) =>
  invoke<JsonValue>("set_destination_preference", { destination, networkId });
export const deleteDestinationPreference = (destination: string) => invoke<void>("delete_destination_preference", { destination });

export async function describeDevice(networkId: string, nodeId: string): Promise<DescribeDeviceResponse> {
  return fromJson(DescribeDeviceResponseSchema, await invoke<JsonValue>("describe_device", { networkId, nodeId }), options);
}

export const getSettings = () => invoke<LoadedSettings>("get_settings");
export const setSettings = (settings: Settings) => invoke<LoadedSettings>("set_settings", { settings });
export const setTraySummary = (text: string) => invoke<void>("set_tray_summary", { text });
export const appVersion = () => invoke<string>("app_version");

import { create } from "@bufbuild/protobuf";
import { NetworkConnectionState } from "@gen/lattice/v1/common_pb";
import type { GetDaemonInfoResponse, GetStateSnapshotResponse } from "@gen/lattice/v1/daemon_pb";
import type { Device } from "@gen/lattice/v1/device_pb";
import type { DaemonEvent } from "@gen/lattice/v1/events_pb";
import { AuthenticationPromptSchema, type Network } from "@gen/lattice/v1/network_pb";
import type { StreamMessage, UiError } from "../../lib/api/types";

export const PROTOCOL_MAJOR = 1;

export type DaemonStatus = "starting" | "ready" | "resyncing" | "unavailable" | "incompatible";

export type SyncState = {
  status: DaemonStatus;
  info: GetDaemonInfoResponse | null;
  instanceId: string;
  sequence: bigint;
  networks: ReadonlyMap<string, Network>;
  devices: ReadonlyMap<string, Device>;
  warnings: readonly string[];
  lastError: UiError | null;
};

export type SyncTransport = {
  getSnapshot(): Promise<GetStateSnapshotResponse>;
  watchEvents(watchId: number, instanceId: string, afterSequence: bigint): Promise<void>;
  onStreamMessage(handler: (m: StreamMessage) => void): Promise<() => void>;
};

export type Scheduler = (fn: () => void, ms: number) => () => void;

const stopped = new Set([
  NetworkConnectionState.DISCONNECTED,
  NetworkConnectionState.ERROR,
  NetworkConnectionState.DISABLED,
  NetworkConnectionState.REMOVING,
]);

export const deviceKey = (networkId: string, nodeId: string) => `${networkId}:${nodeId}`;

const initial: SyncState = {
  status: "starting",
  info: null,
  instanceId: "",
  sequence: 0n,
  networks: new Map(),
  devices: new Map(),
  warnings: [],
  lastError: null,
};

export class DaemonSyncController {
  private state = initial;
  private listeners = new Set<() => void>();
  private watchId = 0;
  private generation = 0;
  private retryDelay = 0;
  private cancelRetry: (() => void) | null = null;
  private unlisten: (() => void) | null = null;
  private disposed = false;

  constructor(
    private readonly transport: SyncTransport,
    private readonly schedule: Scheduler = (fn, ms) => {
      const t = setTimeout(fn, ms);
      return () => clearTimeout(t);
    },
  ) {}

  getState = (): SyncState => this.state;

  subscribe = (listener: () => void): (() => void) => {
    this.listeners.add(listener);
    return () => this.listeners.delete(listener);
  };

  async start(): Promise<void> {
    const unlisten = await this.transport.onStreamMessage((m) => this.receive(m));
    if (this.disposed) {
      unlisten();
      return;
    }
    this.unlisten = unlisten;
    await this.resync();
  }

  dispose(): void {
    this.disposed = true;
    this.unlisten?.();
    this.cancelRetry?.();
    this.generation++;
    this.watchId++;
  }

  async resync(): Promise<void> {
    if (this.disposed || this.state.status === "incompatible") return;
    this.cancelRetry?.();
    this.cancelRetry = null;
    const generation = ++this.generation;
    this.watchId++;
    this.set({ status: this.state.info ? "resyncing" : "starting" });
    let snap: GetStateSnapshotResponse;
    try {
      snap = await this.transport.getSnapshot();
    } catch (e) {
      if (generation === this.generation) this.fail(e as UiError);
      return;
    }
    if (generation !== this.generation) return;
    if (snap.daemon && snap.daemon.protocolMajor !== PROTOCOL_MAJOR) {
      this.set({ status: "incompatible", info: snap.daemon });
      return;
    }
    const devices = new Map<string, Device>();
    for (const d of snap.devices) {
      if (d.id) devices.set(deviceKey(d.id.networkId, d.id.nodeId), d);
    }
    this.set({
      info: snap.daemon ?? null,
      instanceId: snap.daemonInstanceId,
      sequence: snap.snapshotSequence,
      networks: new Map(snap.networks.map((n) => [n.id, n])),
      devices,
      warnings: [],
    });
    const watchId = ++this.watchId;
    try {
      await this.transport.watchEvents(watchId, snap.daemonInstanceId, snap.snapshotSequence);
    } catch (e) {
      if (generation === this.generation) this.fail(e as UiError);
      return;
    }
    if (generation !== this.generation) return;
    this.retryDelay = 0;
    this.set({ status: "ready", lastError: null });
  }

  private receive(m: StreamMessage): void {
    if (this.disposed || m.watchId !== this.watchId) return;
    if (m.kind === "closed") {
      if (m.error?.code === "LATTICE_ERROR_CODE_RESYNC_REQUIRED") {
        void this.resync();
      } else {
        this.fail(m.error ?? { kind: "unavailable", code: null, message: "event stream ended", retryable: true });
      }
      return;
    }
    const ev = m.event;
    if (ev.daemonInstanceId !== this.state.instanceId) {
      void this.resync();
      return;
    }
    if (ev.sequenceId <= this.state.sequence) return;
    if (ev.sequenceId !== this.state.sequence + 1n) {
      void this.resync();
      return;
    }
    this.apply(ev);
  }

  private apply(ev: DaemonEvent): void {
    const networks = new Map(this.state.networks);
    let devices = this.state.devices;
    let warnings = this.state.warnings;
    const editDevices = () => {
      if (devices === this.state.devices) devices = new Map(devices);
      return devices as Map<string, Device>;
    };
    const p = ev.payload;
    switch (p.case) {
      case "networkAdded":
      case "networkUpdated": {
        const n = p.value.network;
        if (!n) break;
        const prev = networks.get(n.id);
        networks.set(n.id, prev && p.case === "networkUpdated" ? { ...n, state: prev.state, authentication: prev.authentication } : n);
        break;
      }
      case "networkRemoved": {
        networks.delete(p.value.networkId);
        dropDevices(editDevices(), p.value.networkId);
        break;
      }
      case "networkStateChanged": {
        const prev = networks.get(p.value.networkId);
        if (!prev) break;
        const next = { ...prev, state: p.value.state };
        if (stopped.has(p.value.state)) {
          next.authentication = undefined;
          dropDevices(editDevices(), p.value.networkId);
        } else if (prev.authentication?.kind === "approval" && p.value.state !== NetworkConnectionState.AWAITING_APPROVAL) {
          next.authentication = undefined;
        } else if (p.value.state === NetworkConnectionState.CONNECTED) {
          next.authentication = undefined;
        }
        networks.set(prev.id, next);
        break;
      }
      case "authenticationRequired": {
        const prev = networks.get(p.value.networkId);
        if (!prev) break;
        networks.set(prev.id, {
          ...prev,
          authentication: create(AuthenticationPromptSchema, {
            flowId: p.value.flowId,
            kind: "authentication",
            authUrl: p.value.authUrl,
            provider: p.value.provider,
            createdAt: ev.timestamp,
          }),
        });
        break;
      }
      case "authenticationCompleted": {
        const prev = networks.get(p.value.networkId);
        if (prev) networks.set(prev.id, { ...prev, authentication: undefined });
        break;
      }
      case "approvalRequired": {
        const prev = networks.get(p.value.networkId);
        if (!prev) break;
        networks.set(prev.id, {
          ...prev,
          state: NetworkConnectionState.AWAITING_APPROVAL,
          authentication: create(AuthenticationPromptSchema, { kind: "approval", provider: p.value.provider }),
        });
        break;
      }
      case "peerAdded":
      case "peerUpdated": {
        const d = p.value.device;
        if (d?.id && networks.has(d.id.networkId)) editDevices().set(deviceKey(d.id.networkId, d.id.nodeId), d);
        break;
      }
      case "peerRemoved": {
        const id = p.value.id;
        if (id) editDevices().delete(deviceKey(id.networkId, id.nodeId));
        break;
      }
      case "daemonWarning": {
        warnings = [...warnings, p.value.safeMessage].slice(-20);
        break;
      }
    }
    this.set({ sequence: ev.sequenceId, networks, devices, warnings });
  }

  private fail(error: UiError): void {
    this.watchId++;
    this.set({ status: "unavailable", lastError: error });
    this.retryDelay = Math.min(Math.max(this.retryDelay * 2, 500), 8000);
    this.cancelRetry?.();
    this.cancelRetry = this.schedule(() => {
      this.cancelRetry = null;
      void this.resync();
    }, this.retryDelay);
  }

  private set(patch: Partial<SyncState>): void {
    this.state = { ...this.state, ...patch };
    for (const l of this.listeners) l();
  }
}

function dropDevices(devices: Map<string, Device>, networkId: string): void {
  const prefix = `${networkId}:`;
  for (const key of devices.keys()) {
    if (key.startsWith(prefix)) devices.delete(key);
  }
}

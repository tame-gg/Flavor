import { create } from "@bufbuild/protobuf";
import { NetworkConnectionState, ProviderType } from "@gen/lattice/v1/common_pb";
import { GetDaemonInfoResponseSchema, GetStateSnapshotResponseSchema, type GetStateSnapshotResponse } from "@gen/lattice/v1/daemon_pb";
import { DeviceSchema, type Device } from "@gen/lattice/v1/device_pb";
import { DaemonEventSchema, type DaemonEvent } from "@gen/lattice/v1/events_pb";
import { AuthenticationPromptSchema, NetworkSchema, type Network } from "@gen/lattice/v1/network_pb";
import { describe, expect, it } from "vitest";
import type { StreamMessage, UiError } from "../../lib/api/types";
import { DaemonSyncController, deviceKey, type SyncTransport } from "./controller";

const net = (id: string, state = NetworkConnectionState.CONNECTED) =>
  create(NetworkSchema, { id, displayName: id, provider: ProviderType.HEADSCALE, controlUrl: "https://hs.example.com", state });

const dev = (networkId: string, nodeId: string, ip: string) =>
  create(DeviceSchema, { id: { networkId, nodeId }, hostname: nodeId, addresses: [ip], online: true });

const snapshot = (instance: string, seq: bigint, extra: { networks?: Network[]; devices?: Device[] } = {}) =>
  create(GetStateSnapshotResponseSchema, {
    daemonInstanceId: instance,
    snapshotSequence: seq,
    daemon: create(GetDaemonInfoResponseSchema, { protocolMajor: 1, daemonInstanceId: instance, daemonVersion: "test" }),
    ...extra,
  });

const event = (instance: string, seq: bigint, payload: DaemonEvent["payload"]) =>
  create(DaemonEventSchema, { daemonInstanceId: instance, sequenceId: seq, payload });

class FakeTransport implements SyncTransport {
  snapshots: GetStateSnapshotResponse[] = [];
  snapshotCalls = 0;
  watches: { watchId: number; instanceId: string; after: bigint }[] = [];
  failSnapshot: UiError | null = null;
  private handler: ((m: StreamMessage) => void) | null = null;

  async getSnapshot() {
    this.snapshotCalls++;
    if (this.failSnapshot) throw this.failSnapshot;
    const s = this.snapshots.length > 1 ? this.snapshots.shift()! : this.snapshots[0];
    return s;
  }
  async watchEvents(watchId: number, instanceId: string, after: bigint) {
    this.watches.push({ watchId, instanceId, after });
  }
  async onStreamMessage(h: (m: StreamMessage) => void) {
    this.handler = h;
    return () => {
      this.handler = null;
    };
  }
  get watchId() {
    return this.watches[this.watches.length - 1].watchId;
  }
  emit(ev: DaemonEvent, watchId = this.watchId) {
    this.handler?.({ kind: "event", watchId, event: ev });
  }
  close(error: UiError | null, watchId = this.watchId) {
    this.handler?.({ kind: "closed", watchId, error });
  }
}

const flush = () => new Promise((r) => setTimeout(r, 0));

function setup(...snaps: GetStateSnapshotResponse[]) {
  const t = new FakeTransport();
  t.snapshots = snaps;
  const timers: (() => void)[] = [];
  const c = new DaemonSyncController(t, (fn) => {
    timers.push(fn);
    return () => {};
  });
  return { t, c, timers };
}

describe("DaemonSyncController", () => {
  it("watches from the snapshot baseline and applies ordered events", async () => {
    const { t, c } = setup(snapshot("i1", 5n, { networks: [net("a")] }));
    await c.start();
    expect(c.getState().status).toBe("ready");
    expect(t.watches[0]).toMatchObject({ instanceId: "i1", after: 5n });

    t.emit(event("i1", 5n, { case: "networkRemoved", value: { networkId: "a" } as never }));
    expect(c.getState().networks.has("a")).toBe(true);

    t.emit(event("i1", 6n, { case: "peerAdded", value: { device: dev("a", "n1", "100.64.0.1") } as never }));
    expect(c.getState().sequence).toBe(6n);
    expect(c.getState().devices.get(deviceKey("a", "n1"))?.addresses).toEqual(["100.64.0.1"]);
  });

  it("keeps duplicate addresses across networks as distinct devices", async () => {
    const { c } = setup(
      snapshot("i1", 1n, {
        networks: [net("a"), net("b")],
        devices: [dev("a", "n1", "100.64.0.1"), dev("b", "n1", "100.64.0.1")],
      }),
    );
    await c.start();
    expect([...c.getState().devices.keys()].sort()).toEqual(["a:n1", "b:n1"]);
  });

  it("resyncs on a sequence gap", async () => {
    const { t, c } = setup(snapshot("i1", 1n, { networks: [net("a")] }), snapshot("i1", 9n, { networks: [net("a"), net("b")] }));
    await c.start();
    t.emit(event("i1", 3n, { case: "networkRemoved", value: { networkId: "a" } as never }));
    await flush();
    expect(t.snapshotCalls).toBe(2);
    expect(c.getState().networks.has("a")).toBe(true);
    expect(c.getState().sequence).toBe(9n);
    expect(t.watches[1].after).toBe(9n);
  });

  it("resyncs when the daemon instance changes", async () => {
    const { t, c } = setup(snapshot("i1", 1n), snapshot("i2", 1n));
    await c.start();
    t.emit(event("i2", 2n, { case: "daemonWarning", value: { code: "x", safeMessage: "y" } as never }));
    await flush();
    expect(c.getState().instanceId).toBe("i2");
    expect(t.watches[1].instanceId).toBe("i2");
  });

  it("resyncs when the stream reports RESYNC_REQUIRED", async () => {
    const { t, c } = setup(snapshot("i1", 1n));
    await c.start();
    t.close({ kind: "daemon", code: "LATTICE_ERROR_CODE_RESYNC_REQUIRED", message: "", retryable: true });
    await flush();
    expect(t.snapshotCalls).toBe(2);
    expect(c.getState().status).toBe("ready");
  });

  it("ignores messages from superseded streams", async () => {
    const { t, c } = setup(snapshot("i1", 1n, { networks: [net("a")] }));
    await c.start();
    const old = t.watchId;
    t.close({ kind: "daemon", code: "LATTICE_ERROR_CODE_RESYNC_REQUIRED", message: "", retryable: true });
    await flush();
    t.emit(event("i1", 2n, { case: "networkRemoved", value: { networkId: "a" } as never }), old);
    t.close(null, old);
    expect(c.getState().networks.has("a")).toBe(true);
    expect(c.getState().status).toBe("ready");
  });

  it("keeps last-known data while unavailable and recovers", async () => {
    const { t, c, timers } = setup(snapshot("i1", 1n, { networks: [net("a")] }));
    await c.start();
    t.failSnapshot = { kind: "unavailable", code: null, message: "down", retryable: true };
    t.close(null);
    expect(c.getState().status).toBe("unavailable");
    expect(c.getState().networks.has("a")).toBe(true);
    timers.shift()!();
    await flush();
    expect(c.getState().status).toBe("unavailable");
    t.failSnapshot = null;
    timers.shift()!();
    await flush();
    expect(c.getState().status).toBe("ready");
  });

  it("stops at a protocol major mismatch without watching", async () => {
    const s = snapshot("i1", 1n);
    s.daemon!.protocolMajor = 2;
    const { t, c } = setup(s);
    await c.start();
    expect(c.getState().status).toBe("incompatible");
    expect(t.watches).toHaveLength(0);
  });

  it("restores an active auth prompt from the snapshot after a GUI restart", async () => {
    const pending = net("a", NetworkConnectionState.AUTHENTICATING);
    pending.authentication = create(AuthenticationPromptSchema, {
      flowId: "f1",
      kind: "authentication",
      authUrl: "https://login.example.com/a/1",
    });
    const { c } = setup(snapshot("i1", 4n, { networks: [pending] }));
    await c.start();
    expect(c.getState().networks.get("a")?.authentication?.flowId).toBe("f1");
  });

  it("tracks auth prompts from events and clears them on completion or stop", async () => {
    const { t, c } = setup(snapshot("i1", 1n, { networks: [net("a", NetworkConnectionState.CONNECTING)] }));
    await c.start();
    t.emit(event("i1", 2n, { case: "authenticationRequired", value: { networkId: "a", flowId: "f2", authUrl: "https://idp.example.org/x" } as never }));
    expect(c.getState().networks.get("a")?.authentication?.flowId).toBe("f2");
    t.emit(event("i1", 3n, { case: "authenticationCompleted", value: { networkId: "a" } as never }));
    expect(c.getState().networks.get("a")?.authentication).toBeUndefined();

    t.emit(event("i1", 4n, { case: "authenticationRequired", value: { networkId: "a", flowId: "f3", authUrl: "https://idp.example.org/y" } as never }));
    t.emit(event("i1", 5n, { case: "peerAdded", value: { device: dev("a", "n1", "100.64.0.9") } as never }));
    t.emit(event("i1", 6n, { case: "networkStateChanged", value: { networkId: "a", state: NetworkConnectionState.DISCONNECTED } as never }));
    expect(c.getState().networks.get("a")?.authentication).toBeUndefined();
    expect(c.getState().devices.size).toBe(0);
  });

  it("keeps runtime state when a network's config is updated", async () => {
    const { t, c } = setup(snapshot("i1", 1n, { networks: [net("a", NetworkConnectionState.CONNECTED)] }));
    await c.start();
    const renamed = net("a", NetworkConnectionState.UNSPECIFIED);
    renamed.displayName = "Renamed";
    t.emit(event("i1", 2n, { case: "networkUpdated", value: { network: renamed } as never }));
    expect(c.getState().networks.get("a")).toMatchObject({ displayName: "Renamed", state: NetworkConnectionState.CONNECTED });
  });
});

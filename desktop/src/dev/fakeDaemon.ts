import { create, toJson, type DescMessage, type MessageShape } from "@bufbuild/protobuf";
import { timestampNow } from "@bufbuild/protobuf/wkt";
import { mockIPC } from "@tauri-apps/api/mocks";
import { Capability, NetworkConnectionState as S, ProviderType } from "@gen/flavor/v1/common_pb";
import {
  ConflictScope,
  ConflictSeverity,
  ConflictType,
  ListConflictsResponseSchema,
} from "@gen/flavor/v1/conflicts_pb";
import { GetStateSnapshotResponseSchema } from "@gen/flavor/v1/daemon_pb";
import { DeviceSchema, type Device } from "@gen/flavor/v1/device_pb";
import { RunDiagnosticsResponseSchema } from "@gen/flavor/v1/diagnostics_pb";
import { ListDNSResponseSchema } from "@gen/flavor/v1/dns_pb";
import {
  CandidateStatus,
  DescribeDeviceResponseSchema,
  InspectDestinationResponseSchema,
  MatchKind,
  ResolutionDecision,
  DecisionReason,
} from "@gen/flavor/v1/inspector_pb";
import { NetworkSchema, type Network } from "@gen/flavor/v1/network_pb";
import { GetRoutesResponseSchema, ListSubnetRoutesResponseSchema, SubnetRouteUse } from "@gen/flavor/v1/routes_pb";
import { WorkspaceSchema } from "@gen/flavor/v1/workspaces_pb";

const json = <D extends DescMessage>(schema: D, value: MessageShape<D>) => toJson(schema, value);

const networks: Network[] = [
  create(NetworkSchema, {
    id: "01LUNARLABS0000000000000000",
    displayName: "LunarLabs",
    provider: ProviderType.HEADSCALE,
    controlUrl: "https://hs.lunarlabs.example",
    autoConnect: true,
    nodeHostname: "luna-desktop",
    state: S.CONNECTED,
    label: "lunarlabs",
  }),
  create(NetworkSchema, {
    id: "01DARKLESS00000000000000000",
    displayName: "Darkless",
    provider: ProviderType.HEADSCALE,
    controlUrl: "https://hs.darkless.example",
    autoConnect: true,
    nodeHostname: "luna-desktop",
    state: S.DEGRADED,
    label: "darkless",
  }),
  create(NetworkSchema, {
    id: "01PLUTOFLAVOR00000000000000",
    displayName: "PlutoFlavor",
    provider: ProviderType.TAILSCALE,
    autoConnect: true,
    nodeHostname: "luna-desktop",
    state: S.CONNECTED,
    label: "plutoflavor",
  }),
];

type Row = [
  network: number,
  node: string,
  host: string,
  ip: string,
  online: boolean,
  os: string,
  extra?: Partial<Pick<Device, "local" | "routes" | "exitNodeOption" | "exitNode">>,
];
const rows: Row[] = [
  [0, "1", "luna-desktop", "100.64.0.1", true, "linux", { local: true }],
  [0, "2", "ll-prod-app-01", "100.64.0.13", true, "linux", { routes: ["10.20.0.0/24"] }],
  [0, "3", "sona-prod-core-01", "100.64.0.14", true, "linux"],
  [0, "4", "git-runner", "100.64.0.21", true, "linux"],
  [0, "5", "nyc-1", "100.64.0.30", true, "linux", { exitNodeOption: true, exitNode: true }],
  [0, "6", "build-mac", "100.64.0.31", false, "macOS"],
  [1, "1", "luna-desktop", "100.70.0.2", true, "linux", { local: true }],
  [1, "2", "dk-db-01", "100.64.0.14", true, "linux"],
  [1, "3", "dk-edge", "100.70.0.9", true, "linux", { routes: ["192.168.40.0/24"], exitNodeOption: true }],
  [1, "4", "pixel-8", "100.70.0.12", false, "android"],
  [2, "1", "luna-desktop", "100.101.4.7", true, "linux", { local: true }],
  [2, "2", "nc-game-budget-01", "100.64.0.14", true, "linux"],
  [2, "3", "media-box", "100.101.4.20", true, "linux"],
  [2, "4", "iphone", "100.101.4.33", true, "iOS"],
];

const devices = rows.map(([n, node, host, ip, online, os, extra]) =>
  create(DeviceSchema, {
    id: { networkId: networks[n].id, nodeId: node },
    hostname: host,
    dnsName: `${host}.${networks[n].label}.ts.net`,
    addresses: [ip],
    online,
    lastSeen: timestampNow(),
    os,
    ...extra,
  }),
);

const snapshot = create(GetStateSnapshotResponseSchema, {
  daemonInstanceId: "fake-instance",
  snapshotSequence: 42n,
  daemon: {
    daemonVersion: "0.1.0-dev",
    protocolMajor: 1,
    protocolMinor: 3,
    daemonInstanceId: "fake-instance",
    buildCommit: "fake",
    capabilities: [
      Capability.HEADSCALE,
      Capability.DEVICE_SNAPSHOTS,
      Capability.CONNECTION_INSPECTOR,
      Capability.CONFLICT_CENTER,
      Capability.WORKSPACES,
      Capability.DESTINATION_PREFERENCES,
      Capability.FORWARDING,
      Capability.SOCKS_PROXY,
      Capability.EXIT_NODES,
      Capability.ROUTES,
    ],
  },
  networks,
  devices,
  capturedAt: timestampNow(),
  workspaces: [
    create(WorkspaceSchema, {
      id: "ws-work",
      name: "Work",
      description: "Company networks",
      networkIds: [networks[0].id, networks[1].id],
    }),
  ],
  activeWorkspaceId: "ws-work",
});

const ref = (n: Network) => ({ id: n.id, displayName: n.displayName, provider: n.provider, state: n.state });
const deviceAt = (n: number, node: string) =>
  devices.find((d) => d.id?.networkId === networks[n].id && d.id?.nodeId === node)!;

let settings = { theme: "system", closeBehavior: "tray" };

const handlers: Record<string, (args: Record<string, unknown>) => unknown> = {
  get_snapshot: () => json(GetStateSnapshotResponseSchema, snapshot),
  watch_events: () => null,
  get_settings: () => ({ ...settings, warning: null }),
  set_settings: (a) => {
    settings = a.settings as typeof settings;
    return { ...settings, warning: null };
  },
  app_version: () => "0.1.0-dev",
  run_diagnostics: () =>
    json(
      RunDiagnosticsResponseSchema,
      create(RunDiagnosticsResponseSchema, {
        secretStoreBackend: "secret-service",
        secretStoreState: "ok",
        checks: [
          { name: "daemon", status: "ok", safeDetail: "flavord 0.1.0-dev" },
          { name: "database", status: "ok", safeDetail: "schema version 6" },
          {
            name: `network/${networks[1].id}`,
            status: "warn",
            safeDetail:
              "headscale, control hs.darkless.example, state degraded; control server reports: DERP region fra is unreachable",
          },
        ],
      }),
    ),
  list_conflicts: () =>
    json(
      ListConflictsResponseSchema,
      create(ListConflictsResponseSchema, {
        daemonInstanceId: "fake-instance",
        snapshotSequence: 42n,
        conflicts: [
          {
            id: "address:100.64.0.14",
            type: ConflictType.ADDRESS_COLLISION,
            severity: ConflictSeverity.AMBIGUOUS,
            scope: ConflictScope.CROSS_NETWORK,
            value: "100.64.0.14",
            members: [
              { network: ref(networks[0]), device: deviceAt(0, "3") },
              { network: ref(networks[1]), device: deviceAt(1, "2") },
              { network: ref(networks[2]), device: deviceAt(2, "2") },
            ],
          },
        ],
      }),
    ),
  inspect_destination: (a) => {
    const q = String(a.destination).toLowerCase();
    const matches = devices.filter(
      (d) => !d.local && (d.addresses.includes(q) || d.hostname === q || d.dnsName.startsWith(`${q}.`)),
    );
    const byRecord = q.startsWith("git.intra") ? [deviceAt(0, "2")] : [];
    const found = matches.length > 0 ? matches : byRecord;
    const decision =
      found.length === 0
        ? ResolutionDecision.NO_MATCH
        : found.length === 1
          ? ResolutionDecision.UNIQUE
          : ResolutionDecision.AMBIGUOUS;
    const match =
      byRecord.length && !matches.length
        ? MatchKind.DNS_RECORD
        : q.includes(".") || /^\d/.test(q)
          ? MatchKind.DEVICE_ADDRESS
          : MatchKind.DEVICE_HOSTNAME;
    return json(
      InspectDestinationResponseSchema,
      create(InspectDestinationResponseSchema, {
        daemonInstanceId: "fake-instance",
        snapshotSequence: 42n,
        query: q,
        normalized: q,
        decision,
        reason:
          decision === ResolutionDecision.AMBIGUOUS
            ? DecisionReason.MULTIPLE_MATCHES
            : decision === ResolutionDecision.NO_MATCH
              ? DecisionReason.NO_MATCH
              : DecisionReason.DNS_RECORD,
        decidedBy: match,
        candidates: found.map((d) => ({
          network: ref(networks.find((n) => n.id === d.id?.networkId)!),
          device: d,
          match,
          matchedValue: d.addresses[0],
          status: decision === ResolutionDecision.UNIQUE ? CandidateStatus.SELECTED : CandidateStatus.TIED,
        })),
      }),
    );
  },
  describe_device: (a) => {
    const d = devices.find((x) => x.id?.networkId === a.networkId && x.id?.nodeId === a.nodeId);
    const n = networks.find((x) => x.id === a.networkId);
    return json(
      DescribeDeviceResponseSchema,
      create(DescribeDeviceResponseSchema, {
        name: d && n ? `${d.hostname}.${n.label}.flavor.internal` : "",
        stableName: d && n ? `id-${d.id?.nodeId}.${n.label}.flavor.internal` : "",
      }),
    );
  },
  list_dns: (a) =>
    json(
      ListDNSResponseSchema,
      create(ListDNSResponseSchema, {
        networks:
          a.networkId === networks[0].id
            ? [
                {
                  network: ref(networks[0]),
                  records: [{ name: "git.intra.lunarlabs.internal", addresses: ["100.64.0.13"] }],
                  searchDomains: ["lunarlabs.internal"],
                },
              ]
            : [],
      }),
    ),
  list_subnet_routes: (a) =>
    json(
      ListSubnetRoutesResponseSchema,
      create(ListSubnetRoutesResponseSchema, {
        routes:
          a.networkId === networks[0].id
            ? [
                {
                  network: ref(networks[0]),
                  prefix: "10.20.0.0/24",
                  routers: [deviceAt(0, "2")],
                  use: SubnetRouteUse.NOT_USED,
                },
              ]
            : [],
      }),
    ),
  get_routes: () => json(GetRoutesResponseSchema, create(GetRoutesResponseSchema, {})),
};

mockIPC(
  (cmd, args) => {
    const handler = handlers[cmd];
    return handler ? handler((args ?? {}) as Record<string, unknown>) : null;
  },
  { shouldMockEvents: true },
);

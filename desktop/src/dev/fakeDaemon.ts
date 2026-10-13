import { create, toJson, type DescMessage, type MessageShape } from "@bufbuild/protobuf";
import { timestampFromMs, timestampNow } from "@bufbuild/protobuf/wkt";
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
import { ActivateWorkspaceResponseSchema, ActivationOutcome, WorkspaceSchema } from "@gen/flavor/v1/workspaces_pb";

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
    createdAt: timestampFromMs(1_760_000_000_000),
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
    statusMessage: "DERP region fra is unreachable",
    createdAt: timestampFromMs(1_760_000_100_000),
  }),
  create(NetworkSchema, {
    id: "01PLUTOFLAVOR00000000000000",
    displayName: "PlutoFlavor",
    provider: ProviderType.TAILSCALE,
    autoConnect: true,
    nodeHostname: "luna-desktop",
    state: S.CONNECTED,
    label: "plutoflavor",
    createdAt: timestampFromMs(1_760_000_200_000),
  }),
  create(NetworkSchema, {
    id: "01HOMELAB000000000000000000",
    displayName: "Home lab",
    provider: ProviderType.HEADSCALE,
    controlUrl: "https://hs.home.example",
    autoConnect: false,
    nodeHostname: "luna-desktop",
    state: S.AUTHENTICATING,
    label: "home-lab",
    authentication: {
      flowId: "flow-1",
      kind: "authentication",
      authUrl: "https://hs.home.example/register/xyz",
      provider: ProviderType.HEADSCALE,
    },
    createdAt: timestampFromMs(1_760_000_300_000),
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
const preferences = new Map<string, string>();

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
        secretStoreState: "available",
        checks: [
          { name: "daemon", status: "ok", safeDetail: "flavord 0.1.0-dev" },
          { name: "database", status: "ok", safeDetail: "schema version 6" },
          { name: "secret_store", status: "ok", safeDetail: "backend secret-service, state available" },
          {
            name: `network/${networks[0].id}`,
            status: "ok",
            safeDetail: "headscale, control hs.lunarlabs.example, state connected, 5 devices, 2 local addresses",
          },
          {
            name: `network/${networks[1].id}`,
            status: "warning",
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
      (d) =>
        !d.local && (d.addresses.includes(q) || d.hostname === q || d.dnsName === q || d.dnsName.startsWith(`${q}.`)),
    );
    const byRecord = q === "git.intra" || q === "git.intra.lunarlabs.internal" ? [deviceAt(0, "2")] : [];
    const found = matches.length > 0 ? matches : byRecord;
    const preferred = preferences.get(q);
    const chosen = preferred ? found.find((d) => d.id?.networkId === preferred) : undefined;
    const decision =
      found.length === 0
        ? ResolutionDecision.NO_MATCH
        : found.length === 1 || chosen
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
        reason: chosen
          ? DecisionReason.DESTINATION_PREFERENCE
          : decision === ResolutionDecision.AMBIGUOUS
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
          status: chosen
            ? d === chosen
              ? CandidateStatus.SELECTED
              : CandidateStatus.OUTRANKED
            : decision === ResolutionDecision.UNIQUE
              ? CandidateStatus.SELECTED
              : CandidateStatus.TIED,
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
  set_destination_preference: (a) => {
    preferences.set(String(a.destination), String(a.networkId));
    return { destination: a.destination, networkId: a.networkId };
  },
  delete_destination_preference: (a) => {
    preferences.delete(String(a.destination));
    return null;
  },
  activate_workspace: (a) =>
    json(
      ActivateWorkspaceResponseSchema,
      create(ActivateWorkspaceResponseSchema, {
        workspace: snapshot.workspaces.find((w) => w.id === a.workspaceId),
        results: networks.slice(0, 2).map((n) => ({ network: ref(n), outcome: ActivationOutcome.ALREADY_ACTIVE })),
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

import { NetworkConnectionState as S } from "@gen/flavor/v1/common_pb";

const labels: Record<S, string> = {
  [S.UNSPECIFIED]: "Unknown",
  [S.DISABLED]: "Disabled",
  [S.DISCONNECTED]: "Disconnected",
  [S.CONNECTING]: "Connecting",
  [S.AUTHENTICATING]: "Sign-in required",
  [S.AWAITING_APPROVAL]: "Awaiting approval",
  [S.CONNECTED]: "Connected",
  [S.DEGRADED]: "Degraded",
  [S.RECONNECTING]: "Reconnecting",
  [S.REMOVING]: "Removing",
  [S.ERROR]: "Error",
};

export const stateLabel = (state: S) => labels[state] ?? "Unknown";

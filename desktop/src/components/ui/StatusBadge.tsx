import { NetworkConnectionState as S } from "@gen/lattice/v1/common_pb";

const labels: Record<S, [string, string]> = {
  [S.UNSPECIFIED]: ["Unknown", ""],
  [S.DISABLED]: ["Disabled", ""],
  [S.DISCONNECTED]: ["Disconnected", ""],
  [S.CONNECTING]: ["Connecting", "badge-info"],
  [S.AUTHENTICATING]: ["Sign-in required", "badge-warn"],
  [S.AWAITING_APPROVAL]: ["Awaiting approval", "badge-warn"],
  [S.CONNECTED]: ["Connected", "badge-ok"],
  [S.DEGRADED]: ["Degraded", "badge-warn"],
  [S.RECONNECTING]: ["Reconnecting", "badge-info"],
  [S.REMOVING]: ["Removing", ""],
  [S.ERROR]: ["Error", "badge-danger"],
};

export const stateLabel = (state: S) => labels[state]?.[0] ?? "Unknown";

export function StatusBadge({ state }: { state: S }) {
  const [label, tone] = labels[state] ?? labels[S.UNSPECIFIED];
  return <span className={`badge ${tone}`}>{label}</span>;
}

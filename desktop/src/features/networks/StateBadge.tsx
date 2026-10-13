import type { ReactNode } from "react";
import type { NetworkConnectionState } from "@gen/flavor/v1/common_pb";
import { stateLabel } from "@/lib/networkState";
import { Badge } from "@/ui/badge";
import { stateTone, type Tone } from "./format";

const toneClass: Record<Tone, string> = {
  ok: "bg-success/15 text-success",
  warn: "bg-warning/15 text-warning",
  danger: "bg-destructive/15 text-destructive",
  info: "bg-primary/15 text-primary",
  neutral: "bg-muted text-muted-foreground",
};

export function ToneBadge({ tone, children }: { tone: Tone; children: ReactNode }) {
  return (
    <Badge variant="secondary" className={toneClass[tone]}>
      {children}
    </Badge>
  );
}

export function StateBadge({ state }: { state: NetworkConnectionState }) {
  return <ToneBadge tone={stateTone(state)}>{stateLabel(state)}</ToneBadge>;
}

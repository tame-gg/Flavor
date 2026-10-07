import type { JsonValue } from "@bufbuild/protobuf";
import type { DaemonEvent } from "@gen/lattice/v1/events_pb";

export type UiError = {
  kind: "daemon" | "unavailable" | "transport" | "invalid";
  code: string | null;
  message: string;
  retryable: boolean;
};

export type RawStreamMessage =
  | { kind: "event"; watchId: number; event: JsonValue }
  | { kind: "closed"; watchId: number; error: UiError | null };

export type StreamMessage =
  | { kind: "event"; watchId: number; event: DaemonEvent }
  | { kind: "closed"; watchId: number; error: UiError | null };

export type Theme = "system" | "light" | "dark";
export type CloseBehavior = "tray" | "quit_gui";
export type Settings = { theme: Theme; closeBehavior: CloseBehavior };
export type LoadedSettings = Settings & { warning: string | null };

export function isUiError(e: unknown): e is UiError {
  return typeof e === "object" && e !== null && "kind" in e && "message" in e;
}

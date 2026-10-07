import { isUiError } from "./types";

const copy: Record<string, string> = {
  LATTICE_ERROR_CODE_NETWORK_NOT_FOUND: "This network no longer exists.",
  LATTICE_ERROR_CODE_INVALID_CONTROL_URL: "Enter a control server address starting with https:// or http://, without a username or password.",
  LATTICE_ERROR_CODE_CONTROL_SERVER_UNREACHABLE: "The control server could not be reached.",
  LATTICE_ERROR_CODE_AUTHENTICATION_REQUIRED: "Sign-in is required for this network.",
  LATTICE_ERROR_CODE_AUTHENTICATION_FAILED: "Sign-in failed. Try again.",
  LATTICE_ERROR_CODE_APPROVAL_REQUIRED: "An administrator must approve this device.",
  LATTICE_ERROR_CODE_NETWORK_ALREADY_CONNECTED: "This network is already connected or connecting.",
  LATTICE_ERROR_CODE_NETWORK_BUSY: "Another change to this network is still in progress. Try again in a moment.",
  LATTICE_ERROR_CODE_SECRET_STORE_UNAVAILABLE: "The system keyring is unavailable.",
  LATTICE_ERROR_CODE_DAEMON_PERMISSION_DENIED: "The Lattice daemon refused this connection.",
  LATTICE_ERROR_CODE_STATE_DIRECTORY_ERROR: "Lattice could not access this network's local state.",
  LATTICE_ERROR_CODE_DAEMON_SHUTTING_DOWN: "The Lattice daemon is shutting down.",
  LATTICE_ERROR_CODE_INVALID_ARGUMENT: "Some of the details entered are not valid.",
  LATTICE_ERROR_CODE_INTERNAL: "The Lattice daemon hit an internal error.",
};

export function errorMessage(e: unknown): string {
  if (!isUiError(e)) return "Something went wrong.";
  if (e.kind === "unavailable") return "The Lattice daemon is not running.";
  if (e.kind === "invalid") return e.message;
  if (e.code === "LATTICE_ERROR_CODE_STATE_DIRECTORY_ERROR" && e.message) return e.message;
  return (e.code && copy[e.code]) || "The Lattice daemon could not complete this request.";
}

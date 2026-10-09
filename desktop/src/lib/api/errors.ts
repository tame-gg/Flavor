import { isUiError } from "./types";

const copy: Record<string, string> = {
  FLAVOR_ERROR_CODE_NETWORK_NOT_FOUND: "This network no longer exists.",
  FLAVOR_ERROR_CODE_INVALID_CONTROL_URL: "Enter a control server address such as vpn.example.com or https://vpn.example.com, without a username or password.",
  FLAVOR_ERROR_CODE_CONTROL_SERVER_UNREACHABLE: "The control server could not be reached.",
  FLAVOR_ERROR_CODE_AUTHENTICATION_REQUIRED: "Sign-in is required for this network.",
  FLAVOR_ERROR_CODE_AUTHENTICATION_FAILED: "Sign-in failed. Try again.",
  FLAVOR_ERROR_CODE_APPROVAL_REQUIRED: "An administrator must approve this device.",
  FLAVOR_ERROR_CODE_NETWORK_ALREADY_CONNECTED: "This network is already connected or connecting.",
  FLAVOR_ERROR_CODE_NETWORK_BUSY: "Another change to this network is still in progress. Try again in a moment.",
  FLAVOR_ERROR_CODE_SECRET_STORE_UNAVAILABLE: "The system keyring is unavailable.",
  FLAVOR_ERROR_CODE_DAEMON_PERMISSION_DENIED: "The Flavor daemon refused this connection.",
  FLAVOR_ERROR_CODE_STATE_DIRECTORY_ERROR: "Flavor could not access this network's local state.",
  FLAVOR_ERROR_CODE_DAEMON_SHUTTING_DOWN: "The Flavor daemon is shutting down.",
  FLAVOR_ERROR_CODE_INVALID_ARGUMENT: "Some of the details entered are not valid.",
  FLAVOR_ERROR_CODE_INTERNAL: "The Flavor daemon hit an internal error.",
};

export function errorMessage(e: unknown): string {
  if (!isUiError(e)) return "Something went wrong.";
  if (e.kind === "unavailable") return "The Flavor daemon is not running.";
  if (e.kind === "invalid") return e.message;
  if (e.code === "FLAVOR_ERROR_CODE_STATE_DIRECTORY_ERROR" && e.message) return e.message;
  return (e.code && copy[e.code]) || "The Flavor daemon could not complete this request.";
}

package domain

import "errors"

type NetworkConnectionState string

const (
	StateDisabled         NetworkConnectionState = "disabled"
	StateDisconnected     NetworkConnectionState = "disconnected"
	StateConnecting       NetworkConnectionState = "connecting"
	StateAuthenticating   NetworkConnectionState = "authenticating"
	StateAwaitingApproval NetworkConnectionState = "awaiting_approval"
	StateConnected        NetworkConnectionState = "connected"
	StateDegraded         NetworkConnectionState = "degraded"
	StateReconnecting     NetworkConnectionState = "reconnecting"
	StateRemoving         NetworkConnectionState = "removing"
	StateError            NetworkConnectionState = "error"
)

var ErrInvalidConnectionState = errors.New("invalid connection state")

func ParseConnectionState(s string) (NetworkConnectionState, error) {
	switch NetworkConnectionState(s) {
	case StateDisabled, StateDisconnected, StateConnecting, StateAuthenticating,
		StateAwaitingApproval, StateConnected, StateDegraded, StateReconnecting,
		StateRemoving, StateError:
		return NetworkConnectionState(s), nil
	default:
		return "", ErrInvalidConnectionState
	}
}

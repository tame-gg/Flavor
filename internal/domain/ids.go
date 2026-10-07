package domain

import (
	"crypto/rand"
	"errors"
	"fmt"
	"strings"
	"time"

	"github.com/oklog/ulid/v2"
)

type NetworkID string
type NodeID string
type ControlPlaneID string

var (
	ErrInvalidNetworkID = errors.New("invalid network id")
	ErrInvalidNodeID    = errors.New("invalid node id")
)

func NewNetworkID() NetworkID {
	entropy := ulid.Monotonic(rand.Reader, 0)
	return NetworkID(ulid.MustNew(ulid.Timestamp(time.Now().UTC()), entropy).String())
}

func ParseNetworkID(s string) (NetworkID, error) {
	if s == "" || s == "." || s == ".." {
		return "", ErrInvalidNetworkID
	}
	if len(s) > 64 {
		return "", ErrInvalidNetworkID
	}
	if strings.ContainsAny(s, "/\\ \t\n\r\x00") {
		return "", ErrInvalidNetworkID
	}
	for _, r := range s {
		if r < 33 || r > 126 {
			return "", ErrInvalidNetworkID
		}
		switch {
		case r >= '0' && r <= '9':
		case r >= 'A' && r <= 'Z':
		case r >= 'a' && r <= 'z':
		case r == '-' || r == '_':
		default:
			return "", ErrInvalidNetworkID
		}
	}
	return NetworkID(s), nil
}

func ParseNodeID(s string) (NodeID, error) {
	if s == "" || strings.ContainsAny(s, "\x00") {
		return "", ErrInvalidNodeID
	}
	if len(s) > 256 {
		return "", ErrInvalidNodeID
	}
	return NodeID(s), nil
}

func ControlPlaneIDFor(provider ProviderType, controlURL string) ControlPlaneID {
	switch provider {
	case ProviderTailscale:
		return ControlPlaneID("tailscale:default")
	case ProviderHeadscale:
		return ControlPlaneID(fmt.Sprintf("headscale:%s", controlURL))
	default:
		return ControlPlaneID(fmt.Sprintf("unknown:%s", controlURL))
	}
}

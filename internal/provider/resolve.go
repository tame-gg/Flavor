package provider

import (
	"fmt"
	"net/netip"
	"slices"

	"git.lunarlabs.dev/flavor/flavor/internal/domain"
)

type ResolvedSessionConfig struct {
	NetworkID    domain.NetworkID
	Provider     domain.ProviderType
	ControlURL   string
	NodeHostname string
	StateDir     string
	ControlPlane domain.ControlPlaneID

	ExitNode         domain.NodeID
	AdvertisedRoutes []netip.Prefix
}

func Resolve(n domain.Network, stateDir string) (ResolvedSessionConfig, error) {
	if err := n.Validate(); err != nil {
		return ResolvedSessionConfig{}, err
	}
	if stateDir == "" {
		return ResolvedSessionConfig{}, fmt.Errorf("state directory required")
	}
	control, err := domain.NormalizeControlURL(n.Provider, n.ControlURL)
	if err != nil {
		return ResolvedSessionConfig{}, err
	}
	cfg := ResolvedSessionConfig{
		NetworkID:    n.ID,
		Provider:     n.Provider,
		ControlURL:   control,
		NodeHostname: n.NodeHostname,
		StateDir:     stateDir,
		ControlPlane: domain.ControlPlaneIDFor(n.Provider, control),

		ExitNode:         n.ExitNode,
		AdvertisedRoutes: slices.Clone(n.AdvertisedRoutes),
	}
	return cfg, nil
}

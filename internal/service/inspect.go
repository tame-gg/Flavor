package service

import (
	"context"
	"errors"

	"git.lunarlabs.dev/flavor/flavor/internal/domain"
	"git.lunarlabs.dev/flavor/flavor/internal/inspect"
	"git.lunarlabs.dev/flavor/flavor/internal/naming"
	"git.lunarlabs.dev/flavor/flavor/internal/store"
)

func (s *Service) Inspect(ctx context.Context, destination string) (inspect.Result, uint64, error) {
	q, err := parseDestination(destination)
	if err != nil {
		return inspect.Result{}, 0, err
	}
	nets, seq, err := s.liveNetworks(ctx)
	if err != nil {
		return inspect.Result{}, 0, err
	}
	var pref *domain.DestinationPreference
	if p, err := s.cfg.Store.Preferences().Get(ctx, q.Normalized()); err == nil {
		pref = &p
	} else if !errors.Is(err, store.ErrNotFound) {
		return inspect.Result{}, 0, s.storeErr(err)
	}
	return inspect.Resolve(q, nets, pref), seq, nil
}

func (s *Service) Conflicts(ctx context.Context) (inspect.ConflictReport, uint64, error) {
	nets, seq, err := s.liveNetworks(ctx)
	if err != nil {
		return inspect.ConflictReport{}, 0, err
	}
	prefs, err := s.cfg.Store.Preferences().List(ctx)
	if err != nil {
		return inspect.ConflictReport{}, 0, s.storeErr(err)
	}
	return inspect.Conflicts(nets, prefs), seq, nil
}

func (s *Service) liveNetworks(ctx context.Context) ([]inspect.Network, uint64, error) {
	seq := s.cfg.Bus.Sequence()
	all, err := s.cfg.Store.Networks().List(ctx)
	if err != nil {
		return nil, 0, s.storeErr(err)
	}
	nets := make([]inspect.Network, 0, len(all))
	for _, n := range all {
		v := s.view(n)
		live := v.State == domain.StateConnected || v.State == domain.StateDegraded || v.State == domain.StateReconnecting
		in := inspect.Network{Network: n, State: v.State, Live: live}
		if live {
			in.Devices = s.devices(n.ID)
			in.Records = s.dnsRecords(n.ID)
		}
		nets = append(nets, in)
	}
	return nets, seq, nil
}

const CodeDeviceNotFound Code = "DEVICE_NOT_FOUND"

func (s *Service) DescribeDevice(ctx context.Context, rawNetwork, rawNode string) (name, stable string, err error) {
	id, err := parseID(rawNetwork)
	if err != nil {
		return "", "", err
	}
	nets, _, err := s.liveNetworks(ctx)
	if err != nil {
		return "", "", err
	}
	all := make([]domain.Network, 0, len(nets))
	for _, n := range nets {
		all = append(all, n.Network)
	}
	netLabels := naming.NetworkLabels(all)
	for _, n := range nets {
		if n.Network.ID != id {
			continue
		}
		devLabels := naming.DeviceLabels(n.Devices)
		if l, ok := devLabels[domain.NodeID(rawNode)]; ok {
			nl := netLabels[id]
			return naming.Name(l.Published(), nl.Published()), naming.Name(l.Stable, nl.Stable), nil
		}
	}
	return "", "", fail(CodeDeviceNotFound, "device not found on a connected network", false)
}

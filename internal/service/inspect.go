package service

import (
	"context"

	"git.lunarlabs.dev/lattice/lattice/internal/domain"
	"git.lunarlabs.dev/lattice/lattice/internal/inspect"
)

func (s *Service) Inspect(ctx context.Context, destination string) (inspect.Result, uint64, error) {
	q, err := inspect.ParseQuery(destination)
	if err != nil {
		return inspect.Result{}, 0, fail(CodeInvalidArgument, "enter an IP address or a device name, optionally with a port", false)
	}
	seq := s.cfg.Bus.Sequence()
	all, err := s.cfg.Store.Networks().List(ctx)
	if err != nil {
		return inspect.Result{}, 0, s.storeErr(err)
	}
	nets := make([]inspect.Network, 0, len(all))
	for _, n := range all {
		v := s.view(n)
		live := v.State == domain.StateConnected || v.State == domain.StateDegraded || v.State == domain.StateReconnecting
		in := inspect.Network{Network: n, State: v.State, Live: live}
		if live {
			in.Devices = s.devices(n.ID)
		}
		nets = append(nets, in)
	}
	return inspect.Resolve(q, nets), seq, nil
}

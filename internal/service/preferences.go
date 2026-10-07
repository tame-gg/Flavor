package service

import (
	"context"
	"errors"

	"git.lunarlabs.dev/lattice/lattice/internal/domain"
	"git.lunarlabs.dev/lattice/lattice/internal/events"
	"git.lunarlabs.dev/lattice/lattice/internal/inspect"
	"git.lunarlabs.dev/lattice/lattice/internal/store"
)

func (s *Service) ListPreferences(ctx context.Context) ([]domain.DestinationPreference, uint64, error) {
	seq := s.cfg.Bus.Sequence()
	all, err := s.cfg.Store.Preferences().List(ctx)
	if err != nil {
		return nil, 0, s.storeErr(err)
	}
	return all, seq, nil
}

func (s *Service) SetPreference(ctx context.Context, destination, rawNetwork string) (domain.DestinationPreference, error) {
	if err := s.checkRunning(); err != nil {
		return domain.DestinationPreference{}, err
	}
	q, err := parseDestination(destination)
	if err != nil {
		return domain.DestinationPreference{}, err
	}
	id, err := parseID(rawNetwork)
	if err != nil {
		return domain.DestinationPreference{}, err
	}
	if _, err := s.cfg.Store.Networks().Get(ctx, id); err != nil {
		return domain.DestinationPreference{}, s.storeErr(err)
	}
	p := domain.DestinationPreference{Destination: q.Normalized(), Kind: q.DestinationKind(), NetworkID: id}
	if err := s.cfg.Store.Preferences().Set(ctx, p); err != nil {
		return domain.DestinationPreference{}, s.storeErr(err)
	}
	saved, err := s.cfg.Store.Preferences().Get(ctx, p.Destination)
	if err != nil {
		return domain.DestinationPreference{}, s.storeErr(err)
	}
	s.publish(events.DestinationPreferenceChanged{Preference: saved})
	return saved, nil
}

func (s *Service) DeletePreference(ctx context.Context, destination string) error {
	if err := s.checkRunning(); err != nil {
		return err
	}
	q, err := parseDestination(destination)
	if err != nil {
		return err
	}
	removed, err := s.cfg.Store.Preferences().Delete(ctx, q.Normalized())
	if err != nil {
		return s.storeErr(err)
	}
	if removed {
		s.publish(events.DestinationPreferenceRemoved{Destination: q.Normalized()})
	}
	return nil
}

func (s *Service) preferencesFor(ctx context.Context, id domain.NetworkID) []domain.DestinationPreference {
	prefs, err := s.cfg.Store.Preferences().ForNetwork(ctx, id)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		s.cfg.Log.Error("listing preferences for removed network failed", "network_id", id, "err", err.Error())
	}
	return prefs
}

func (s *Service) announcePreferencesGone(prefs []domain.DestinationPreference) {
	for _, p := range prefs {
		s.publish(events.DestinationPreferenceRemoved{Destination: p.Destination})
	}
}

func parseDestination(raw string) (inspect.Query, error) {
	q, err := inspect.ParseQuery(raw)
	if err != nil {
		return inspect.Query{}, fail(CodeInvalidArgument, "enter an IP address or a device name, optionally with a port", false)
	}
	return q, nil
}

package service

import (
	"context"
	"fmt"
	"sort"
	"time"

	"git.lunarlabs.dev/flavor/flavor/internal/domain"
	"git.lunarlabs.dev/flavor/flavor/internal/logging"
)

type Snapshot struct {
	Sequence        uint64
	Info            DaemonInfo
	Networks        []NetworkView
	Devices         []domain.Device
	Workspaces      []domain.Workspace
	ActiveWorkspace domain.WorkspaceID
	Preferences     []domain.DestinationPreference
	CapturedAt      time.Time
}

func (s *Service) Snapshot(ctx context.Context) (Snapshot, error) {
	seq := s.cfg.Bus.Sequence()
	all, err := s.cfg.Store.Networks().List(ctx)
	if err != nil {
		return Snapshot{}, s.storeErr(err)
	}
	snap := Snapshot{Sequence: seq, Info: s.Info(), CapturedAt: time.Now().UTC()}
	for _, n := range all {
		snap.Networks = append(snap.Networks, s.view(n))
		snap.Devices = append(snap.Devices, s.devices(n.ID)...)
	}
	if snap.Workspaces, err = s.cfg.Store.Workspaces().List(ctx); err != nil {
		return Snapshot{}, s.storeErr(err)
	}
	if snap.ActiveWorkspace, err = s.cfg.Store.Workspaces().Active(ctx); err != nil {
		return Snapshot{}, s.storeErr(err)
	}
	if snap.Preferences, err = s.cfg.Store.Preferences().List(ctx); err != nil {
		return Snapshot{}, s.storeErr(err)
	}
	return snap, nil
}

func (s *Service) devices(id domain.NetworkID) []domain.Device {
	sess, ok := s.cfg.Sessions.Get(id)
	if !ok {
		return nil
	}
	out := sess.Devices()
	sort.Slice(out, func(i, j int) bool { return out[i].ID.NodeID < out[j].ID.NodeID })
	return out
}

func (s *Service) ListDevices(ctx context.Context, rawID string) ([]domain.Device, uint64, error) {
	seq := s.cfg.Bus.Sequence()
	if rawID == "" {
		all, err := s.cfg.Store.Networks().List(ctx)
		if err != nil {
			return nil, 0, s.storeErr(err)
		}
		var out []domain.Device
		for _, n := range all {
			out = append(out, s.devices(n.ID)...)
		}
		return out, seq, nil
	}
	id, err := parseID(rawID)
	if err != nil {
		return nil, 0, err
	}
	if _, err := s.cfg.Store.Networks().Get(ctx, id); err != nil {
		return nil, 0, s.storeErr(err)
	}
	return s.devices(id), seq, nil
}

type Check struct {
	Name   string
	Status string
	Detail string
}

type Diagnostics struct {
	Checks        []Check
	SecretBackend string
	SecretState   string
}

func (s *Service) Diagnostics(ctx context.Context) Diagnostics {
	info := s.Info()
	d := Diagnostics{Checks: []Check{{
		Name:   "daemon",
		Status: "ok",
		Detail: fmt.Sprintf("version %s, protocol %d.%d", info.DaemonVersion, info.ProtocolMajor, info.ProtocolMinor),
	}}}

	if v, err := s.cfg.Store.SchemaVersion(ctx); err != nil {
		d.Checks = append(d.Checks, Check{Name: "database", Status: "error", Detail: "database is not readable"})
	} else {
		d.Checks = append(d.Checks, Check{Name: "database", Status: "ok", Detail: fmt.Sprintf("schema version %d", v)})
	}

	st := s.cfg.Secrets.Status(ctx)
	d.SecretBackend, d.SecretState = string(st.Backend), string(st.State)
	secretStatus := "ok"
	if !st.Persistent() {
		secretStatus = "warning"
	}
	d.Checks = append(d.Checks, Check{
		Name:   "secret_store",
		Status: secretStatus,
		Detail: fmt.Sprintf("backend %s, state %s", st.Backend, st.State),
	})

	all, err := s.cfg.Store.Networks().List(ctx)
	if err != nil {
		return d
	}
	for _, n := range all {
		v := s.view(n)
		control := "Tailscale"
		if n.ControlURL != "" {
			control = logging.SanitizeURL(n.ControlURL)
		}
		detail := fmt.Sprintf("%s, control %s, state %s", n.Provider, control, v.State)
		if sess, ok := s.cfg.Sessions.Get(n.ID); ok {
			sd := sess.Diagnostics(ctx)
			detail += fmt.Sprintf(", %d devices, %d local addresses", sd.DeviceCount, sd.LocalAddrCount)
		}
		status := "ok"
		if v.State == domain.StateError {
			status = "error"
		}
		d.Checks = append(d.Checks, Check{Name: "network/" + string(n.ID), Status: status, Detail: detail})
	}
	return d
}

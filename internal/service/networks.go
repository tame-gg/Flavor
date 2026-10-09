package service

import (
	"context"
	"errors"
	"strings"
	"unicode"

	"git.lunarlabs.dev/flavor/flavor/internal/domain"
	"git.lunarlabs.dev/flavor/flavor/internal/events"
	"git.lunarlabs.dev/flavor/flavor/internal/provider"
	"git.lunarlabs.dev/flavor/flavor/internal/secret"
	"git.lunarlabs.dev/flavor/flavor/internal/session"
	"git.lunarlabs.dev/flavor/flavor/internal/store"
)

type NetworkView struct {
	Network domain.Network
	State   domain.NetworkConnectionState
	Prompt  *domain.AuthPrompt
}

type AddInput struct {
	DisplayName string
	Provider    domain.ProviderType
	ControlURL  string
	AutoConnect bool
}

func (s *Service) view(n domain.Network) NetworkView {
	v := NetworkView{Network: n, State: domain.StateDisconnected}
	if sess, ok := s.cfg.Sessions.Get(n.ID); ok {
		v.State = sess.State()
		v.Prompt = sess.AuthPrompt()
	}
	if op := s.pendingOp(n.ID); op == opRemove || op == opDelete {
		v.State = domain.StateRemoving
	}
	return v
}

const (
	opConnect    = "connect"
	opDisconnect = "disconnect"
	opUpdate     = "update"
	opRemove     = "remove"
	opDelete     = "delete"
)

func (s *Service) ListNetworks(ctx context.Context) ([]NetworkView, uint64, error) {
	seq := s.cfg.Bus.Sequence()
	all, err := s.cfg.Store.Networks().List(ctx)
	if err != nil {
		return nil, 0, s.storeErr(err)
	}
	out := make([]NetworkView, 0, len(all))
	for _, n := range all {
		out = append(out, s.view(n))
	}
	return out, seq, nil
}

func (s *Service) GetNetwork(ctx context.Context, rawID string) (NetworkView, uint64, error) {
	id, err := parseID(rawID)
	if err != nil {
		return NetworkView{}, 0, err
	}
	seq := s.cfg.Bus.Sequence()
	n, err := s.cfg.Store.Networks().Get(ctx, id)
	if err != nil {
		return NetworkView{}, 0, s.storeErr(err)
	}
	return s.view(n), seq, nil
}

func (s *Service) AddNetwork(ctx context.Context, in AddInput) (NetworkView, error) {
	if err := s.checkRunning(); err != nil {
		return NetworkView{}, err
	}
	name := strings.TrimSpace(in.DisplayName)
	if domain.ValidateDisplayName(name) != nil {
		return NetworkView{}, fail(CodeInvalidArgument, "display name must be 1-128 printable characters", false)
	}
	if _, err := domain.ParseProvider(string(in.Provider)); err != nil {
		return NetworkView{}, fail(CodeInvalidArgument, "unsupported provider", false)
	}
	controlURL, err := domain.NormalizeControlURL(in.Provider, in.ControlURL)
	if err != nil {
		return NetworkView{}, fail(CodeInvalidControlURL, "control server URL must be an http(s) URL without credentials", false)
	}
	n := domain.Network{
		ID:           domain.NewNetworkID(),
		DisplayName:  name,
		Provider:     in.Provider,
		ControlURL:   controlURL,
		AutoConnect:  in.AutoConnect,
		NodeHostname: s.cfg.NodeHostname,
	}
	if err := s.cfg.Store.Networks().Create(ctx, n); err != nil {
		return NetworkView{}, s.storeErr(err)
	}
	created, err := s.cfg.Store.Networks().Get(ctx, n.ID)
	if err != nil {
		return NetworkView{}, s.storeErr(err)
	}
	s.publish(events.NetworkAdded{Network: created, State: domain.StateDisconnected})
	return s.view(created), nil
}

func (s *Service) UpdateNetwork(ctx context.Context, rawID string, displayName *string, autoConnect *bool) (NetworkView, error) {
	id, err := parseID(rawID)
	if err != nil {
		return NetworkView{}, err
	}
	release, err := s.acquire(id, opUpdate)
	if err != nil {
		return NetworkView{}, err
	}
	defer release()
	n, err := s.cfg.Store.Networks().Get(ctx, id)
	if err != nil {
		return NetworkView{}, s.storeErr(err)
	}
	if displayName != nil {
		name := strings.TrimSpace(*displayName)
		if domain.ValidateDisplayName(name) != nil {
			return NetworkView{}, fail(CodeInvalidArgument, "display name must be 1-128 printable characters", false)
		}
		n.DisplayName = name
	}
	if autoConnect != nil {
		n.AutoConnect = *autoConnect
	}
	if err := s.cfg.Store.Networks().Update(ctx, n); err != nil {
		return NetworkView{}, s.storeErr(err)
	}
	updated, err := s.cfg.Store.Networks().Get(ctx, id)
	if err != nil {
		return NetworkView{}, s.storeErr(err)
	}
	s.publish(events.NetworkUpdated{Network: updated})
	return s.view(updated), nil
}

func (s *Service) ConnectNetwork(ctx context.Context, rawID string) error {
	return s.begin(ctx, rawID, nil)
}

func (s *Service) EnrollNetwork(ctx context.Context, rawID string, preAuthKey secret.Secret) error {
	if !validPreAuthKey(preAuthKey.Reveal()) {
		return fail(CodeInvalidArgument, "pre-auth key is empty or malformed", false)
	}
	return s.begin(ctx, rawID, &session.EnrollmentInput{Method: session.EnrollmentAuthKey, Credential: preAuthKey})
}

func validPreAuthKey(k string) bool {
	if k == "" || len(k) > 512 {
		return false
	}
	for _, r := range k {
		if r > unicode.MaxASCII || unicode.IsSpace(r) || unicode.IsControl(r) {
			return false
		}
	}
	return true
}

func (s *Service) begin(ctx context.Context, rawID string, enrollment *session.EnrollmentInput) error {
	id, err := parseID(rawID)
	if err != nil {
		return err
	}
	release, err := s.acquire(id, opConnect)
	if err != nil {
		return err
	}
	defer release()
	n, err := s.cfg.Store.Networks().Get(ctx, id)
	if err != nil {
		return s.storeErr(err)
	}
	s.cfg.Log.Info("connect requested", "network_id", id, "enroll", enrollment != nil)
	sess, err := s.session(n)
	if err != nil {
		return err
	}
	if enrollment != nil {
		if err := s.stop(ctx, sess); err != nil {
			return err
		}
	}
	switch err := sess.Begin(ctx, enrollment); {
	case err == nil:
		return nil
	case errors.Is(err, session.ErrAlreadyActive):
		return fail(CodeAlreadyConnected, "network is already connected or connecting", false)
	case errors.Is(err, session.ErrBusy):
		return fail(CodeBusy, "network is starting", true)
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return fail(CodeBusy, "previous session is still shutting down", true)
	default:
		s.cfg.Log.Error("session begin failed", "network_id", id, "err", err.Error())
		return fail(CodeInternal, "could not start network session", true)
	}
}

func (s *Service) session(n domain.Network) (*session.Session, error) {
	if _, err := s.cfg.Paths.EnsureNetworkDirs(n.ID); err != nil {
		s.cfg.Log.Error("network state directory unavailable", "network_id", n.ID, "err", err.Error())
		return nil, fail(CodeStateDirectory, "network state directory is unavailable", false)
	}
	dir, err := s.cfg.Paths.TsnetDir(n.ID)
	if err != nil {
		return nil, fail(CodeStateDirectory, "network state directory is unavailable", false)
	}
	cfg, err := provider.Resolve(n, dir)
	if err != nil {
		return nil, fail(CodeInvalidArgument, "stored network configuration is invalid", false)
	}
	sess, err := s.cfg.Sessions.GetOrCreate(cfg)
	if err != nil {
		return nil, fail(CodeInternal, "could not create network session", false)
	}
	return sess, nil
}

func (s *Service) stop(ctx context.Context, sess *session.Session) error {
	ctx, cancel := context.WithTimeout(ctx, s.cfg.StopTimeout)
	defer cancel()
	if err := sess.Stop(ctx); err != nil {
		return fail(CodeBusy, "network is still shutting down; try again", true)
	}
	return nil
}

func (s *Service) DisconnectNetwork(ctx context.Context, rawID string) error {
	id, err := parseID(rawID)
	if err != nil {
		return err
	}
	release, err := s.acquire(id, opDisconnect)
	if err != nil {
		return err
	}
	defer release()
	if _, err := s.cfg.Store.Networks().Get(ctx, id); err != nil {
		return s.storeErr(err)
	}
	s.cfg.Log.Info("disconnect requested", "network_id", id)
	sess, ok := s.cfg.Sessions.Get(id)
	if !ok {
		return nil
	}
	return s.stop(ctx, sess)
}

func (s *Service) RemoveNetwork(ctx context.Context, rawID string) error {
	id, err := parseID(rawID)
	if err != nil {
		return err
	}
	release, err := s.acquire(id, opRemove)
	if err != nil {
		return err
	}
	defer release()
	if _, err := s.cfg.Store.Networks().Get(ctx, id); err != nil {
		return s.storeErr(err)
	}
	if err := s.retireSession(ctx, id); err != nil {
		return err
	}
	affected := s.workspacesContaining(ctx, id)
	prefs := s.preferencesFor(ctx, id)
	if err := s.cfg.Store.SoftRemove(ctx, id); err != nil {
		return s.storeErr(err)
	}
	s.publish(events.NetworkRemoved{NetworkID: id})
	s.reannounceWorkspaces(ctx, affected)
	s.announcePreferencesGone(prefs)
	return nil
}

func (s *Service) DeleteNetworkIdentity(ctx context.Context, rawID string) error {
	id, err := parseID(rawID)
	if err != nil {
		return err
	}
	release, err := s.acquire(id, opDelete)
	if err != nil {
		return err
	}
	defer release()
	_, err = s.cfg.Store.Networks().Get(ctx, id)
	configured := err == nil
	switch {
	case configured:
		if err := s.retireSession(ctx, id); err != nil {
			return err
		}
	case errors.Is(err, store.ErrNotFound):
		if _, err := s.cfg.Store.Retained().Get(ctx, id); err != nil {
			return s.storeErr(err)
		}
	default:
		return s.storeErr(err)
	}
	affected := s.workspacesContaining(ctx, id)
	prefs := s.preferencesFor(ctx, id)
	err = s.cfg.Store.HardDeleteIdentity(ctx, id, store.PathResolver{Root: s.cfg.Paths.NetworksRoot})
	if err != nil && !errors.Is(err, store.ErrIdentityDeletePending) {
		s.cfg.Log.Error("identity delete failed", "network_id", id, "err", err.Error())
		return fail(CodeStateDirectory, "local identity could not be deleted; nothing was removed", true)
	}
	if configured {
		s.publish(events.NetworkRemoved{NetworkID: id})
		s.reannounceWorkspaces(ctx, affected)
		s.announcePreferencesGone(prefs)
	}
	if err != nil {
		s.cfg.Log.Error("identity directory removal pending", "network_id", id, "err", err.Error())
		return fail(CodeStateDirectory, "network removed; leftover identity files will be deleted on next daemon start", false)
	}
	return nil
}

func (s *Service) retireSession(ctx context.Context, id domain.NetworkID) error {
	if sess, ok := s.cfg.Sessions.Get(id); ok {
		if err := s.stop(ctx, sess); err != nil {
			return err
		}
	}
	if err := s.cfg.Sessions.Remove(id); err != nil {
		return fail(CodeBusy, "network is still shutting down; try again", true)
	}
	s.publish(events.NetworkStateChanged{NetworkID: id, State: domain.StateRemoving})
	return nil
}

func (s *Service) ReconcileAutoConnect(ctx context.Context) {
	all, err := s.cfg.Store.Networks().List(ctx)
	if err != nil {
		s.cfg.Log.Error("auto-connect reconcile: list networks failed", "err", err.Error())
		return
	}
	for _, n := range all {
		if !n.AutoConnect {
			continue
		}
		if err := s.ConnectNetwork(ctx, string(n.ID)); err != nil {
			s.cfg.Log.Info("auto-connect skipped", "network_id", n.ID, "err", err.Error())
		}
	}
}

func (s *Service) publish(p events.Payload) {
	_, _ = s.cfg.Bus.Publish(p)
}

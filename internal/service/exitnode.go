package service

import (
	"context"
	"errors"

	"git.lunarlabs.dev/flavor/flavor/internal/domain"
	"git.lunarlabs.dev/flavor/flavor/internal/session"
)

const (
	CodeNotAnExitNode Code = "NOT_AN_EXIT_NODE"

	opExitNode = "exit_node"
)

func (s *Service) SetExitNode(ctx context.Context, rawNetwork, rawNode string) (domain.Device, error) {
	id, err := parseID(rawNetwork)
	if err != nil {
		return domain.Device{}, err
	}
	if rawNode == "" {
		return domain.Device{}, fail(CodeInvalidArgument, "choose a device to use as the exit node", false)
	}
	release, err := s.acquire(id, opExitNode)
	if err != nil {
		return domain.Device{}, err
	}
	defer release()
	sess, err := s.connectedSession(ctx, id)
	if err != nil {
		return domain.Device{}, err
	}
	node := domain.NodeID(rawNode)
	var target domain.Device
	for _, d := range sess.Devices() {
		if d.ID.NodeID == node {
			target = d
		}
	}
	switch {
	case target.ID.NodeID == "":
		return domain.Device{}, fail(CodeDeviceNotFound, "device not found on this network", false)
	case !target.ExitNodeOption:
		return domain.Device{}, fail(CodeNotAnExitNode, "this device does not offer to be an exit node", false)
	}
	if sess.LocalNode().ExitNode.Offered {
		return domain.Device{}, fail(CodeInvalidArgument, "stop offering this machine as an exit node first", false)
	}
	if err := s.applyExitNode(ctx, sess, node); err != nil {
		return domain.Device{}, err
	}
	for _, d := range sess.Devices() {
		if d.ID.NodeID == node {
			return d, nil
		}
	}
	return target, nil
}

func (s *Service) ClearExitNode(ctx context.Context, rawNetwork string) error {
	id, err := parseID(rawNetwork)
	if err != nil {
		return err
	}
	release, err := s.acquire(id, opExitNode)
	if err != nil {
		return err
	}
	defer release()
	sess, err := s.connectedSession(ctx, id)
	if err != nil {
		return err
	}
	return s.applyExitNode(ctx, sess, "")
}

func (s *Service) connectedSession(ctx context.Context, id domain.NetworkID) (*session.Session, error) {
	if _, err := s.cfg.Store.Networks().Get(ctx, id); err != nil {
		return nil, s.storeErr(err)
	}
	sess, ok := s.cfg.Sessions.Get(id)
	if !ok {
		return nil, fail(CodeDestinationUnreachable, "network is not connected", false)
	}
	switch sess.State() {
	case domain.StateConnected, domain.StateDegraded, domain.StateReconnecting:
		return sess, nil
	}
	return nil, fail(CodeDestinationUnreachable, "network is not connected", false)
}

func (s *Service) applyExitNode(ctx context.Context, sess *session.Session, node domain.NodeID) error {
	return s.sessionError(sess, sess.SetExitNode(ctx, node), "setting exit node failed", "could not change the exit node")
}

func (s *Service) sessionError(sess *session.Session, err error, logMsg, safeMsg string) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, session.ErrNotRunning):
		return fail(CodeDestinationUnreachable, "network is not connected", false)
	case errors.Is(err, context.Canceled), errors.Is(err, context.DeadlineExceeded):
		return fail(CodeBusy, "operation did not complete in time", true)
	}
	s.cfg.Log.Error(logMsg, "network_id", sess.ID(), "err", err.Error())
	return fail(CodeInternal, safeMsg, true)
}

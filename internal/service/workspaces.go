package service

import (
	"context"
	"errors"

	"git.lunarlabs.dev/lattice/lattice/internal/domain"
	"git.lunarlabs.dev/lattice/lattice/internal/events"
	"git.lunarlabs.dev/lattice/lattice/internal/store"
)

const CodeWorkspaceNotFound Code = "WORKSPACE_NOT_FOUND"

type ActivationOutcome int

const (
	OutcomeConnecting ActivationOutcome = iota + 1
	OutcomeAlreadyActive
	OutcomeDisconnected
	OutcomeFailed
)

type ActivationResult struct {
	Network     domain.Network
	State       domain.NetworkConnectionState
	Outcome     ActivationOutcome
	SafeMessage string
}

type WorkspaceInput struct {
	Name        *string
	Description *string
	NetworkIDs  *[]string
}

func (s *Service) ListWorkspaces(ctx context.Context) ([]domain.Workspace, domain.WorkspaceID, uint64, error) {
	seq := s.cfg.Bus.Sequence()
	all, err := s.cfg.Store.Workspaces().List(ctx)
	if err != nil {
		return nil, "", 0, s.storeErr(err)
	}
	active, err := s.cfg.Store.Workspaces().Active(ctx)
	if err != nil {
		return nil, "", 0, s.storeErr(err)
	}
	return all, active, seq, nil
}

func (s *Service) CreateWorkspace(ctx context.Context, in WorkspaceInput) (domain.Workspace, error) {
	if err := s.checkRunning(); err != nil {
		return domain.Workspace{}, err
	}
	w := domain.Workspace{ID: domain.NewWorkspaceID()}
	if err := s.applyWorkspaceInput(ctx, &w, in); err != nil {
		return domain.Workspace{}, err
	}
	if err := s.cfg.Store.Workspaces().Create(ctx, w); err != nil {
		return domain.Workspace{}, s.workspaceErr(err)
	}
	return s.announceWorkspace(ctx, w.ID)
}

func (s *Service) UpdateWorkspace(ctx context.Context, rawID string, in WorkspaceInput) (domain.Workspace, error) {
	if err := s.checkRunning(); err != nil {
		return domain.Workspace{}, err
	}
	w, err := s.workspace(ctx, rawID)
	if err != nil {
		return domain.Workspace{}, err
	}
	if err := s.applyWorkspaceInput(ctx, &w, in); err != nil {
		return domain.Workspace{}, err
	}
	if err := s.cfg.Store.Workspaces().Update(ctx, w); err != nil {
		return domain.Workspace{}, s.workspaceErr(err)
	}
	return s.announceWorkspace(ctx, w.ID)
}

func (s *Service) DeleteWorkspace(ctx context.Context, rawID string) error {
	if err := s.checkRunning(); err != nil {
		return err
	}
	w, err := s.workspace(ctx, rawID)
	if err != nil {
		return err
	}
	active, err := s.cfg.Store.Workspaces().Active(ctx)
	if err != nil {
		return s.storeErr(err)
	}
	if err := s.cfg.Store.Workspaces().Delete(ctx, w.ID); err != nil {
		return s.workspaceErr(err)
	}
	s.publish(events.WorkspaceRemoved{WorkspaceID: w.ID})
	if active == w.ID {
		s.publish(events.ActiveWorkspaceChanged{})
	}
	return nil
}

func (s *Service) ActivateWorkspace(ctx context.Context, rawID string, disconnectOthers bool) (domain.Workspace, []ActivationResult, error) {
	if err := s.checkRunning(); err != nil {
		return domain.Workspace{}, nil, err
	}
	w, err := s.workspace(ctx, rawID)
	if err != nil {
		return domain.Workspace{}, nil, err
	}
	if err := s.cfg.Store.Workspaces().SetActive(ctx, w.ID); err != nil {
		return domain.Workspace{}, nil, s.workspaceErr(err)
	}
	s.publish(events.ActiveWorkspaceChanged{WorkspaceID: w.ID})

	all, err := s.cfg.Store.Networks().List(ctx)
	if err != nil {
		return w, nil, s.storeErr(err)
	}
	member := make(map[domain.NetworkID]bool, len(w.NetworkIDs))
	for _, id := range w.NetworkIDs {
		member[id] = true
	}
	var results []ActivationResult
	for _, n := range all {
		if member[n.ID] {
			results = append(results, s.activateMember(ctx, n))
			continue
		}
		if !disconnectOthers || !isRunning(s.view(n).State) {
			continue
		}
		r := ActivationResult{Network: n, Outcome: OutcomeDisconnected}
		if err := s.DisconnectNetwork(ctx, string(n.ID)); err != nil {
			r.Outcome, r.SafeMessage = OutcomeFailed, safeMessage(err)
		}
		r.State = s.view(n).State
		results = append(results, r)
	}
	return w, results, nil
}

func (s *Service) activateMember(ctx context.Context, n domain.Network) ActivationResult {
	r := ActivationResult{Network: n, Outcome: OutcomeConnecting}
	err := s.ConnectNetwork(ctx, string(n.ID))
	var se *Error
	switch {
	case err == nil:
	case errors.As(err, &se) && se.Code == CodeAlreadyConnected:
		r.Outcome = OutcomeAlreadyActive
	default:
		r.Outcome, r.SafeMessage = OutcomeFailed, safeMessage(err)
	}
	r.State = s.view(n).State
	return r
}

func (s *Service) DeactivateWorkspace(ctx context.Context) error {
	if err := s.cfg.Store.Workspaces().SetActive(ctx, ""); err != nil {
		return s.storeErr(err)
	}
	s.publish(events.ActiveWorkspaceChanged{})
	return nil
}

func (s *Service) workspacesContaining(ctx context.Context, id domain.NetworkID) []domain.WorkspaceID {
	ids, err := s.cfg.Store.Workspaces().Containing(ctx, id)
	if err != nil {
		s.cfg.Log.Error("listing workspaces for removed network failed", "network_id", id, "err", err.Error())
	}
	return ids
}

func (s *Service) reannounceWorkspaces(ctx context.Context, ids []domain.WorkspaceID) {
	for _, id := range ids {
		_, _ = s.announceWorkspace(ctx, id)
	}
}

func (s *Service) announceWorkspace(ctx context.Context, id domain.WorkspaceID) (domain.Workspace, error) {
	w, err := s.cfg.Store.Workspaces().Get(ctx, id)
	if err != nil {
		return domain.Workspace{}, s.workspaceErr(err)
	}
	s.publish(events.WorkspaceChanged{Workspace: w})
	return w, nil
}

func (s *Service) workspace(ctx context.Context, rawID string) (domain.Workspace, error) {
	id, err := domain.ParseWorkspaceID(rawID)
	if err != nil {
		return domain.Workspace{}, fail(CodeInvalidArgument, "invalid workspace id", false)
	}
	w, err := s.cfg.Store.Workspaces().Get(ctx, id)
	if err != nil {
		return domain.Workspace{}, s.workspaceErr(err)
	}
	return w, nil
}

func (s *Service) applyWorkspaceInput(ctx context.Context, w *domain.Workspace, in WorkspaceInput) error {
	if in.Name != nil {
		w.Name = *in.Name
	}
	if in.Description != nil {
		w.Description = *in.Description
	}
	if domain.ValidateWorkspaceName(w.Name) != nil {
		return fail(CodeInvalidArgument, "workspace name must be 1-64 characters without leading or trailing spaces", false)
	}
	if domain.ValidateWorkspaceDescription(w.Description) != nil {
		return fail(CodeInvalidArgument, "workspace description must be at most 256 characters", false)
	}
	if in.NetworkIDs == nil {
		return nil
	}
	w.NetworkIDs = w.NetworkIDs[:0]
	for _, raw := range *in.NetworkIDs {
		id, err := parseID(raw)
		if err != nil {
			return err
		}
		if _, err := s.cfg.Store.Networks().Get(ctx, id); err != nil {
			if errors.Is(err, store.ErrNotFound) {
				return fail(CodeNetworkNotFound, "one of the selected networks no longer exists", false)
			}
			return s.storeErr(err)
		}
		w.NetworkIDs = append(w.NetworkIDs, id)
	}
	return nil
}

func (s *Service) workspaceErr(err error) error {
	if errors.Is(err, store.ErrNotFound) {
		return fail(CodeWorkspaceNotFound, "workspace not found", false)
	}
	return s.storeErr(err)
}

func isRunning(st domain.NetworkConnectionState) bool {
	switch st {
	case domain.StateDisconnected, domain.StateDisabled, domain.StateError, domain.StateRemoving, "":
		return false
	}
	return true
}

func safeMessage(err error) string {
	var se *Error
	if errors.As(err, &se) {
		return se.SafeMessage
	}
	return "unexpected error"
}

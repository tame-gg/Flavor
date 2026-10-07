package server

import (
	"context"

	"connectrpc.com/connect"
	v1 "git.lunarlabs.dev/lattice/lattice/gen/go/lattice/v1"
	"git.lunarlabs.dev/lattice/lattice/internal/domain"
	"git.lunarlabs.dev/lattice/lattice/internal/service"
)

var outcomes = map[service.ActivationOutcome]v1.ActivationOutcome{
	service.OutcomeConnecting:    v1.ActivationOutcome_ACTIVATION_OUTCOME_CONNECTING,
	service.OutcomeAlreadyActive: v1.ActivationOutcome_ACTIVATION_OUTCOME_ALREADY_ACTIVE,
	service.OutcomeDisconnected:  v1.ActivationOutcome_ACTIVATION_OUTCOME_DISCONNECTED,
	service.OutcomeFailed:        v1.ActivationOutcome_ACTIVATION_OUTCOME_FAILED,
}

func workspace(w domain.Workspace) *v1.Workspace {
	out := &v1.Workspace{
		Id:          string(w.ID),
		Name:        w.Name,
		Description: w.Description,
		CreatedAt:   timestamp(w.CreatedAt),
		UpdatedAt:   timestamp(w.UpdatedAt),
	}
	for _, id := range w.NetworkIDs {
		out.NetworkIds = append(out.NetworkIds, string(id))
	}
	return out
}

func (h *handlers) ListWorkspaces(ctx context.Context, _ *connect.Request[v1.ListWorkspacesRequest]) (*connect.Response[v1.ListWorkspacesResponse], error) {
	all, active, seq, err := h.svc.ListWorkspaces(ctx)
	if err != nil {
		return nil, toConnect(err)
	}
	out := &v1.ListWorkspacesResponse{DaemonInstanceId: h.svc.InstanceID(), SnapshotSequence: seq, ActiveWorkspaceId: string(active)}
	for _, w := range all {
		out.Workspaces = append(out.Workspaces, workspace(w))
	}
	return connect.NewResponse(out), nil
}

func (h *handlers) CreateWorkspace(ctx context.Context, req *connect.Request[v1.CreateWorkspaceRequest]) (*connect.Response[v1.CreateWorkspaceResponse], error) {
	ids := req.Msg.NetworkIds
	w, err := h.svc.CreateWorkspace(ctx, service.WorkspaceInput{Name: &req.Msg.Name, Description: &req.Msg.Description, NetworkIDs: &ids})
	if err != nil {
		return nil, toConnect(err)
	}
	return connect.NewResponse(&v1.CreateWorkspaceResponse{Workspace: workspace(w)}), nil
}

func (h *handlers) UpdateWorkspace(ctx context.Context, req *connect.Request[v1.UpdateWorkspaceRequest]) (*connect.Response[v1.UpdateWorkspaceResponse], error) {
	in := service.WorkspaceInput{Name: req.Msg.Name, Description: req.Msg.Description}
	if req.Msg.Networks != nil {
		ids := req.Msg.Networks.NetworkIds
		in.NetworkIDs = &ids
	}
	w, err := h.svc.UpdateWorkspace(ctx, req.Msg.WorkspaceId, in)
	if err != nil {
		return nil, toConnect(err)
	}
	return connect.NewResponse(&v1.UpdateWorkspaceResponse{Workspace: workspace(w)}), nil
}

func (h *handlers) DeleteWorkspace(ctx context.Context, req *connect.Request[v1.DeleteWorkspaceRequest]) (*connect.Response[v1.DeleteWorkspaceResponse], error) {
	if err := h.svc.DeleteWorkspace(ctx, req.Msg.WorkspaceId); err != nil {
		return nil, toConnect(err)
	}
	return connect.NewResponse(&v1.DeleteWorkspaceResponse{}), nil
}

func (h *handlers) ActivateWorkspace(ctx context.Context, req *connect.Request[v1.ActivateWorkspaceRequest]) (*connect.Response[v1.ActivateWorkspaceResponse], error) {
	w, results, err := h.svc.ActivateWorkspace(ctx, req.Msg.WorkspaceId, req.Msg.DisconnectOthers)
	if err != nil {
		return nil, toConnect(err)
	}
	out := &v1.ActivateWorkspaceResponse{Workspace: workspace(w)}
	for _, r := range results {
		out.Results = append(out.Results, &v1.ActivationResult{
			Network:     networkRef(r.Network, r.State),
			Outcome:     outcomes[r.Outcome],
			SafeMessage: r.SafeMessage,
		})
	}
	return connect.NewResponse(out), nil
}

func (h *handlers) DeactivateWorkspace(ctx context.Context, _ *connect.Request[v1.DeactivateWorkspaceRequest]) (*connect.Response[v1.DeactivateWorkspaceResponse], error) {
	if err := h.svc.DeactivateWorkspace(ctx); err != nil {
		return nil, toConnect(err)
	}
	return connect.NewResponse(&v1.DeactivateWorkspaceResponse{}), nil
}

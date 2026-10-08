package server

import (
	"context"

	"connectrpc.com/connect"
	v1 "git.lunarlabs.dev/flavor/flavor/gen/go/flavor/v1"
	"git.lunarlabs.dev/flavor/flavor/internal/domain"
	"git.lunarlabs.dev/flavor/flavor/internal/inspect"
)

var preferenceStates = map[inspect.PreferenceState]v1.PreferenceState{
	inspect.PreferenceApplied:             v1.PreferenceState_PREFERENCE_STATE_APPLIED,
	inspect.PreferenceNetworkNotConnected: v1.PreferenceState_PREFERENCE_STATE_NETWORK_NOT_CONNECTED,
	inspect.PreferenceNoMatchOnNetwork:    v1.PreferenceState_PREFERENCE_STATE_NO_MATCH_ON_NETWORK,
}

func preference(p domain.DestinationPreference) *v1.DestinationPreference {
	kind := v1.DestinationKind_DESTINATION_KIND_NAME
	if p.Kind == domain.DestinationAddress {
		kind = v1.DestinationKind_DESTINATION_KIND_ADDRESS
	}
	return &v1.DestinationPreference{
		Destination: p.Destination,
		Kind:        kind,
		NetworkId:   string(p.NetworkID),
		CreatedAt:   timestamp(p.CreatedAt),
		UpdatedAt:   timestamp(p.UpdatedAt),
	}
}

func (h *handlers) ListDestinationPreferences(ctx context.Context, _ *connect.Request[v1.ListDestinationPreferencesRequest]) (*connect.Response[v1.ListDestinationPreferencesResponse], error) {
	all, seq, err := h.svc.ListPreferences(ctx)
	if err != nil {
		return nil, toConnect(err)
	}
	out := &v1.ListDestinationPreferencesResponse{DaemonInstanceId: h.svc.InstanceID(), SnapshotSequence: seq}
	for _, p := range all {
		out.Preferences = append(out.Preferences, preference(p))
	}
	return connect.NewResponse(out), nil
}

func (h *handlers) SetDestinationPreference(ctx context.Context, req *connect.Request[v1.SetDestinationPreferenceRequest]) (*connect.Response[v1.SetDestinationPreferenceResponse], error) {
	p, err := h.svc.SetPreference(ctx, req.Msg.Destination, req.Msg.NetworkId)
	if err != nil {
		return nil, toConnect(err)
	}
	return connect.NewResponse(&v1.SetDestinationPreferenceResponse{Preference: preference(p)}), nil
}

func (h *handlers) DeleteDestinationPreference(ctx context.Context, req *connect.Request[v1.DeleteDestinationPreferenceRequest]) (*connect.Response[v1.DeleteDestinationPreferenceResponse], error) {
	if err := h.svc.DeletePreference(ctx, req.Msg.Destination); err != nil {
		return nil, toConnect(err)
	}
	return connect.NewResponse(&v1.DeleteDestinationPreferenceResponse{}), nil
}

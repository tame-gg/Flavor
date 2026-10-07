package server

import (
	"context"

	"connectrpc.com/connect"
	v1 "git.lunarlabs.dev/lattice/lattice/gen/go/lattice/v1"
	"git.lunarlabs.dev/lattice/lattice/internal/domain"
	"git.lunarlabs.dev/lattice/lattice/internal/inspect"
)

var (
	destinationKinds = map[inspect.QueryKind]v1.DestinationKind{
		inspect.KindAddress: v1.DestinationKind_DESTINATION_KIND_ADDRESS,
		inspect.KindName:    v1.DestinationKind_DESTINATION_KIND_NAME,
	}
	matchKinds = map[inspect.MatchKind]v1.MatchKind{
		inspect.MatchDeviceAddress:  v1.MatchKind_MATCH_KIND_DEVICE_ADDRESS,
		inspect.MatchDeviceDNSName:  v1.MatchKind_MATCH_KIND_DEVICE_DNS_NAME,
		inspect.MatchDeviceHostname: v1.MatchKind_MATCH_KIND_DEVICE_HOSTNAME,
		inspect.MatchSubnetRoute:    v1.MatchKind_MATCH_KIND_SUBNET_ROUTE,
		inspect.MatchQualifiedName:  v1.MatchKind_MATCH_KIND_QUALIFIED_NAME,
	}
	decisions = map[inspect.Decision]v1.ResolutionDecision{
		inspect.DecisionUnique:    v1.ResolutionDecision_RESOLUTION_DECISION_UNIQUE,
		inspect.DecisionAmbiguous: v1.ResolutionDecision_RESOLUTION_DECISION_AMBIGUOUS,
		inspect.DecisionNoMatch:   v1.ResolutionDecision_RESOLUTION_DECISION_NO_MATCH,
	}
	reasons = map[inspect.Reason]v1.DecisionReason{
		inspect.ReasonExactDeviceAddress:    v1.DecisionReason_DECISION_REASON_EXACT_DEVICE_ADDRESS,
		inspect.ReasonDeviceDNSName:         v1.DecisionReason_DECISION_REASON_DEVICE_DNS_NAME,
		inspect.ReasonDeviceHostname:        v1.DecisionReason_DECISION_REASON_DEVICE_HOSTNAME,
		inspect.ReasonMultipleMatches:       v1.DecisionReason_DECISION_REASON_MULTIPLE_MATCHES,
		inspect.ReasonNoMatch:               v1.DecisionReason_DECISION_REASON_NO_MATCH,
		inspect.ReasonDestinationPreference: v1.DecisionReason_DECISION_REASON_DESTINATION_PREFERENCE,
		inspect.ReasonSubnetRoute:           v1.DecisionReason_DECISION_REASON_SUBNET_ROUTE,
		inspect.ReasonLongestPrefix:         v1.DecisionReason_DECISION_REASON_LONGEST_PREFIX,
		inspect.ReasonNetworkQualifiedName:  v1.DecisionReason_DECISION_REASON_NETWORK_QUALIFIED_NAME,
		inspect.ReasonExplicitNetwork:       v1.DecisionReason_DECISION_REASON_EXPLICIT_NETWORK,
		inspect.ReasonAmbiguousNetworkLabel: v1.DecisionReason_DECISION_REASON_AMBIGUOUS_NETWORK_LABEL,
	}
	candidateStatuses = map[inspect.CandidateStatus]v1.CandidateStatus{
		inspect.StatusSelected:  v1.CandidateStatus_CANDIDATE_STATUS_SELECTED,
		inspect.StatusTied:      v1.CandidateStatus_CANDIDATE_STATUS_TIED,
		inspect.StatusOutranked: v1.CandidateStatus_CANDIDATE_STATUS_OUTRANKED,
	}
)

func (h *handlers) InspectDestination(ctx context.Context, req *connect.Request[v1.InspectDestinationRequest]) (*connect.Response[v1.InspectDestinationResponse], error) {
	res, seq, err := h.svc.Inspect(ctx, req.Msg.Destination)
	if err != nil {
		return nil, toConnect(err)
	}
	return connect.NewResponse(inspection(res, seq, h.svc.InstanceID())), nil
}

func inspection(res inspect.Result, seq uint64, instance string) *v1.InspectDestinationResponse {
	out := &v1.InspectDestinationResponse{
		DaemonInstanceId: instance,
		SnapshotSequence: seq,
		Query:            res.Query.Raw,
		Kind:             destinationKinds[res.Query.Kind],
		Normalized:       res.Query.Normalized(),
		Port:             uint32(res.Query.Port),
		Decision:         decisions[res.Decision],
		Reason:           reasons[res.Reason],
		DecidedBy:        matchKinds[res.DecidedBy],
	}
	for _, c := range res.Candidates {
		out.Candidates = append(out.Candidates, &v1.ResolutionCandidate{
			Network:       networkRef(c.Network, c.State),
			Device:        device(c.Device),
			Match:         matchKinds[c.Match],
			MatchedValue:  c.MatchedValue,
			Status:        candidateStatuses[c.Status],
			QualifiedName: c.Name,
			StableName:    c.StableName,
		})
	}
	for _, n := range res.NotInspected {
		out.NotInspected = append(out.NotInspected, networkRef(n.Network, n.State))
	}
	if p := res.Preference; p != nil {
		out.Preference = &v1.PreferenceUse{
			Destination: p.Preference.Destination,
			Network:     &v1.NetworkRef{Id: string(p.Preference.NetworkID), DisplayName: p.Network.DisplayName, Provider: providerToProto(p.Network.Provider)},
			State:       preferenceStates[p.State],
		}
	}
	return out
}

func networkRef(n domain.Network, state domain.NetworkConnectionState) *v1.NetworkRef {
	return &v1.NetworkRef{Id: string(n.ID), DisplayName: n.DisplayName, Provider: providerToProto(n.Provider), State: states[state]}
}

func (h *handlers) DescribeDevice(ctx context.Context, req *connect.Request[v1.DescribeDeviceRequest]) (*connect.Response[v1.DescribeDeviceResponse], error) {
	name, stable, err := h.svc.DescribeDevice(ctx, req.Msg.NetworkId, req.Msg.NodeId)
	if err != nil {
		return nil, toConnect(err)
	}
	return connect.NewResponse(&v1.DescribeDeviceResponse{Name: name, StableName: stable}), nil
}

package server

import (
	"context"

	"connectrpc.com/connect"
	v1 "git.lunarlabs.dev/lattice/lattice/gen/go/lattice/v1"
	"git.lunarlabs.dev/lattice/lattice/internal/inspect"
)

var (
	conflictTypes = map[inspect.ConflictType]v1.ConflictType{
		inspect.ConflictAddress:  v1.ConflictType_CONFLICT_TYPE_ADDRESS_COLLISION,
		inspect.ConflictDNSName:  v1.ConflictType_CONFLICT_TYPE_DNS_NAME_COLLISION,
		inspect.ConflictHostname: v1.ConflictType_CONFLICT_TYPE_HOSTNAME_COLLISION,
	}
	severities = map[inspect.Severity]v1.ConflictSeverity{
		inspect.SeverityAmbiguous: v1.ConflictSeverity_CONFLICT_SEVERITY_AMBIGUOUS,
		inspect.SeverityExpected:  v1.ConflictSeverity_CONFLICT_SEVERITY_EXPECTED,
	}
	scopes = map[inspect.Scope]v1.ConflictScope{
		inspect.ScopeCrossNetwork:  v1.ConflictScope_CONFLICT_SCOPE_CROSS_NETWORK,
		inspect.ScopeWithinNetwork: v1.ConflictScope_CONFLICT_SCOPE_WITHIN_NETWORK,
	}
)

func (h *handlers) ListConflicts(ctx context.Context, _ *connect.Request[v1.ListConflictsRequest]) (*connect.Response[v1.ListConflictsResponse], error) {
	rep, seq, err := h.svc.Conflicts(ctx)
	if err != nil {
		return nil, toConnect(err)
	}
	out := &v1.ListConflictsResponse{DaemonInstanceId: h.svc.InstanceID(), SnapshotSequence: seq}
	for _, c := range rep.Conflicts {
		pc := &v1.Conflict{
			Id:                     c.ID,
			Type:                   conflictTypes[c.Type],
			Severity:               severities[c.Severity],
			Scope:                  scopes[c.Scope],
			Value:                  c.Value,
			NetworkContextResolves: c.ContextResolves,
			PreferredNetworkId:     string(c.PreferredNetwork),
		}
		for _, m := range c.Members {
			pc.Members = append(pc.Members, &v1.ConflictMember{Network: networkRef(m.Network, m.State), Device: device(m.Device), UniqueName: m.UniqueName})
		}
		out.Conflicts = append(out.Conflicts, pc)
	}
	for _, n := range rep.NotInspected {
		out.NotInspected = append(out.NotInspected, networkRef(n.Network, n.State))
	}
	return connect.NewResponse(out), nil
}

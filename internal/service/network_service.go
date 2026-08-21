package service

import (
	"context"
	"fmt"

	"task141-firehydraulics/internal/hydraulics"
	"task141-firehydraulics/internal/idlib"
	"task141-firehydraulics/internal/model"
	"task141-firehydraulics/internal/store"
)

// --- Network (nodes + pipes) ---

// CreateNode inserts a node into a system's network. It detects cycles and
// dangling references lazily (at Calculate time, when the full tree is built);
// per-node creation only validates the system exists and the type/k-factor.
func (s *Services) CreateNode(ctx context.Context, systemID string, t model.NodeType, label string, elevMM, kFactor, minPressure int64, seq int64) (*model.Node, error) {
	sys, err := s.st.GetSystem(ctx, systemID)
	if err != nil {
		return nil, err
	}
	if sys.State != model.StateDraft && sys.State != model.StateDesigned {
		return nil, fmt.Errorf("%w: cannot edit network in state %s", store.ErrStateConflict, sys.State)
	}
	if (t == model.NodeSprinkler || t == model.NodeStandpipe) && kFactor <= 0 {
		return nil, fmt.Errorf("%w: emitter node requires k_factor > 0", store.ErrInvariant)
	}
	n := &model.Node{
		ID:                idlib.New("node"),
		SystemID:          systemID,
		Type:              t,
		Label:             label,
		ElevationMM:       elevMM,
		KFactor:           kFactor,
		DesignMinPressure: minPressure,
		Seq:               seq,
	}
	if err := s.st.CreateNode(ctx, nil, n); err != nil {
		return nil, err
	}
	return n, nil
}

// ListNodes returns the nodes of a system.
func (s *Services) ListNodes(ctx context.Context, systemID string) ([]model.Node, error) {
	return s.st.ListNodesBySystem(ctx, systemID)
}

// CreatePipe inserts a pipe segment. It validates that both endpoints exist and
// belong to the same system, and that adding the edge does not form a cycle
// (an edge that would make the downstream node have two parents is rejected).
func (s *Services) CreatePipe(ctx context.Context, systemID, upstreamID, downstreamID string, nominalDia, innerDia, length, cFactor, fittingEquiv, seq int64) (*model.PipeSegment, error) {
	sys, err := s.st.GetSystem(ctx, systemID)
	if err != nil {
		return nil, err
	}
	if sys.State != model.StateDraft && sys.State != model.StateDesigned {
		return nil, fmt.Errorf("%w: cannot edit network in state %s", store.ErrStateConflict, sys.State)
	}
	// Load existing pipes to check the downstream node doesn't already have a
	// parent (tree invariant: every non-source node has exactly one upstream).
	pipes, err := s.st.ListPipesBySystem(ctx, systemID)
	if err != nil {
		return nil, err
	}
	for _, p := range pipes {
		if p.DownstreamNodeID == downstreamID {
			return nil, fmt.Errorf("%w: downstream node %s already has an upstream pipe", store.ErrNetworkCycle, downstreamID)
		}
		if p.UpstreamNodeID == upstreamID && p.DownstreamNodeID == downstreamID {
			return nil, fmt.Errorf("%w: duplicate pipe %s→%s", store.ErrConflict, upstreamID, downstreamID)
		}
	}
	if innerDia <= 0 {
		innerDia = hydraulics.PipeInnerDiameter(nominalDia)
	}
	p := &model.PipeSegment{
		ID:               idlib.New("pipe"),
		SystemID:         systemID,
		UpstreamNodeID:   upstreamID,
		DownstreamNodeID: downstreamID,
		NominalDiaMM:     nominalDia,
		InnerDiaMM:       innerDia,
		LengthMM:         length,
		CFactor:           cFactor,
		FittingEquivMM:   fittingEquiv,
		Seq:              seq,
	}
	if err := s.st.CreatePipe(ctx, nil, p); err != nil {
		return nil, err
	}
	return p, nil
}

// ListPipes returns the pipes of a system.
func (s *Services) ListPipes(ctx context.Context, systemID string) ([]model.PipeSegment, error) {
	return s.st.ListPipesBySystem(ctx, systemID)
}

// SetRemoteNode links the most-unfavourable sprinkler to a system.
func (s *Services) SetRemoteNode(ctx context.Context, systemID, nodeID string) (*model.System, error) {
	nodes, err := s.st.ListNodesBySystem(ctx, systemID)
	if err != nil {
		return nil, err
	}
	found := false
	for _, n := range nodes {
		if n.ID == nodeID && (n.Type == model.NodeSprinkler || n.Type == model.NodeStandpipe) {
			found = true
			break
		}
	}
	if !found {
		return nil, fmt.Errorf("%w: remote node must be an existing sprinkler/standpipe in the system", store.ErrInvariant)
	}
	if err := s.st.SetSystemRemoteNode(ctx, nil, systemID, nodeID, s.now()); err != nil {
		return nil, err
	}
	return s.st.GetSystem(ctx, systemID)
}

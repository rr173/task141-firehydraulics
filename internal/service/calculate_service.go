package service

import (
	"context"
	"database/sql"
	"fmt"

	"task141-firehydraulics/internal/compliance"
	"task141-firehydraulics/internal/hydraulics"
	"task141-firehydraulics/internal/idlib"
	"task141-firehydraulics/internal/model"
	"task141-firehydraulics/internal/store"
	"task141-firehydraulics/internal/watersupply"
)

// --- Hydraulic calculation ---

// buildNetwork assembles the stored nodes + pipes into a hydraulics.Network
// rooted at the system's source node. It returns ErrNetworkCycle if the graph
// is not a connected tree.
func (s *Services) buildNetwork(ctx context.Context, systemID string) (nodes []model.Node, pipes []model.PipeSegment, sourceID string, net *hydraulics.Network, err error) {
	nodes, err = s.st.ListNodesBySystem(ctx, systemID)
	if err != nil {
		return nil, nil, "", nil, err
	}
	pipes, err = s.st.ListPipesBySystem(ctx, systemID)
	if err != nil {
		return nil, nil, "", nil, err
	}
	var src *model.Node
	for i := range nodes {
		if nodes[i].Type == model.NodeSource || nodes[i].Type == model.NodeBase {
			src = &nodes[i]
			break
		}
	}
	if src == nil {
		return nil, nil, "", nil, fmt.Errorf("%w: system %s has no source/base node", store.ErrInvariant, systemID)
	}
	sourceID = src.ID
	net, err = hydraulics.BuildNetwork(sourceID, nodes, pipes)
	if err != nil {
		return nil, nil, "", nil, fmt.Errorf("%w: %v", store.ErrNetworkCycle, err)
	}
	return nodes, pipes, sourceID, net, nil
}

// remoteStartPressure returns the minimum working pressure (mbar) to start the
// remote sprinkler at: the node's own design_min_pressure if set, else the
// hazard-class minimum (0.5 bar).
func (s *Services) remoteStartPressure(sys *model.System, nodes []model.Node) (string, int64, error) {
	for _, n := range nodes {
		if n.ID == sys.RemoteNodeID {
			p := n.DesignMinPressure
			if p <= 0 {
				p = 500
			}
			return n.ID, p, nil
		}
	}
	return "", 0, fmt.Errorf("%w: remote node %s not found in system", store.ErrInvariant, sys.RemoteNodeID)
}

// Calculate runs the tree hydraulic calculation for a system and stores the
// derived result, the supply comparison, and (optionally) recomputes
// compliance. It is idempotent: re-running over the same inputs yields the
// same derived rows. Concurrent calls on the same system are serialized.
func (s *Services) Calculate(ctx context.Context, systemID string) (*model.HydraulicResult, *model.SupplyComparison, error) {
	mu := s.systemLock(systemID)
	mu.Lock()
	defer mu.Unlock()

	sys, err := s.st.GetSystem(ctx, systemID)
	if err != nil {
		return nil, nil, err
	}
	if sys.RemoteNodeID == "" {
		return nil, nil, fmt.Errorf("%w: system has no remote (most-unfavourable) node", store.ErrInvariant)
	}
	nodes, _, sourceID, net, err := s.buildNetwork(ctx, systemID)
	if err != nil {
		return nil, nil, err
	}
	remoteID, remoteP, err := s.remoteStartPressure(sys, nodes)
	if err != nil {
		return nil, nil, err
	}
	res, err := hydraulics.Calc(net, remoteID, remoteP)
	if err != nil {
		return nil, nil, fmt.Errorf("%w: %v", store.ErrInvariant, err)
	}
	res.ID = idlib.New("hyd")
	res.SystemID = systemID
	res.CalcEpoch = s.now()

	// Build the supply comparison at the base flow.
	sup, err := s.st.GetWaterSupply(ctx, systemID)
	if err != nil && err != store.ErrNotFound {
		return nil, nil, err
	}
	var pump *model.FirePump
	if sys.PumpID != "" {
		pump, err = s.st.GetFirePump(ctx, systemID)
		if err != nil && err != store.ErrNotFound {
			return nil, nil, err
		}
	}
	var cmp *model.SupplyComparison
	if sup != nil {
		cmp = watersupply.Compare(sup, pump, res.BaseFlowLPM, res.BaseRequiredPressure)
		cmp.ID = idlib.New("cmp")
		cmp.SystemID = systemID
		cmp.CheckedEpoch = s.now()
	} else {
		// No supply configured: comparison is unavailable; mark required only.
		cmp = &model.SupplyComparison{
			ID:                idlib.New("cmp"),
			SystemID:          systemID,
			BaseFlowLPM:       res.BaseFlowLPM,
			AvailablePressure: 0,
			RequiredPressure:  res.BaseRequiredPressure,
			SurplusMbar:       -res.BaseRequiredPressure,
			NeedsPump:         true,
			PumpAdded:         false,
			CheckedEpoch:       s.now(),
		}
	}
	_ = sourceID

	// Persist derived rows in one transaction.
	if err := s.persistDerived(ctx, res, cmp); err != nil {
		return nil, nil, err
	}
	// Promote state draft→designed if applicable.
	if sys.State == model.StateDraft {
		if err := s.st.UpdateSystemState(ctx, nil, systemID, model.StateDesigned, s.now()); err != nil {
			return nil, nil, err
		}
	}
	return res, cmp, nil
}

// persistDerived stores the hydraulic result and supply comparison in one
// transaction.
func (s *Services) persistDerived(ctx context.Context, res *model.HydraulicResult, cmp *model.SupplyComparison) error {
	return s.st.InTx(ctx, func(tx *sql.Tx) error {
		if err := s.st.UpsertHydraulicResult(ctx, tx, res); err != nil {
			return err
		}
		if cmp != nil {
			if err := s.st.UpsertSupplyComparison(ctx, tx, cmp); err != nil {
				return err
			}
		}
		return nil
	})
}

// RunCompliance runs the NFPA 13 checks for a system and stores them. It is
// serialized against Calculate (and other RunCompliance calls) on the same
// system so the checks always see a consistent derived row set.
func (s *Services) RunCompliance(ctx context.Context, systemID string) ([]model.ComplianceCheck, error) {
	mu := s.systemLock(systemID)
	mu.Lock()
	defer mu.Unlock()

	sys, err := s.st.GetSystem(ctx, systemID)
	if err != nil {
		return nil, err
	}
	nodes, err := s.st.ListNodesBySystem(ctx, systemID)
	if err != nil {
		return nil, err
	}
	pipes, err := s.st.ListPipesBySystem(ctx, systemID)
	if err != nil {
		return nil, err
	}
	_, _, _, net, netErr := s.buildNetwork(ctx, systemID)
	sup, _ := s.st.GetWaterSupply(ctx, systemID)
	var pump *model.FirePump
	if sys.PumpID != "" {
		pump, _ = s.st.GetFirePump(ctx, systemID)
	}
	hyd, _ := s.st.GetHydraulicResult(ctx, systemID)
	cmp, _ := s.st.GetSupplyComparison(ctx, systemID)

	in := compliance.Input{
		System:      sys,
		Nodes:       nodes,
		Pipes:       pipes,
		WaterSupply: sup,
		Pump:        pump,
		Hydraulic:   hyd,
		Supply:      cmp,
		Network:     net,
	}
	if netErr != nil {
		// Tree integrity rule will capture this; keep Network nil so the rule
		// fails informatively.
		in.Network = nil
	}
	checks := compliance.Check(in)
	for i := range checks {
		checks[i].ID = idlib.New("chk")
		checks[i].SystemID = systemID
		checks[i].CheckedEpoch = s.now()
	}
	if err := s.st.ReplaceComplianceChecks(ctx, nil, systemID, checks, s.now()); err != nil {
		return nil, err
	}
	return checks, nil
}

// GetHydraulicResult returns the stored result for a system.
func (s *Services) GetHydraulicResult(ctx context.Context, systemID string) (*model.HydraulicResult, error) {
	return s.st.GetHydraulicResult(ctx, systemID)
}

// GetSupplyComparison returns the stored comparison for a system.
func (s *Services) GetSupplyComparison(ctx context.Context, systemID string) (*model.SupplyComparison, error) {
	return s.st.GetSupplyComparison(ctx, systemID)
}

// ListComplianceChecks returns the stored checks for a system.
func (s *Services) ListComplianceChecks(ctx context.Context, systemID string) ([]model.ComplianceCheck, error) {
	return s.st.ListComplianceChecks(ctx, systemID)
}

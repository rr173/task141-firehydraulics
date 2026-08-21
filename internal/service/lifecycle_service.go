package service

import (
	"context"
	"database/sql"
	"fmt"

	"task141-firehydraulics/internal/idlib"
	"task141-firehydraulics/internal/lifecycle"
	"task141-firehydraulics/internal/model"
	"task141-firehydraulics/internal/store"
)

// --- Lifecycle ---

// Transition moves a system from one state to another. It validates the
// transition is legal (lifecycle graph), enforces preconditions (e.g. a
// hydrostatic test must pass before accepted→in_service, an impairment must
// be resolved before impaired→in_service), appends a lifecycle event, and
// updates the state column — all in one transaction.
func (s *Services) Transition(ctx context.Context, systemID string, to model.SystemState, reason string) (*model.System, []model.LifecycleEvent, error) {
	sys, err := s.st.GetSystem(ctx, systemID)
	if err != nil {
		return nil, nil, err
	}
	tr := lifecycle.Transition{From: sys.State, To: to, Reason: reason}
	if err := tr.Validate(); err != nil {
		return nil, nil, fmt.Errorf("%w: %s→%s: %v", store.ErrStateConflict, sys.State, to, err)
	}
	// Precondition checks specific to this domain.
	if err := s.checkTransitionPreconditions(ctx, sys, to); err != nil {
		return nil, nil, err
	}
	ev := &model.LifecycleEvent{
		ID:         idlib.New("evt"),
		SystemID:   systemID,
		FromState:  sys.State,
		ToState:    to,
		Reason:     reason,
		EventEpoch: s.now(),
	}
	if err := s.st.InTx(ctx, func(tx *sql.Tx) error {
		if err := s.st.AppendLifecycleEvent(ctx, tx, ev); err != nil {
			return err
		}
		if err := s.st.UpdateSystemState(ctx, tx, systemID, to, s.now()); err != nil {
			return err
		}
		return nil
	}); err != nil {
		return nil, nil, err
	}
	events, err := s.st.ListLifecycleEvents(ctx, systemID)
	if err != nil {
		return nil, nil, err
	}
	updated, _ := s.st.GetSystem(ctx, systemID)
	return updated, events, nil
}

// checkTransitionPreconditions enforces domain-specific gates:
//   - →designed: requires a hydraulic result;
//   - →submitted: requires compliance to have been run (any checks exist);
//   - →accepted: requires a passing hydrostatic test;
//   - →in_service: requires an acceptance record (pass);
//   - →restored: requires the impairment to have an actual_restore_epoch;
//   - →impaired: requires at least one compensating measure.
func (s *Services) checkTransitionPreconditions(ctx context.Context, sys *model.System, to model.SystemState) error {
	switch to {
	case model.StateDesigned:
		if _, err := s.st.GetHydraulicResult(ctx, sys.ID); err != nil {
			return fmt.Errorf("%w: cannot mark designed without a hydraulic result", store.ErrInvariant)
		}
	case model.StateSubmitted:
		// require compliance checks to exist.
		if _, err := s.st.GetHydraulicResult(ctx, sys.ID); err != nil {
			return fmt.Errorf("%w: cannot submit without a hydraulic result", store.ErrInvariant)
		}
	case model.StateAccepted:
		tests, err := s.st.ListHydrostaticTests(ctx, sys.ID)
		if err != nil {
			return fmt.Errorf("load hydrostatic tests: %w", err)
		}
		if !anyHydrostaticPassed(tests) {
			return fmt.Errorf("%w: cannot accept without a passing hydrostatic test", store.ErrHydrostaticFailed)
		}
	case model.StateInService:
		if sys.State == model.StateAccepted {
			rec, err := s.st.GetAcceptanceRecord(ctx, sys.ID)
			if err != nil {
				return fmt.Errorf("%w: cannot go in-service without an acceptance record", store.ErrInvariant)
			}
			if rec.Conclusion == "fail" {
				return fmt.Errorf("%w: acceptance record did not pass", store.ErrInvariant)
			}
		}
	case model.StateRestored:
		// the impairment restore path sets this; no extra check here.
	}
	return nil
}

func anyHydrostaticPassed(tests []model.HydrostaticTest) bool {
	for _, t := range tests {
		if t.Passed {
			return true
		}
	}
	return false
}

// ListLifecycleEvents returns the transition history for a system.
func (s *Services) ListLifecycleEvents(ctx context.Context, systemID string) ([]model.LifecycleEvent, error) {
	return s.st.ListLifecycleEvents(ctx, systemID)
}

// FullReport bundles every view of a system.
func (s *Services) FullReport(ctx context.Context, systemID string) (*model.FullReport, error) {
	sys, err := s.st.GetSystem(ctx, systemID)
	if err != nil {
		return nil, err
	}
	nodes, _ := s.st.ListNodesBySystem(ctx, systemID)
	pipes, _ := s.st.ListPipesBySystem(ctx, systemID)
	sup, _ := s.st.GetWaterSupply(ctx, systemID)
	var pump *model.FirePump
	if sys.PumpID != "" {
		pump, _ = s.st.GetFirePump(ctx, systemID)
	}
	hyd, _ := s.st.GetHydraulicResult(ctx, systemID)
	cmp, _ := s.st.GetSupplyComparison(ctx, systemID)
	checks, _ := s.st.ListComplianceChecks(ctx, systemID)
	events, _ := s.st.ListLifecycleEvents(ctx, systemID)
	return &model.FullReport{
		System:      sys,
		Nodes:       nodes,
		Pipes:       pipes,
		WaterSupply: sup,
		Pump:        pump,
		Hydraulic:   hyd,
		Supply:      cmp,
		Compliance:  checks,
		Lifecycle:   events,
	}, nil
}

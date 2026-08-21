package service

import (
	"context"
	"database/sql"
	"fmt"
	"log"

	"task141-firehydraulics/internal/compliance"
	"task141-firehydraulics/internal/hydraulics"
	"task141-firehydraulics/internal/model"
	"task141-firehydraulics/internal/watersupply"
)

// Reconciler recomputes every derived table (hydraulic results, supply
// comparisons, compliance checks) from the persisted authoritative inputs and
// corrects the persisted system state against the lifecycle event stream. It
// also auto-restores past-due impairments. This is the restart-recovery path:
// ReconcileAll is called once on startup so a process that died between writes
// converges to the same state.
type Reconciler struct {
	svc *Services
}

// ReconcileAll recomputes all derived rows and returns the count of systems
// reconciled.
func (r *Reconciler) ReconcileAll(ctx context.Context) (int, error) {
	systems, err := r.svc.st.AllSystems(ctx)
	if err != nil {
		return 0, fmt.Errorf("reconcile: load systems: %w", err)
	}
	n := 0
	for _, sys := range systems {
		if sys.State == model.StateDraft {
			continue
		}
		if err := r.reconcileSystem(ctx, &sys); err != nil {
			log.Printf("reconcile system %s: %v", sys.ID, err)
			continue
		}
		n++
	}
	// Correct persisted system states against the event stream.
	if err := r.correctStatesFromEvents(ctx); err != nil {
		log.Printf("reconcile: correct states: %v", err)
	}
	// Auto-restore past-due impairments.
	if err := r.autoRestorePastDue(ctx); err != nil {
		log.Printf("reconcile: auto restore: %v", err)
	}
	return n, nil
}

// reconcileSystem rebuilds the network, re-runs the hydraulic calculation,
// re-runs compliance, and rewrites the derived rows.
func (r *Reconciler) reconcileSystem(ctx context.Context, sys *model.System) error {
	mu := r.svc.systemLock(sys.ID)
	mu.Lock()
	defer mu.Unlock()

	nodes, err := r.svc.st.ListNodesBySystem(ctx, sys.ID)
	if err != nil {
		return err
	}
	pipes, err := r.svc.st.ListPipesBySystem(ctx, sys.ID)
	if err != nil {
		return err
	}
	// Re-run hydraulic calc if there is a remote node + source.
	if sys.RemoteNodeID == "" {
		return nil // nothing to reconcile yet
	}
	var src *model.Node
	for i := range nodes {
		if nodes[i].Type == model.NodeSource || nodes[i].Type == model.NodeBase {
			src = &nodes[i]
			break
		}
	}
	if src == nil {
		return nil
	}
	net, err := hydraulics.BuildNetwork(src.ID, nodes, pipes)
	if err != nil {
		return fmt.Errorf("build network: %w", err)
	}
	remoteID, remoteP, err := r.svc.remoteStartPressure(sys, nodes)
	if err != nil {
		return err
	}
	res, err := hydraulics.Calc(net, remoteID, remoteP)
	if err != nil {
		return fmt.Errorf("calc: %w", err)
	}
	res.ID = idlibSafe()
	res.SystemID = sys.ID
	res.CalcEpoch = r.svc.now()

	sup, _ := r.svc.st.GetWaterSupply(ctx, sys.ID)
	var pump *model.FirePump
	if sys.PumpID != "" {
		pump, _ = r.svc.st.GetFirePump(ctx, sys.ID)
	}
	var cmp *model.SupplyComparison
	if sup != nil {
		cmp = watersupply.Compare(sup, pump, res.BaseFlowLPM, res.BaseRequiredPressure)
		cmp.ID = idlibSafe()
		cmp.SystemID = sys.ID
		cmp.CheckedEpoch = r.svc.now()
	}
	// Persist in one transaction.
	if err := r.svc.st.InTx(ctx, func(tx *sql.Tx) error {
		if err := r.svc.st.UpsertHydraulicResult(ctx, tx, res); err != nil {
			return err
		}
		if cmp != nil {
			if err := r.svc.st.UpsertSupplyComparison(ctx, tx, cmp); err != nil {
				return err
			}
		}
		return nil
	}); err != nil {
		return err
	}

	// Re-run compliance.
	in := compliance.Input{
		System:      sys,
		Nodes:       nodes,
		Pipes:       pipes,
		WaterSupply: sup,
		Pump:        pump,
		Hydraulic:   res,
		Supply:      cmp,
		Network:     net,
	}
	checks := compliance.Check(in)
	for i := range checks {
		checks[i].ID = idlibSafe()
		checks[i].SystemID = sys.ID
		checks[i].CheckedEpoch = r.svc.now()
	}
	if err := r.svc.st.ReplaceComplianceChecks(ctx, nil, sys.ID, checks, r.svc.now()); err != nil {
		return err
	}
	return nil
}

// correctStatesFromEvents recomputes the persisted system.state from the last
// lifecycle event (the event stream is authoritative on restart) and corrects
// any drift.
func (r *Reconciler) correctStatesFromEvents(ctx context.Context) error {
	systems, err := r.svc.st.AllSystems(ctx)
	if err != nil {
		return err
	}
	for _, sys := range systems {
		events, err := r.svc.st.ListLifecycleEvents(ctx, sys.ID)
		if err != nil {
			continue
		}
		if len(events) == 0 {
			continue
		}
		last := events[len(events)-1]
		if sys.State != last.ToState {
			if err := r.svc.st.SetSystemStateRaw(ctx, nil, sys.ID, last.ToState, r.svc.now()); err != nil {
				return fmt.Errorf("correct state %s: %w", sys.ID, err)
			}
		}
	}
	return nil
}

// autoRestorePastDue restores impairments whose expected_restore_epoch has
// passed; it does NOT call the full Transition path (which requires the
// in_service→impaired→restored graph) because the system may already have been
// advanced by the event-stream correction. It sets the impairment status and
// the actual_restore_epoch.
func (r *Reconciler) autoRestorePastDue(ctx context.Context) error {
	active, err := r.svc.st.ListActiveImpairments(ctx)
	if err != nil {
		return err
	}
	now := r.svc.now()
	for _, im := range active {
		if im.ExpectedRestoreEpoch <= now {
			// Restore the impairment record.
			if err := r.svc.st.RestoreImpairment(ctx, nil, im.ID, now); err != nil {
				continue
			}
			// Append a restore event if the system is still impaired.
			sys, err := r.svc.st.GetSystem(ctx, im.SystemID)
			if err != nil {
				continue
			}
			if sys.State == model.StateImpaired {
				if _, _, err := r.svc.Transition(ctx, im.SystemID, model.StateRestored, "auto-restore past due"); err == nil {
					if _, _, err := r.svc.Transition(ctx, im.SystemID, model.StateInService, "back in service"); err != nil {
						return err
					}
				}
			}
		}
	}
	return nil
}

// idlibSafe is a thin shim so the reconcile file does not need to import idlib
// directly (kept local for clarity of the recovery path).
func idlibSafe() string { return newID() }

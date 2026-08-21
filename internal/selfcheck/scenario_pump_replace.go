package selfcheck

import (
	"fmt"
	"net/http/httptest"

	"task141-firehydraulics/internal/clock"
	"task141-firehydraulics/internal/model"
	"task141-firehydraulics/internal/service"
)

// addFirePumpStrong attaches the same strong pump shape addFirePump uses, with a
// distinct rated head so a regression can detect a stale (old) pump being
// resolved instead of the replacement.
func addFirePumpStrong(srv *httptest.Server, systemID string, ratedHead int64) error {
	return mustDo(srv, "POST", "/api/systems/"+systemID+"/pump",
		map[string]any{
			"rated_flow_lpm":         1500,
			"rated_head_mbar":        ratedHead,
			"churn_pressure_mbar":    10000,
			"hundred_fifty_flow_lpm": 2250,
			"hundred_fifty_head_mbar": 5000,
			"driver_type":            "electric",
			"rated_rpm":              3000,
		}, nil)
}

// getFirePumpByID fetches the currently linked pump for a system.
func getFirePumpByID(srv *httptest.Server, systemID string) (*model.FirePump, error) {
	var p model.FirePump
	if err := mustDo(srv, "GET", "/api/systems/"+systemID+"/pump", nil, &p); err != nil {
		return nil, err
	}
	return &p, nil
}

// smokePumpReplacementAdopted reproduces the reported defect: after the fire
// pump is swapped for a new qualified device, both the immediate (re)calculate
// path and the restart-reconcile recovery path must adopt the replacement
// (supply turns adequate), not silently keep resolving the deleted old pump
// (which left supply judged inadequate).
//
// Regression for the bug where CreateFirePump linked the system to the OLD
// pump id on replacement, so GetFirePump found no row and the new pump's boost
// was dropped in Calculate and reconcileSystem.
func smokePumpReplacementAdopted(srv *httptest.Server, clk *clock.Fake) error {
	_, sid, _, _, err := buildSimpleTree(srv, model.HazardExtra2, 95, 23200, 50)
	if err != nil {
		return err
	}
	// Weak supply alone → deficit (needs pump), mirroring smokePumpAugmentsSupply.
	if err := mustDo(srv, "POST", "/api/systems/"+sid+"/water-supply",
		map[string]any{"kind": "city", "static_pressure_mbar": 700,
			"points": []map[string]any{
				{"flow_lpm": 200, "residual_pressure_mbar": 400},
				{"flow_lpm": 500, "residual_pressure_mbar": 100},
			}}, nil); err != nil {
		return err
	}
	// First device installed: rated head 8000 mbar — adequate.
	if err := addFirePumpStrong(srv, sid, 8000); err != nil {
		return err
	}
	_, cmp1, err := callCalc(srv, sid)
	if err != nil {
		return err
	}
	if cmp1 == nil {
		return fmt.Errorf("no supply comparison after first pump")
	}
	if !cmp1.PumpAdded || cmp1.SurplusMbar < 0 {
		return fmt.Errorf("first pump inadequate (pump_added=%v surplus=%d)", cmp1.PumpAdded, cmp1.SurplusMbar)
	}

	// --- The reported failure: replace the pump with a NEW qualified device. ---
	// Distinct rated head (9000) lets us prove the resolved pump is the new one,
	// not a stale reference to the old deleted row.
	if err := addFirePumpStrong(srv, sid, 9000); err != nil {
		return err
	}
	// The GET /pump endpoint (joins systems.pump_id → fire_pumps) must return the
	// REPLACEMENT device, proving the link points at the new row.
	pump, err := getFirePumpByID(srv, sid)
	if err != nil {
		return fmt.Errorf("get pump after replacement: %w", err)
	}
	if pump.RatedHeadMbar != 9000 {
		return fmt.Errorf("post-replacement pump resolved stale: want rated_head 9000, got %d", pump.RatedHeadMbar)
	}

	// Immediate recalc must adopt the new pump → adequate.
	_, cmp2, err := callCalc(srv, sid)
	if err != nil {
		return err
	}
	if cmp2 == nil {
		return fmt.Errorf("no supply comparison after replacement")
	}
	if !cmp2.PumpAdded {
		return fmt.Errorf("replacement pump not adopted in calculate (pump_added=false)")
	}
	if cmp2.SurplusMbar < 0 {
		return fmt.Errorf("replacement pump did not lift supply (surplus %d)", cmp2.SurplusMbar)
	}
	// R-supply-adequacy must pass on the immediate path.
	if err := mustDo(srv, "POST", "/api/systems/"+sid+"/compliance", nil, nil); err != nil {
		return err
	}
	return nil
}

// smokePumpReplacementAfterRestart is the restart-recovery half of the same
// regression: after the pump is swapped, a crash + ReconcileAll must recompute
// an adequate supply from the persisted replacement pump (not a stale/deleted
// old pump that leaves supply inadequate).
func smokePumpReplacementAfterRestart(dbPath string, clk *clock.Fake) error {
	// Phase 1: build the same weak-supply + swapped-pump system on a crashable
	// store, leaving the replacement pump persisted.
	srv1, st1, err := restartServer(dbPath, clk)
	if err != nil {
		return err
	}
	defer func() { _ = st1.Close() }()
	defer srv1.Close()
	_, sid, _, _, err := buildSimpleTree(srv1, model.HazardExtra2, 95, 23200, 50)
	if err != nil {
		return err
	}
	if err := mustDo(srv1, "POST", "/api/systems/"+sid+"/water-supply",
		map[string]any{"kind": "city", "static_pressure_mbar": 700,
			"points": []map[string]any{
				{"flow_lpm": 200, "residual_pressure_mbar": 400},
				{"flow_lpm": 500, "residual_pressure_mbar": 100},
			}}, nil); err != nil {
		return err
	}
	if err := addFirePumpStrong(srv1, sid, 8000); err != nil {
		return err
	}
	// Swap to the new qualified device (rated head 9000).
	if err := addFirePumpStrong(srv1, sid, 9000); err != nil {
		return err
	}
	if _, _, err := callCalc(srv1, sid); err != nil {
		return err
	}
	// Snapshot the post-swap adequate surplus (immediate path, pre-crash).
	preSwap, err := getSupplyComparison(srv1, sid)
	if err != nil {
		return err
	}
	if preSwap == nil || !preSwap.PumpAdded || preSwap.SurplusMbar < 0 {
		return fmt.Errorf("pre-crash swap: expected adequate supply, got pump_added=%v surplus=%d", boolPtr(preSwap), surplusPtr(preSwap))
	}

	// Crash: close server + store.
	srv1.Close()
	if err := st1.Close(); err != nil {
		return err
	}

	// Phase 2: reopen + reconcile.
	srv2, st2, err := restartServer(dbPath, clk)
	if err != nil {
		return err
	}
	defer srv2.Close()
	defer st2.Close()
	svc2 := service.NewWithClock(st2, clk)
	if _, err := svc2.Reconcile().ReconcileAll(ctx()); err != nil {
		return fmt.Errorf("reconcile: %w", err)
	}
	// The reconciled pump must be the REPLACEMENT (rated head 9000), proving the
	// restart path resolves the new device, not the deleted old one.
	pump, err := getFirePumpByID(srv2, sid)
	if err != nil {
		return fmt.Errorf("get pump after restart: %w", err)
	}
	if pump.RatedHeadMbar != 9000 {
		return fmt.Errorf("post-restart pump resolved stale: want rated_head 9000, got %d", pump.RatedHeadMbar)
	}
	// The reconciled supply comparison must be adequate (pump adopted) and match
	// the pre-crash surplus (idempotent recovery).
	postSwap, err := getSupplyComparison(srv2, sid)
	if err != nil {
		return err
	}
	if postSwap == nil {
		return fmt.Errorf("post-restart: no supply comparison")
	}
	if !postSwap.PumpAdded {
		return fmt.Errorf("post-restart: replacement pump not adopted (pump_added=false)")
	}
	if postSwap.SurplusMbar < 0 {
		return fmt.Errorf("post-restart: supply still inadequate after reconcile (surplus %d) — replacement pump dropped", postSwap.SurplusMbar)
	}
	if postSwap.SurplusMbar != preSwap.SurplusMbar {
		return fmt.Errorf("post-restart surplus drifted: pre=%d post=%d", preSwap.SurplusMbar, postSwap.SurplusMbar)
	}
	return nil
}

// getSupplyComparison fetches the stored supply comparison for a system.
func getSupplyComparison(srv *httptest.Server, systemID string) (*model.SupplyComparison, error) {
	var c model.SupplyComparison
	code, body, err := doJSON(srv, "GET", "/api/systems/"+systemID+"/supply-comparison", nil)
	if err != nil {
		return nil, err
	}
	if code == 404 {
		return nil, nil
	}
	if code >= 300 {
		return nil, fmt.Errorf("get supply comparison: %d: %s", code, string(body))
	}
	if err := jsonUnmarshal(body, &c); err != nil {
		return nil, err
	}
	return &c, nil
}

// boolPtr / surplusPtr keep the pre-crash assertion message readable when the
// pointer may be nil (avoids dereferencing a nil *model.SupplyComparison).
func boolPtr(c *model.SupplyComparison) bool { return c != nil && c.PumpAdded }
func surplusPtr(c *model.SupplyComparison) int64 {
	if c == nil {
		return 0
	}
	return c.SurplusMbar
}

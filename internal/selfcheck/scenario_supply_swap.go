package selfcheck

import (
	"fmt"
	"net/http/httptest"
	"os"
	"path/filepath"

	"task141-firehydraulics/internal/clock"
	"task141-firehydraulics/internal/model"
	"task141-firehydraulics/internal/service"
)

// getSupplyComparison fetches the stored supply comparison for a system (nil if
// the system has no stored comparison, i.e. the endpoint returns 404).
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

// weakSupply attaches a deliberately low-pressure municipal curve that produces
// a deficit at the base flow.
func weakSupply(srv *httptest.Server, systemID string) error {
	return mustDo(srv, "POST", "/api/systems/"+systemID+"/water-supply",
		map[string]any{"kind": "city", "static_pressure_mbar": 700,
			"points": []map[string]any{
				{"flow_lpm": 200, "residual_pressure_mbar": 400},
				{"flow_lpm": 500, "residual_pressure_mbar": 100},
			}}, nil)
}

// smokeSupplyReplacementRebuilds reproduces the reported defect: after the
// generous municipal supply curve is replaced by a low-pressure curve, the
// page must NOT keep showing the old surplus, and a restart must rebuild the
// comparison from the new curve rather than carrying the stale surplus forward.
//
// The system stays in the designed state (where supply edits are legal). Flow:
// build a tree, attach an adequate supply, run the calc (positive surplus);
// swap the supply for a weak curve WITHOUT re-running the calc, so the stale
// (adequate) surplus is left on disk — exactly the reported situation. Then
// close the store, reopen the same SQLite file, run ReconcileAll and assert the
// rebuilt comparison now reflects the weak curve's deficit — not the cached old
// surplus. (Before the fix the restart reconcile short-circuited whenever a
// comparison already existed, so the stale surplus survived the restart.)
//
// The scenario owns its own db file + server pair so it can reopen the same
// file for the restart phase; the srv handed to it by Run is closed by Run.
func smokeSupplyReplacementRebuilds(_ *httptest.Server, clk *clock.Fake) error {
	dir, err := os.MkdirTemp("", "firehydraulics-swap-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)
	dbPath := filepath.Join(dir, "swap.db")

	// Phase 1: build a tree + adequate supply + calc → positive surplus.
	srv1, st1, err := restartServer(dbPath, clk)
	if err != nil {
		return err
	}
	_, sid, _, _, err := buildSimpleTree(srv1, model.HazardExtra2, 95, 23200, 50)
	if err != nil {
		srv1.Close()
		_ = st1.Close()
		return err
	}
	if err := addAdequateSupply(srv1, sid); err != nil {
		srv1.Close()
		_ = st1.Close()
		return err
	}
	if _, _, err := callCalc(srv1, sid); err != nil {
		srv1.Close()
		_ = st1.Close()
		return err
	}
	pre, err := getSupplyComparison(srv1, sid)
	if err != nil {
		srv1.Close()
		_ = st1.Close()
		return err
	}
	if pre == nil {
		srv1.Close()
		_ = st1.Close()
		return fmt.Errorf("pre-swap: no supply comparison")
	}
	if pre.SurplusMbar < 0 {
		srv1.Close()
		_ = st1.Close()
		return fmt.Errorf("pre-swap: expected surplus with adequate supply, got deficit %d", pre.SurplusMbar)
	}

	// Swap the generous curve for a weak one (legal in designed state) but do
	// NOT re-run the calc — leaving the stale adequate surplus on disk, as in
	// the reported incident.
	if err := weakSupply(srv1, sid); err != nil {
		srv1.Close()
		_ = st1.Close()
		return err
	}

	// Phase 2: close to simulate a crash, reopen the same file, reconcile.
	srv1.Close()
	if err := st1.Close(); err != nil {
		return err
	}
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
	// After restart the comparison must reflect the weak curve's deficit, NOT
	// the stale adequate surplus that was left on disk. A non-negative surplus
	// here means the restart reconcile skipped recompute and served the old
	// result — the exact reported defect.
	rec, err := getSupplyComparison(srv2, sid)
	if err != nil {
		return err
	}
	if rec == nil {
		return fmt.Errorf("post-restart: no supply comparison")
	}
	if rec.SurplusMbar >= 0 {
		return fmt.Errorf("post-restart: expected deficit after replacing supply, got surplus %d (stale result retained across restart)", rec.SurplusMbar)
	}
	return nil
}

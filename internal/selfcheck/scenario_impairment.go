package selfcheck

import (
	"fmt"
	"net/http/httptest"

	"task141-firehydraulics/internal/clock"
	"task141-firehydraulics/internal/model"
)

// bringToInService is a shared helper that drives a system fully in-service.
func bringToInService(srv *httptest.Server, hazard model.HazardClass, name string) (string, error) {
	_, sid, _, _, err := buildSimpleTree(srv, hazard, densityFor(hazard), areaFor(hazard), perHeadFor(hazard))
	if err != nil {
		return "", err
	}
	if err := addAdequateSupply(srv, sid); err != nil {
		return "", err
	}
	if _, _, err := callCalc(srv, sid); err != nil {
		return "", err
	}
	if err := mustDo(srv, "POST", "/api/systems/"+sid+"/compliance", nil, nil); err != nil {
		return "", err
	}
	if _, err := transitionTo(srv, sid, model.StateSubmitted, "submit"); err != nil {
		return "", err
	}
	if _, err := transitionTo(srv, sid, model.StateApproved, "approve"); err != nil {
		return "", err
	}
	if _, err := transitionTo(srv, sid, model.StateInstalled, "install"); err != nil {
		return "", err
	}
	if err := mustDo(srv, "POST", "/api/systems/"+sid+"/hydrostatic-test",
		map[string]any{"test_pressure_mbar": 34000, "hold_seconds": 7200, "leaked": false}, nil); err != nil {
		return "", err
	}
	if _, err := transitionTo(srv, sid, model.StateHydrostatic, "hydro"); err != nil {
		return "", err
	}
	if err := mustDo(srv, "POST", "/api/systems/"+sid+"/acceptance",
		map[string]any{"conclusion": "pass", "accepted_by": "AHJ", "defects": []string{}}, nil); err != nil {
		return "", err
	}
	if _, err := transitionTo(srv, sid, model.StateAccepted, "accept"); err != nil {
		return "", err
	}
	if _, err := transitionTo(srv, sid, model.StateInService, "in service"); err != nil {
		return "", err
	}
	return sid, nil
}

func densityFor(h model.HazardClass) int64 {
	switch h {
	case model.HazardLight:
		return 21
	case model.HazardOrdinary1:
		return 43
	case model.HazardOrdinary2:
		return 52
	case model.HazardExtra1:
		return 75
	case model.HazardExtra2:
		return 95
	case model.HazardStorage:
		return 80
	}
	return 95
}
func areaFor(h model.HazardClass) int64 {
	switch h {
	case model.HazardLight, model.HazardOrdinary1, model.HazardOrdinary2:
		return 13900
	default:
		return 23200
	}
}

// perHeadFor returns the per-sprinkler coverage (dm²) sized so the remote
// sprinkler's k80 @ 0.5 bar (57 L/min) discharge meets the design density with
// a small margin, keeping the smoke test's two-head tree a valid design.
func perHeadFor(h model.HazardClass) int64 {
	switch h {
	case model.HazardLight:
		return 200 // 57/2 = 28 ≥ 21
	case model.HazardOrdinary1:
		return 100 // 57/1 = 57 ≥ 43
	case model.HazardOrdinary2:
		return 90 // 57/0.9 = 63 ≥ 52
	case model.HazardExtra1:
		return 60 // 57/0.6 = 95 ≥ 75
	case model.HazardExtra2:
		return 50 // 57/0.5 = 114 ≥ 95
	case model.HazardStorage:
		return 55
	}
	return 50
}

// smokeImpairmentRequiresCompensation drives a system to in_service, then
// attempts to register an impairment with NO compensating measure; the engine
// must reject with 422. Then registering with a patrol measure succeeds.
func smokeImpairmentRequiresCompensation(srv *httptest.Server, clk *clock.Fake) error {
	sid, err := bringToInService(srv, model.HazardOrdinary1, "sys")
	if err != nil {
		return err
	}
	// No measures → 422.
	if err := expectCode(srv, "POST", "/api/systems/"+sid+"/impairments",
		map[string]any{"scope": "zone-A", "reason": "valve maintenance",
			"started_epoch": 0, "expected_restore_epoch": 86400, "measures": []any{}}, 422); err != nil {
		return err
	}
	// With a patrol measure → 201.
	var im model.Impairment
	if err := mustDo(srv, "POST", "/api/systems/"+sid+"/impairments",
		map[string]any{"scope": "zone-A", "reason": "valve maintenance",
			"started_epoch": 0, "expected_restore_epoch": 86400,
			"measures": []map[string]any{{"kind": "patrol", "owner": "guard-1"}}}, &im); err != nil {
		return err
	}
	if im.Status != model.ImpairmentActive {
		return fmt.Errorf("impairment not active: %s", im.Status)
	}
	// System should now be impaired.
	sys, err := getSystem(srv, sid)
	if err != nil {
		return err
	}
	if sys.State != model.StateImpaired {
		return fmt.Errorf("system not impaired: %s", sys.State)
	}
	return nil
}

// smokeImpairmentRestore restores an impairment and asserts the system goes
// back to in_service.
func smokeImpairmentRestore(srv *httptest.Server, clk *clock.Fake) error {
	sid, err := bringToInService(srv, model.HazardOrdinary1, "sys2")
	if err != nil {
		return err
	}
	var im model.Impairment
	if err := mustDo(srv, "POST", "/api/systems/"+sid+"/impairments",
		map[string]any{"scope": "zone-B", "reason": "pump swap",
			"started_epoch": 0, "expected_restore_epoch": 86400,
			"measures": []map[string]any{{"kind": "patrol", "owner": "guard-2"}}}, &im); err != nil {
		return err
	}
	// Advance the fake clock past expected restore and call restore.
	clk.Advance(90000 * 1e9) // 25h
	var restored model.Impairment
	if err := mustDo(srv, "POST", "/api/impairments/"+im.ID+"/restore", nil, &restored); err != nil {
		return err
	}
	if restored.Status != model.ImpairmentRestored {
		return fmt.Errorf("impairment not restored: %s", restored.Status)
	}
	if restored.ActualRestoreEpoch == 0 {
		return fmt.Errorf("actual restore epoch not set")
	}
	sys, err := getSystem(srv, sid)
	if err != nil {
		return err
	}
	if sys.State != model.StateInService {
		return fmt.Errorf("system not back in service: %s", sys.State)
	}
	return nil
}

// buildSystemInProject adds a complete sprinkler tree under an existing project.
func buildSystemInProject(srv *httptest.Server, projectID, sysName string, hazard model.HazardClass) (string, error) {
	_, sid, _, _, err := buildTreeInProject(srv, projectID, sysName, hazard, densityFor(hazard), areaFor(hazard), perHeadFor(hazard))
	return sid, err
}

// driveToInService builds + drives a system that already has a project+system
// + network set up (created by buildSystemInProject) all the way to in_service.
func driveToInService(srv *httptest.Server, sid string) error {
	if err := addAdequateSupply(srv, sid); err != nil {
		return err
	}
	if _, _, err := callCalc(srv, sid); err != nil {
		return err
	}
	if err := mustDo(srv, "POST", "/api/systems/"+sid+"/compliance", nil, nil); err != nil {
		return err
	}
	if _, err := transitionTo(srv, sid, model.StateSubmitted, "submit"); err != nil {
		return err
	}
	if _, err := transitionTo(srv, sid, model.StateApproved, "approve"); err != nil {
		return err
	}
	if _, err := transitionTo(srv, sid, model.StateInstalled, "install"); err != nil {
		return err
	}
	if err := mustDo(srv, "POST", "/api/systems/"+sid+"/hydrostatic-test",
		map[string]any{"test_pressure_mbar": 34000, "hold_seconds": 7200, "leaked": false}, nil); err != nil {
		return err
	}
	if _, err := transitionTo(srv, sid, model.StateHydrostatic, "hydro"); err != nil {
		return err
	}
	if err := mustDo(srv, "POST", "/api/systems/"+sid+"/acceptance",
		map[string]any{"conclusion": "pass", "accepted_by": "AHJ", "defects": []string{}}, nil); err != nil {
		return err
	}
	if _, err := transitionTo(srv, sid, model.StateAccepted, "accept"); err != nil {
		return err
	}
	if _, err := transitionTo(srv, sid, model.StateInService, "in service"); err != nil {
		return err
	}
	return nil
}

// smokeCrossSystemCompensation creates two systems in the SAME project, impairs
// the first (patrol only), then attempts to impair the second with patrol
// only — adjacent impairments without a manual fire watch must be rejected.
func smokeCrossSystemCompensation(srv *httptest.Server, clk *clock.Fake) error {
	// One shared project, two systems under it.
	var proj struct {
		ID string `json:"id"`
	}
	if err := mustDo(srv, "POST", "/api/projects",
		map[string]any{"name": "共享项目", "hazard_class": string(model.HazardOrdinary1), "design_date": 0}, &proj); err != nil {
		return err
	}
	pid := proj.ID
	sid1, err := buildSystemInProject(srv, pid, "sysA", model.HazardOrdinary1)
	if err != nil {
		return err
	}
	if err := driveToInService(srv, sid1); err != nil {
		return err
	}
	sid2, err := buildSystemInProject(srv, pid, "sysB", model.HazardOrdinary1)
	if err != nil {
		return err
	}
	if err := driveToInService(srv, sid2); err != nil {
		return err
	}
	// Impair sid1 with patrol only.
	if err := mustDo(srv, "POST", "/api/systems/"+sid1+"/impairments",
		map[string]any{"scope": "zone-A", "reason": "valve",
			"started_epoch": 0, "expected_restore_epoch": 86400,
			"measures": []map[string]any{{"kind": "patrol", "owner": "g1"}}}, nil); err != nil {
		return err
	}
	// Attempt to impair sid2 with patrol only → 422 (adjacent, no watch).
	if err := expectCode(srv, "POST", "/api/systems/"+sid2+"/impairments",
		map[string]any{"scope": "zone-B", "reason": "valve",
			"started_epoch": 0, "expected_restore_epoch": 86400,
			"measures": []map[string]any{{"kind": "patrol", "owner": "g2"}}}, 422); err != nil {
		return err
	}
	// Impair sid2 with a manual fire watch → 201.
	if err := mustDo(srv, "POST", "/api/systems/"+sid2+"/impairments",
		map[string]any{"scope": "zone-B", "reason": "valve",
			"started_epoch": 0, "expected_restore_epoch": 86400,
			"measures": []map[string]any{{"kind": "manual_fire_watch", "owner": "g2"}}}, nil); err != nil {
		return err
	}
	return nil
}

// getSystem fetches a system by id.
func getSystem(srv *httptest.Server, id string) (*model.System, error) {
	var sys model.System
	if err := mustDo(srv, "GET", "/api/systems/"+id, nil, &sys); err != nil {
		return nil, err
	}
	return &sys, nil
}

// driveToHydrostatic walks a system up to (and including) the hydrostatic
// state so a hydrostatic scenario can record its own test(s) before the
// →accepted acceptance gate.
func driveToHydrostatic(srv *httptest.Server, sid string) error {
	if err := addAdequateSupply(srv, sid); err != nil {
		return err
	}
	if _, _, err := callCalc(srv, sid); err != nil {
		return err
	}
	if err := mustDo(srv, "POST", "/api/systems/"+sid+"/compliance", nil, nil); err != nil {
		return err
	}
	if _, err := transitionTo(srv, sid, model.StateSubmitted, "submit"); err != nil {
		return err
	}
	if _, err := transitionTo(srv, sid, model.StateApproved, "approve"); err != nil {
		return err
	}
	if _, err := transitionTo(srv, sid, model.StateInstalled, "install"); err != nil {
		return err
	}
	if _, err := transitionTo(srv, sid, model.StateHydrostatic, "ready to test"); err != nil {
		return err
	}
	return nil
}

// recordHydroTest posts a hydrostatic test and decodes the returned record so
// the scenario can assert on the authoritative verdict.
func recordHydroTest(srv *httptest.Server, sid string, pressureMbar, holdSec int64, leaked bool) (*model.HydrostaticTest, error) {
	var t model.HydrostaticTest
	if err := mustDo(srv, "POST", "/api/systems/"+sid+"/hydrostatic-test",
		map[string]any{"test_pressure_mbar": pressureMbar, "hold_seconds": holdSec, "leaked": leaked}, &t); err != nil {
		return nil, err
	}
	return &t, nil
}

// listHydroTests re-reads the recorded tests for a system (the read-back path).
func listHydroTests(srv *httptest.Server, sid string) ([]model.HydrostaticTest, error) {
	var resp struct {
		Tests []model.HydrostaticTest `json:"tests"`
	}
	if err := mustDo(srv, "GET", "/api/systems/"+sid+"/hydrostatic-test", nil, &resp); err != nil {
		return nil, err
	}
	return resp.Tests, nil
}

// smokeHydrostaticDeficientGate proves a hydrostatic test with sub-floor
// pressure OR insufficient hold time cannot pass the acceptance gate, even
// when it does not leak. It covers all three verdict sites: creation (recorded
// Passed=false), read-back (list re-derives the same verdict), and the
// hydrostatic→accepted acceptance gate (transition rejected 422). A
// floor-clearing test then lets the same system proceed, proving the gate is
// structural, not a blanket block.
func smokeHydrostaticDeficientGate(srv *httptest.Server, clk *clock.Fake) error {
	_, sid, _, _, err := buildSimpleTree(srv, model.HazardOrdinary1, 43, 13900, 100)
	if err != nil {
		return err
	}
	if err := driveToHydrostatic(srv, sid); err != nil {
		return err
	}

	// Sub-floor pressure (below the 2000 mbar floor) but legal hold, no leak.
	// Pre-fix this was recorded as passed and let acceptance proceed.
	t1, err := recordHydroTest(srv, sid, 1200, 7200, false)
	if err != nil {
		return err
	}
	if t1.Passed {
		return fmt.Errorf("sub-floor pressure test recorded as passed: %+v", t1)
	}
	// Read-back must agree (verdict re-derived from pressure/hold, not leaked).
	tests, err := listHydroTests(srv, sid)
	if err != nil {
		return err
	}
	if len(tests) == 0 {
		return fmt.Errorf("no hydrostatic tests read back")
	}
	if tests[len(tests)-1].Passed {
		return fmt.Errorf("sub-floor pressure test read back as passed")
	}
	// The acceptance gate (hydrostatic→accepted) must reject: no
	// floor-clearing test yet.
	if err := expectCode(srv, "POST", "/api/systems/"+sid+"/lifecycle",
		map[string]any{"to": string(model.StateAccepted), "reason": "accept"}, 422); err != nil {
		return err
	}

	// Insufficient hold time (below the 7200s floor) but legal pressure, no leak.
	t2, err := recordHydroTest(srv, sid, 2000, 3600, false)
	if err != nil {
		return err
	}
	if t2.Passed {
		return fmt.Errorf("short-hold test recorded as passed: %+v", t2)
	}
	// Still no clearing test → acceptance still rejected.
	if err := expectCode(srv, "POST", "/api/systems/"+sid+"/lifecycle",
		map[string]any{"to": string(model.StateAccepted), "reason": "accept"}, 422); err != nil {
		return err
	}

	// A genuinely leaking test fails even with floor-clearing pressure/hold.
	t3, err := recordHydroTest(srv, sid, 34000, 7200, true)
	if err != nil {
		return err
	}
	if t3.Passed {
		return fmt.Errorf("leaking test recorded as passed: %+v", t3)
	}

	// A floor-clearing, non-leaking test finally lets the system through.
	if _, err := recordHydroTest(srv, sid, 34000, 7200, false); err != nil {
		return err
	}
	if _, err := transitionTo(srv, sid, model.StateAccepted, "accept"); err != nil {
		return fmt.Errorf("accept with floor-clearing test: %w", err)
	}
	return nil
}

package selfcheck

import (
	"fmt"
	"net/http/httptest"

	"task141-firehydraulics/internal/clock"
	"task141-firehydraulics/internal/model"
)

// addFirePump attaches a fire pump that lifts a weak supply over the required
// pressure.
func addFirePump(srv *httptest.Server, systemID string) error {
	return mustDo(srv, "POST", "/api/systems/"+systemID+"/pump",
		map[string]any{
			"rated_flow_lpm":          1500,
			"rated_head_mbar":         8000,
			"churn_pressure_mbar":     10000,
			"hundred_fifty_flow_lpm":  2250,
			"hundred_fifty_head_mbar": 5000,
			"driver_type":             "electric",
			"rated_rpm":               3000,
		}, nil)
}

// smokePumpAugmentsSupply shows that a weak supply alone fails adequacy, but
// adding a pump turns it green.
func smokePumpAugmentsSupply(srv *httptest.Server, clk *clock.Fake) error {
	_, sid, _, _, err := buildSimpleTree(srv, model.HazardExtra2, 95, 23200, 50)
	if err != nil {
		return err
	}
	// Weak supply: low static, steep drop → deficit at base flow (~114 L/min,
	// required ~800 mbar); supply alone is insufficient.
	if err := mustDo(srv, "POST", "/api/systems/"+sid+"/water-supply",
		map[string]any{"kind": "city", "static_pressure_mbar": 700,
			"points": []map[string]any{
				{"flow_lpm": 200, "residual_pressure_mbar": 400},
				{"flow_lpm": 500, "residual_pressure_mbar": 100},
			}}, nil); err != nil {
		return err
	}
	_, cmp, err := callCalc(srv, sid)
	if err != nil {
		return err
	}
	if cmp == nil {
		return fmt.Errorf("no supply comparison (pre-pump)")
	}
	if !cmp.NeedsPump {
		return fmt.Errorf("expected needs_pump=true pre-pump")
	}
	// Add pump + recalc.
	if err := addFirePump(srv, sid); err != nil {
		return err
	}
	_, cmp2, err := callCalc(srv, sid)
	if err != nil {
		return err
	}
	if cmp2 == nil {
		return fmt.Errorf("no supply comparison (post-pump)")
	}
	if !cmp2.PumpAdded {
		return fmt.Errorf("expected pump_added=true post-pump")
	}
	if cmp2.SurplusMbar < 0 {
		return fmt.Errorf("pump did not lift supply over required (surplus %d)", cmp2.SurplusMbar)
	}
	// Compliance R-supply-adequacy should now pass.
	var comp struct {
		Checks []model.ComplianceCheck `json:"checks"`
	}
	if err := mustDo(srv, "POST", "/api/systems/"+sid+"/compliance", nil, &comp); err != nil {
		return err
	}
	for _, c := range comp.Checks {
		if c.RuleCode == "R-supply-adequacy" && !c.Passed {
			return fmt.Errorf("R-supply-adequacy expected pass post-pump, got fail: %s", c.Detail)
		}
	}
	return nil
}

// transitionTo advances a system through the lifecycle, calling /calculate and
// /compliance as needed to satisfy preconditions.
func transitionTo(srv *httptest.Server, systemID string, to model.SystemState, reason string) (*model.System, error) {
	var resp struct {
		System  *model.System          `json:"system"`
		Lifecycle []model.LifecycleEvent `json:"lifecycle"`
	}
	if err := mustDo(srv, "POST", "/api/systems/"+systemID+"/lifecycle",
		map[string]any{"to": string(to), "reason": reason}, &resp); err != nil {
		return nil, err
	}
	return resp.System, nil
}

// smokeLifecycleToInService drives a system all the way to in_service:
// calc → compliance → submit → approve → install → hydrostatic(pass) →
// accept → in_service.
func smokeLifecycleToInService(srv *httptest.Server, clk *clock.Fake) error {
	_, sid, _, _, err := buildSimpleTree(srv, model.HazardExtra2, 95, 23200, 50)
	if err != nil {
		return err
	}
	if err := addAdequateSupply(srv, sid); err != nil {
		return err
	}
	if _, _, err := callCalc(srv, sid); err != nil {
		return err
	}
	if err := mustDo(srv, "POST", "/api/systems/"+sid+"/compliance", nil, nil); err != nil {
		return err
	}
	if _, err := transitionTo(srv, sid, model.StateSubmitted, "plan review"); err != nil {
		return err
	}
	if _, err := transitionTo(srv, sid, model.StateApproved, "approved"); err != nil {
		return err
	}
	if _, err := transitionTo(srv, sid, model.StateInstalled, "installed"); err != nil {
		return err
	}
	// Hydrostatic test (pass).
	if err := mustDo(srv, "POST", "/api/systems/"+sid+"/hydrostatic-test",
		map[string]any{"test_pressure_mbar": 34000, "hold_seconds": 7200, "leaked": false}, nil); err != nil {
		return err
	}
	if _, err := transitionTo(srv, sid, model.StateHydrostatic, "hydro passed"); err != nil {
		return err
	}
	// Acceptance record (pass).
	if err := mustDo(srv, "POST", "/api/systems/"+sid+"/acceptance",
		map[string]any{"conclusion": "pass", "accepted_by": "AHJ", "defects": []string{}}, nil); err != nil {
		return err
	}
	if _, err := transitionTo(srv, sid, model.StateAccepted, "accepted"); err != nil {
		return err
	}
	sys, err := transitionTo(srv, sid, model.StateInService, "in service")
	if err != nil {
		return err
	}
	if sys.State != model.StateInService {
		return fmt.Errorf("want in_service, got %s", sys.State)
	}
	return nil
}

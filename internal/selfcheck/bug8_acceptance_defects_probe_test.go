package selfcheck

import (
	"net/http/httptest"
	"task141-firehydraulics/internal/clock"
	"task141-firehydraulics/internal/model"
	"testing"
)

func TestBug08_DefectiveAcceptanceCannotEnterService(t *testing.T) {
	clk := clock.NewFake(parseTime("2026-02-06T08:00:00Z"))
	srv, err := newServer(t.TempDir()+"/acceptance.db", clk)
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()
	sid := driveToAcceptedWithDefect(t, srv)
	if err := expectCode(srv, "POST", "/api/systems/"+sid+"/lifecycle", map[string]any{"to": "in_service", "reason": "commission"}, 422); err != nil {
		t.Fatal(err)
	}
}
func driveToAcceptedWithDefect(t *testing.T, srv *httptest.Server) string {
	t.Helper()
	_, sid, _, _, err := buildSimpleTree(srv, model.HazardOrdinary1, 43, 13900, 100)
	if err != nil {
		t.Fatal(err)
	}
	if err := addAdequateSupply(srv, sid); err != nil {
		t.Fatal(err)
	}
	if _, _, err := callCalc(srv, sid); err != nil {
		t.Fatal(err)
	}
	if err := mustDo(srv, "POST", "/api/systems/"+sid+"/compliance", nil, nil); err != nil {
		t.Fatal(err)
	}
	for _, to := range []model.SystemState{model.StateSubmitted, model.StateApproved, model.StateInstalled} {
		if _, err := transitionTo(srv, sid, to, "advance"); err != nil {
			t.Fatal(err)
		}
	}
	if err := mustDo(srv, "POST", "/api/systems/"+sid+"/hydrostatic-test", map[string]any{"test_pressure_mbar": 34000, "hold_seconds": 7200, "leaked": false}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := transitionTo(srv, sid, model.StateHydrostatic, "hydro"); err != nil {
		t.Fatal(err)
	}
	if err := mustDo(srv, "POST", "/api/systems/"+sid+"/acceptance", map[string]any{"conclusion": "pass", "accepted_by": "AHJ", "defects": []string{"alarm valve tag missing"}}, nil); err != nil {
		t.Fatal(err)
	}
	if _, err := transitionTo(srv, sid, model.StateAccepted, "accept"); err != nil {
		t.Fatal(err)
	}
	return sid
}

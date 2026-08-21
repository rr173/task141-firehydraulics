package selfcheck

import (
	"path/filepath"
	"testing"

	"task141-firehydraulics/internal/clock"
	"task141-firehydraulics/internal/model"
	"task141-firehydraulics/internal/service"
	"task141-firehydraulics/internal/watersupply"
)

func TestBug01_PumpBoostSurvivesCalculateAndRecovery(t *testing.T) {
	weak := &model.WaterSupply{StaticPressure: 700, Points: []model.SupplyPoint{{FlowLPM: 200, Pressure: 400}, {FlowLPM: 500, Pressure: 100}}}
	pump := &model.FirePump{RatedFlowLPM: 1500, RatedHeadMbar: 8000, ChurnPressureMbar: 10000, FiftyExtraFlowLPM: 2250, FiftyExtraHeadMbar: 5000}
	if got := watersupply.Compare(weak, pump, 200, 800); got.SurplusMbar < 0 || !got.PumpAdded {
		t.Errorf("pump curve must lift weak supply in direct comparison: %+v", got)
	}

	clk := clock.NewFake(parseTime("2026-01-15T09:00:00Z"))
	dbPath := filepath.Join(t.TempDir(), "pump-recovery.db")
	srv1, st1, err := restartServer(dbPath, clk)
	if err != nil {
		t.Fatal(err)
	}
	_, sid, _, _, err := buildSimpleTree(srv1, model.HazardExtra2, 95, 23200, 50)
	if err != nil {
		t.Fatal(err)
	}
	if err := mustDo(srv1, "POST", "/api/systems/"+sid+"/water-supply", map[string]any{"kind": "city", "static_pressure_mbar": 700, "points": []map[string]any{{"flow_lpm": 200, "residual_pressure_mbar": 400}, {"flow_lpm": 500, "residual_pressure_mbar": 100}}}, nil); err != nil {
		t.Fatal(err)
	}
	if err := addFirePump(srv1, sid); err != nil {
		t.Fatal(err)
	}
	if _, cmp, err := callCalc(srv1, sid); err != nil || cmp == nil || cmp.SurplusMbar < 0 || !cmp.PumpAdded {
		t.Errorf("live calculation must retain pump contribution, cmp=%+v err=%v", cmp, err)
	}
	srv1.Close()
	if err := st1.Close(); err != nil {
		t.Fatal(err)
	}

	srv2, st2, err := restartServer(dbPath, clk)
	if err != nil {
		t.Fatal(err)
	}
	defer srv2.Close()
	defer st2.Close()
	if _, err := service.NewWithClock(st2, clk).Reconcile().ReconcileAll(ctx()); err != nil {
		t.Fatal(err)
	}
	var recovered model.SupplyComparison
	code, body, err := doJSON(srv2, "GET", "/api/systems/"+sid+"/supply-comparison", nil)
	if err != nil || code != 200 || decode(body, &recovered) != nil || recovered.SurplusMbar < 0 || !recovered.PumpAdded {
		t.Errorf("restart recovery must retain pump contribution: code=%d comparison=%+v err=%v", code, recovered, err)
	}
}

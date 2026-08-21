package selfcheck

import (
	"path/filepath"
	"testing"

	"task141-firehydraulics/internal/clock"
	"task141-firehydraulics/internal/model"
	"task141-firehydraulics/internal/service"
)

func TestBug02_ReplacingSupplyInvalidatesLiveAndRecoveredComparison(t *testing.T) {
	clk := clock.NewFake(parseTime("2026-01-15T09:00:00Z"))
	dbPath := filepath.Join(t.TempDir(), "replacement.db")
	srv1, st1, err := restartServer(dbPath, clk)
	if err != nil { t.Fatal(err) }
	_, sid, _, _, err := buildSimpleTree(srv1, model.HazardExtra2, 95, 23200, 50)
	if err != nil { t.Fatal(err) }
	if err := addAdequateSupply(srv1, sid); err != nil { t.Fatal(err) }
	if _, cmp, err := callCalc(srv1, sid); err != nil || cmp == nil || cmp.SurplusMbar < 0 {
		t.Fatalf("initial supply must be adequate: cmp=%+v err=%v", cmp, err)
	}
	weak := map[string]any{"kind": "city", "static_pressure_mbar": 700, "points": []map[string]any{{"flow_lpm": 200, "residual_pressure_mbar": 400}, {"flow_lpm": 500, "residual_pressure_mbar": 100}}}
	if err := mustDo(srv1, "POST", "/api/systems/"+sid+"/water-supply", weak, nil); err != nil { t.Fatal(err) }
	if code, _, err := doJSON(srv1, "GET", "/api/systems/"+sid+"/supply-comparison", nil); err != nil || code != 404 {
		t.Errorf("replaced supply must not expose the prior comparison before recalculation: code=%d err=%v", code, err)
	}
	srv1.Close()
	if err := st1.Close(); err != nil { t.Fatal(err) }

	srv2, st2, err := restartServer(dbPath, clk)
	if err != nil { t.Fatal(err) }
	defer srv2.Close()
	defer st2.Close()
	if _, err := service.NewWithClock(st2, clk).Reconcile().ReconcileAll(ctx()); err != nil { t.Fatal(err) }
	var recovered model.SupplyComparison
	code, body, err := doJSON(srv2, "GET", "/api/systems/"+sid+"/supply-comparison", nil)
	if err != nil || code != 200 || decode(body, &recovered) != nil || recovered.SurplusMbar >= 0 || recovered.AvailablePressure >= recovered.RequiredPressure {
		t.Errorf("restart must rebuild the comparison from the weak replacement supply: code=%d comparison=%+v err=%v", code, recovered, err)
	}
}

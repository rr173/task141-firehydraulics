package selfcheck

import (
	"path/filepath"
	"testing"

	"task141-firehydraulics/internal/clock"
	"task141-firehydraulics/internal/model"
	"task141-firehydraulics/internal/service"
)

func TestBug06_ReplacedPumpFeedsLiveAndRecoveredSupplyComparison(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "pump.db")
	clk := clock.NewFake(parseTime("2026-02-04T08:00:00Z"))
	srv, st, err := restartServer(dbPath, clk)
	if err != nil { t.Fatal(err) }
	_, sid, _, _, err := buildSimpleTree(srv, model.HazardExtra2, 95, 23200, 50)
	if err != nil { t.Fatal(err) }
	if err := mustDo(srv, "POST", "/api/systems/"+sid+"/water-supply", map[string]any{"kind":"city", "static_pressure_mbar":700, "points":[]map[string]any{{"flow_lpm":200,"residual_pressure_mbar":400},{"flow_lpm":500,"residual_pressure_mbar":100}}}, nil); err != nil { t.Fatal(err) }
	pump := map[string]any{"rated_flow_lpm":1000,"rated_head_mbar":6200,"churn_pressure_mbar":7000,"hundred_fifty_flow_lpm":1500,"hundred_fifty_head_mbar":5000,"driver_type":"electric","rated_rpm":2900}
	if err := mustDo(srv, "POST", "/api/systems/"+sid+"/pump", pump, nil); err != nil { t.Fatal(err) }
	if _, cmp, err := callCalc(srv, sid); err != nil || cmp.SurplusMbar < 0 { t.Fatalf("initial pump should make supply adequate: cmp=%#v err=%v", cmp, err) }
	pump["rated_head_mbar"] = 6500
	if err := mustDo(srv, "POST", "/api/systems/"+sid+"/pump", pump, nil); err != nil { t.Fatal(err) }
	if _, cmp, err := callCalc(srv, sid); err != nil || cmp.SurplusMbar < 0 || !cmp.PumpAdded { t.Fatalf("replacement pump was not used live: cmp=%#v err=%v", cmp, err) }
	srv.Close(); if err := st.Close(); err != nil { t.Fatal(err) }
	restarted, st2, err := restartServer(dbPath, clk); if err != nil { t.Fatal(err) }
	defer restarted.Close(); defer st2.Close()
	if _, err := service.NewWithClock(st2, clk).Reconcile().ReconcileAll(ctx()); err != nil { t.Fatal(err) }
	var recovered model.SupplyComparison
	if err := mustDo(restarted, "GET", "/api/systems/"+sid+"/supply-comparison", nil, &recovered); err != nil || recovered.SurplusMbar < 0 || !recovered.PumpAdded { t.Fatalf("replacement pump was not used after recovery: cmp=%#v err=%v", recovered, err) }
}

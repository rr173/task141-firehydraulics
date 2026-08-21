package selfcheck

import (
	"testing"

	"task141-firehydraulics/internal/clock"
	"task141-firehydraulics/internal/model"
	"task141-firehydraulics/internal/service"
)

func TestBug04_RestoredImpairmentClosesMeasuresForManualAndAutoPaths(t *testing.T) {
	clk := newProbeClock()
	srv, st, err := restartServer(t.TempDir()+"/impairment.db", clk)
	if err != nil { t.Fatal(err) }
	defer srv.Close(); defer st.Close()
	sid, err := bringToInService(srv, model.HazardOrdinary1, "manual")
	if err != nil { t.Fatal(err) }
	start := clk.Epoch()
	var manual model.Impairment
	if err := mustDo(srv, "POST", "/api/systems/"+sid+"/impairments", map[string]any{"scope":"zone-a","reason":"manual","started_epoch":start,"expected_restore_epoch":start+60,"measures":[]map[string]any{{"kind":"patrol","owner":"guard"}}}, &manual); err != nil { t.Fatal(err) }
	clk.Advance(60 * 1e9)
	var restored model.Impairment
	if err := mustDo(srv, "POST", "/api/impairments/"+manual.ID+"/restore", nil, &restored); err != nil { t.Fatal(err) }
	if restored.ActualRestoreEpoch != clk.Epoch() || len(restored.Measures) != 1 || restored.Measures[0].EndedEpoch != clk.Epoch() {
		t.Errorf("manual restore must close its measure at the restore time: %+v", restored)
	}

	sid2, err := bringToInService(srv, model.HazardOrdinary1, "auto")
	if err != nil { t.Fatal(err) }
	start2 := clk.Epoch()
	if _, err := service.NewWithClock(st, clk).CreateImpairment(ctx(), sid2, "zone-b", "auto", start2, start2+60, []model.CompensatingMeasure{{Kind:model.CompPatrol, Owner:"guard"}}); err != nil { t.Fatal(err) }
	clk.Advance(60 * 1e9)
	if _, err := service.NewWithClock(st, clk).Reconcile().ReconcileAll(ctx()); err != nil { t.Fatal(err) }
	rows, err := st.ListImpairmentsBySystem(ctx(), sid2)
	if err != nil || len(rows) != 1 || rows[0].Status != model.ImpairmentRestored || len(rows[0].Measures) != 1 || rows[0].Measures[0].EndedEpoch != clk.Epoch() {
		t.Errorf("auto restore must close its measure and mark the record restored: rows=%+v err=%v", rows, err)
	}
}

func newProbeClock() *clock.Fake { return clock.NewFake(parseTime("2026-01-15T09:00:00Z")) }

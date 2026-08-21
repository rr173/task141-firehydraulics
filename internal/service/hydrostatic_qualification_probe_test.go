package service

import (
	"context"
	"path/filepath"
	"testing"
	"time"

	"task141-firehydraulics/internal/clock"
	"task141-firehydraulics/internal/model"
	"task141-firehydraulics/internal/store"
)

func TestBug03_HydrostaticQualificationSurvivesRecordReadAndGate(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(filepath.Join(t.TempDir(), "hydro.db"))
	if err != nil { t.Fatal(err) }
	defer st.Close()
	svc := NewWithClock(st, clock.NewFake(time.Unix(0, 0).UTC()))
	p, err := svc.CreateProject(ctx, "hydro project", model.HazardOrdinary1, 0)
	if err != nil { t.Fatal(err) }
	sys, err := svc.CreateSystem(ctx, p.ID, model.KindSprinkler, "hydro system", 0, 43, 13900, 100)
	if err != nil { t.Fatal(err) }
	if got, err := svc.RecordHydrostaticTest(ctx, sys.ID, 1500, 3600, false, 1); err != nil || got.Passed {
		t.Errorf("under-pressure and short clean test must not qualify: test=%+v err=%v", got, err)
	}
	manual := &model.HydrostaticTest{ID: "manual-low", SystemID: sys.ID, TestPressureMbar: 1500, HoldSeconds: 3600, Leaked: false, Passed: false, TestEpoch: 2}
	if err := st.CreateHydrostaticTest(ctx, nil, manual); err != nil { t.Fatal(err) }
	rows, err := st.ListHydrostaticTests(ctx, sys.ID)
	if err != nil { t.Fatal(err) }
	for _, row := range rows {
		if row.TestPressureMbar == 1500 && row.Passed {
			t.Errorf("stored non-qualifying test must remain failed after read: %+v", row)
		}
	}
	if anyHydrostaticPassed([]model.HydrostaticTest{{TestPressureMbar: 1500, HoldSeconds: 3600, Leaked: false, Passed: false}}) {
		t.Error("acceptance gate must reject a clean but unqualified hydrostatic test")
	}
}

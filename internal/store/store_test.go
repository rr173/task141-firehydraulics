package store

import (
	"context"
	"path/filepath"
	"testing"

	"task141-firehydraulics/internal/model"
)

func newTestStore(t *testing.T) *Store {
	t.Helper()
	dir := t.TempDir()
	st, err := Open(filepath.Join(dir, "test.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	return st
}

func TestProjectCRUD(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	p := &model.Project{ID: "prj_x", Name: "p1", HazardClass: model.HazardLight, DesignDate: 100, CreatedAt: 1}
	if err := st.CreateProject(ctx, nil, p); err != nil {
		t.Fatalf("create: %v", err)
	}
	got, err := st.GetProject(ctx, "prj_x")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if got.Name != "p1" || got.HazardClass != model.HazardLight {
		t.Errorf("get mismatch: %+v", got)
	}
	if _, err := st.GetProject(ctx, "missing"); err != ErrNotFound {
		t.Errorf("missing project: want ErrNotFound got %v", err)
	}
}

func TestSystemLifecycleEvents(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	st.CreateProject(ctx, nil, &model.Project{ID: "prj_l", Name: "p", HazardClass: model.HazardLight, DesignDate: 0, CreatedAt: 1})
	sys := &model.System{
		ID: "sys_l", ProjectID: "prj_l", Kind: model.KindSprinkler, Name: "s",
		BaseElevationMM: 0, DesignAreaDM2: 13900, DesignDensity: 21, PerHeadCoverageDM2: 200,
		State: model.StateDraft, UpdatedAt: 1,
	}
	if err := st.CreateSystem(ctx, nil, sys); err != nil {
		t.Fatalf("create system: %v", err)
	}
	ev := &model.LifecycleEvent{ID: "evt_1", SystemID: "sys_l", FromState: model.StateDraft, ToState: model.StateDesigned, Reason: "calc", EventEpoch: 2}
	if err := st.AppendLifecycleEvent(ctx, nil, ev); err != nil {
		t.Fatalf("append event: %v", err)
	}
	if err := st.UpdateSystemState(ctx, nil, "sys_l", model.StateDesigned, 2); err != nil {
		t.Fatalf("update state: %v", err)
	}
	got, err := st.GetSystem(ctx, "sys_l")
	if err != nil {
		t.Fatalf("get system: %v", err)
	}
	if got.State != model.StateDesigned {
		t.Errorf("state=%s want designed", got.State)
	}
	if got.PerHeadCoverageDM2 != 200 {
		t.Errorf("per-head coverage=%d want 200", got.PerHeadCoverageDM2)
	}
	events, err := st.ListLifecycleEvents(ctx, "sys_l")
	if err != nil {
		t.Fatalf("list events: %v", err)
	}
	if len(events) != 1 || events[0].ToState != model.StateDesigned {
		t.Errorf("events mismatch: %+v", events)
	}
}

func TestImpairmentCompensation(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	st.CreateProject(ctx, nil, &model.Project{ID: "prj_i", Name: "p", HazardClass: model.HazardLight, DesignDate: 0, CreatedAt: 1})
	st.CreateSystem(ctx, nil, &model.System{
		ID: "sys_i", ProjectID: "prj_i", Kind: model.KindSprinkler, Name: "s",
		BaseElevationMM: 0, DesignAreaDM2: 13900, DesignDensity: 21, PerHeadCoverageDM2: 200,
		State: model.StateInService, UpdatedAt: 1,
	})
	im := &model.Impairment{ID: "imp_1", SystemID: "sys_i", Scope: "z", Reason: "r",
		StartedEpoch: 10, ExpectedRestoreEpoch: 100, Status: model.ImpairmentActive}
	if err := st.CreateImpairment(ctx, nil, im); err != nil {
		t.Fatalf("create impairment: %v", err)
	}
	m := &model.CompensatingMeasure{ID: "msh_1", ImpairmentID: "imp_1", Kind: model.CompPatrol, Owner: "g", StartedEpoch: 10}
	if err := st.CreateCompensatingMeasure(ctx, nil, m); err != nil {
		t.Fatalf("create measure: %v", err)
	}
	list, err := st.ListImpairmentsBySystem(ctx, "sys_i")
	if err != nil {
		t.Fatalf("list: %v", err)
	}
	if len(list) != 1 || len(list[0].Measures) != 1 {
		t.Fatalf("impairment/measures mismatch: %+v", list)
	}
	if err := st.RestoreImpairment(ctx, nil, "imp_1", 200); err != nil {
		t.Fatalf("restore: %v", err)
	}
	got, _ := st.GetImpairment(ctx, "imp_1")
	if got.Status != model.ImpairmentRestored || got.ActualRestoreEpoch != 200 {
		t.Errorf("restore not persisted: %+v", got)
	}
}

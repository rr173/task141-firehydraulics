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

func TestHydraulicResultPreservesFullNodeTable(t *testing.T) {
	st := newTestStore(t)
	ctx := context.Background()
	st.CreateProject(ctx, nil, &model.Project{ID: "prj_h", Name: "p", HazardClass: model.HazardLight, DesignDate: 0, CreatedAt: 1})
	st.CreateSystem(ctx, nil, &model.System{
		ID: "sys_h", ProjectID: "prj_h", Kind: model.KindSprinkler, Name: "s",
		BaseElevationMM: 0, DesignAreaDM2: 13900, DesignDensity: 21, PerHeadCoverageDM2: 200,
		State: model.StateDraft, UpdatedAt: 1,
	})
	// A representative tree-shaped result: source, a junction, two sprinklers
	// (one of them the most-unfavourable remote head), and a drain. Every node
	// must survive the round-trip so the remote sprinkler stays traceable.
	full := &model.HydraulicResult{
		ID:                   "hyd_1",
		SystemID:             "sys_h",
		BaseFlowLPM:          200,
		BaseRequiredPressure: 5000,
		RemotePressureMbar:   500,
		RemoteFlowLPM:        80,
		CalcEpoch:            9,
		Nodes: []model.NodeResult{
			{NodeID: "src", Label: "水源", PressureMbar: 5000, ElevationMM: 0},
			{NodeID: "jct", Label: "三通", PressureMbar: 4000, ElevationMM: 500},
			{NodeID: "sp_a", Label: "喷头A(最不利)", PressureMbar: 500, FlowLPM: 80, ElevationMM: 3000},
			{NodeID: "sp_b", Label: "喷头B", PressureMbar: 620, FlowLPM: 89, ElevationMM: 3000},
			{NodeID: "drn", Label: "试验接口", PressureMbar: 0, ElevationMM: 0},
		},
	}
	if err := st.UpsertHydraulicResult(ctx, nil, full); err != nil {
		t.Fatalf("upsert: %v", err)
	}
	got, err := st.GetHydraulicResult(ctx, "sys_h")
	if err != nil {
		t.Fatalf("get: %v", err)
	}
	if len(got.Nodes) != len(full.Nodes) {
		t.Fatalf("node table truncated: got %d nodes, want %d (only the source was kept before the fix)",
			len(got.Nodes), len(full.Nodes))
	}
	// The most-unfavourable remote sprinkler must be recoverable by id.
	remoteByID := map[string]model.NodeResult{}
	for _, nr := range got.Nodes {
		remoteByID[nr.NodeID] = nr
	}
	remote, ok := remoteByID["sp_a"]
	if !ok {
		t.Fatalf("remote sprinkler sp_a missing from persisted node table: %+v", got.Nodes)
	}
	if remote.FlowLPM != 80 {
		t.Errorf("remote flow=%d want 80", remote.FlowLPM)
	}
	// Junction + the second branch sprinkler must survive too.
	if _, ok := remoteByID["jct"]; !ok {
		t.Errorf("junction node jct missing from persisted table")
	}
	if _, ok := remoteByID["sp_b"]; !ok {
		t.Errorf("branch sprinkler sp_b missing from persisted table")
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

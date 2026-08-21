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

func clk() clock.Clock { return clock.NewFake(mustTime("2026-01-15T09:00:00Z")) }

func mustTime(s string) time.Time {
	t, err := time.Parse(time.RFC3339, s)
	if err != nil {
		panic(err)
	}
	return t
}

// TestFullReportDropsLastLifecycleEvent_BUG reproduces BUG 1: FullReport
// truncates the lifecycle slice, dropping the most recent state change. A
// system driven to in_service has its final transition (accepted->in_service)
// missing from the report.
func TestFullReportDropsLastLifecycleEvent_BUG(t *testing.T) {
	ctx := context.Background()
	st, err := store.Open(filepath.Join(t.TempDir(), "t.db"))
	if err != nil {
		t.Fatal(err)
	}
	defer st.Close()
	svc := NewWithClock(st, clk())
	sid := buildTreeAndDriveToInService(t, ctx, svc)

	rep, err := svc.FullReport(ctx, sid)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("lifecycle events in report: %d", len(rep.Lifecycle))
	for i, e := range rep.Lifecycle {
		t.Logf("  [%d] %s -> %s (%s)", i, e.FromState, e.ToState, e.Reason)
	}
	last := rep.Lifecycle[len(rep.Lifecycle)-1]
	if last.ToState != model.StateInService {
		t.Fatalf("BUG 1: report dropped the final transition; last event to=%s, want in_service", last.ToState)
	}
}

// TestRestartReplaysToOldState_BUG reproduces BUG 2: after a restart, the
// persisted system state may replay to an older state than the final one
// because the reconcile step picks the wrong "last" event when events share
// an epoch (the fake clock does not advance between transitions).
func TestRestartReplaysToOldState_BUG(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "t.db")
	st1, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	svc1 := NewWithClock(st1, clk())
	sid := buildTreeAndDriveToInService(t, ctx, svc1)
	preSys, err := svc1.GetSystem(ctx, sid)
	if err != nil {
		t.Fatal(err)
	}
	if preSys.State != model.StateInService {
		t.Fatalf("pre-restart state: want in_service got %s", preSys.State)
	}
	if err := st1.Close(); err != nil {
		t.Fatal(err)
	}

	st2, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer st2.Close()
	svc2 := NewWithClock(st2, clk())
	if _, err := svc2.Reconcile().ReconcileAll(ctx); err != nil {
		t.Fatal(err)
	}
	postSys, err := svc2.GetSystem(ctx, sid)
	if err != nil {
		t.Fatal(err)
	}
	if postSys.State != preSys.State {
		t.Fatalf("BUG 2: state changed across restart: pre=%s post=%s", preSys.State, postSys.State)
	}
}

// buildTreeAndDriveToInService creates a project+system+network+supply, runs
// calc+compliance, and drives the lifecycle to in_service using the service
// layer directly. It returns the system id.
func buildTreeAndDriveToInService(t *testing.T, ctx context.Context, svc *Services) string {
	t.Helper()
	proj, err := svc.CreateProject(ctx, "P", model.HazardOrdinary1, 0)
	if err != nil {
		t.Fatal(err)
	}
	sys, err := svc.CreateSystem(ctx, proj.ID, model.KindSprinkler, "S", 0, 43, 13900, 100)
	if err != nil {
		t.Fatal(err)
	}
	src := mustCreateNode(t, ctx, svc, sys.ID, model.NodeSource, "src", 0, 0, 0, 0)
	mustCreateNode(t, ctx, svc, sys.ID, model.NodeDrain, "drain", 1, 0, 0, 0)
	jct := mustCreateNode(t, ctx, svc, sys.ID, model.NodeJunction, "jct", 2, 500, 0, 0)
	sp1 := mustCreateNode(t, ctx, svc, sys.ID, model.NodeSprinkler, "sp1", 3, 3000, 80, 500)
	sp2 := mustCreateNode(t, ctx, svc, sys.ID, model.NodeSprinkler, "sp2", 4, 3000, 80, 500)
	mustCreatePipe(t, ctx, svc, sys.ID, src, jct, 0)
	mustCreatePipe(t, ctx, svc, sys.ID, jct, sp1, 1)
	mustCreatePipe(t, ctx, svc, sys.ID, jct, sp2, 2)
	if _, err := svc.SetRemoteNode(ctx, sys.ID, sp1); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.CreateWaterSupply(ctx, sys.ID, model.SupplyCity, 6000, []model.SupplyPoint{
		{FlowLPM: 500, Pressure: 5500, Seq: 0},
		{FlowLPM: 1000, Pressure: 5000, Seq: 1},
		{FlowLPM: 2000, Pressure: 4000, Seq: 2},
	}); err != nil {
		t.Fatal(err)
	}
	if _, _, err := svc.Calculate(ctx, sys.ID); err != nil {
		t.Fatal(err)
	}
	if _, err := svc.RunCompliance(ctx, sys.ID); err != nil {
		t.Fatal(err)
	}
	transitions := []struct {
		to     model.SystemState
		reason string
	}{
		{model.StateSubmitted, "submit"},
		{model.StateApproved, "approve"},
		{model.StateInstalled, "install"},
		{model.StateHydrostatic, "hydro"},
		{model.StateAccepted, "accept"},
		{model.StateInService, "in service"},
	}
	for _, tr := range transitions {
		if tr.to == model.StateHydrostatic {
			if _, err := svc.RecordHydrostaticTest(ctx, sys.ID, 34000, 7200, false, 0); err != nil {
				t.Fatal(err)
			}
		}
		if tr.to == model.StateAccepted {
			if _, err := svc.RecordAcceptance(ctx, sys.ID, "pass", "AHJ", nil); err != nil {
				t.Fatal(err)
			}
		}
		if _, _, err := svc.Transition(ctx, sys.ID, tr.to, tr.reason); err != nil {
			t.Fatalf("transition to %s: %v", tr.to, err)
		}
	}
	return sys.ID
}

func mustCreateNode(t *testing.T, ctx context.Context, svc *Services, sysID string, typ model.NodeType, label string, seq, elevMM, k, minP int64) string {
	t.Helper()
	n, err := svc.CreateNode(ctx, sysID, typ, label, elevMM, k, minP, seq)
	if err != nil {
		t.Fatal(err)
	}
	return n.ID
}

func mustCreatePipe(t *testing.T, ctx context.Context, svc *Services, sysID, up, dn string, seq int64) {
	t.Helper()
	if _, err := svc.CreatePipe(ctx, sysID, up, dn, 80, 78, 6000, 150, 0, seq); err != nil {
		t.Fatal(err)
	}
}

// TestRestartRestoresImpairmentSequence_BUG checks that the full impairment
// round-trip (in_service -> impaired -> restored -> in_service) survives a
// restart and that the report retains the complete, in-order event history.
func TestRestartRestoresImpairmentSequence_BUG(t *testing.T) {
	ctx := context.Background()
	dbPath := filepath.Join(t.TempDir(), "t.db")
	st1, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	svc1 := NewWithClock(st1, clk())
	sid := buildTreeAndDriveToInService(t, ctx, svc1)

	// Register an impairment (with a compensating measure) then restore it.
	created, err := svc1.CreateImpairment(ctx, sid, "zone-A", "valve maintenance", 0, 86400,
		[]model.CompensatingMeasure{{Kind: model.CompPatrol, Owner: "g1"}})
	if err != nil {
		t.Fatal(err)
	}
	if _, err := svc1.RestoreImpairment(ctx, created.ID, 0); err != nil {
		t.Fatal(err)
	}
	rep, err := svc1.FullReport(ctx, sid)
	if err != nil {
		t.Fatal(err)
	}
	t.Logf("pre-restart lifecycle events: %d", len(rep.Lifecycle))
	for i, e := range rep.Lifecycle {
		t.Logf("  [%d] %s -> %s", i, e.FromState, e.ToState)
	}
	preSys, _ := svc1.GetSystem(ctx, sid)
	if preSys.State != model.StateInService {
		t.Fatalf("pre-restart state: want in_service got %s", preSys.State)
	}
	if err := st1.Close(); err != nil {
		t.Fatal(err)
	}

	st2, err := store.Open(dbPath)
	if err != nil {
		t.Fatal(err)
	}
	defer st2.Close()
	svc2 := NewWithClock(st2, clk())
	if _, err := svc2.Reconcile().ReconcileAll(ctx); err != nil {
		t.Fatal(err)
	}
	postSys, err := svc2.GetSystem(ctx, sid)
	if err != nil {
		t.Fatal(err)
	}
	if postSys.State != model.StateInService {
		t.Fatalf("BUG 2: state changed across restart: pre=in_service post=%s", postSys.State)
	}
	rep2, err := svc2.FullReport(ctx, sid)
	if err != nil {
		t.Fatal(err)
	}
	// The post-restart report must retain the FULL history, in order.
	last := rep2.Lifecycle[len(rep2.Lifecycle)-1]
	if last.ToState != model.StateInService {
		t.Fatalf("post-restart report last event to=%s, want in_service", last.ToState)
	}
	// And the full sequence must be preserved (every to_state from designed onward).
	want := []model.SystemState{
		model.StateSubmitted, model.StateApproved, model.StateInstalled,
		model.StateHydrostatic, model.StateAccepted, model.StateInService,
		model.StateImpaired, model.StateRestored, model.StateInService,
	}
	if len(rep2.Lifecycle) != len(want) {
		t.Fatalf("post-restart event count: got %d want %d", len(rep2.Lifecycle), len(want))
	}
	for i, w := range want {
		if rep2.Lifecycle[i].ToState != w {
			t.Fatalf("event %d: got to=%s want %s", i, rep2.Lifecycle[i].ToState, w)
		}
	}
}

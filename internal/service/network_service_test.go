package service

import (
	"context"
	"errors"
	"path/filepath"
	"testing"
	"time"

	"task141-firehydraulics/internal/clock"
	"task141-firehydraulics/internal/model"
	"task141-firehydraulics/internal/store"
)

// newTestServices builds a Services over a fresh in-memory SQLite file with a
// fake clock, so service-level invariants can be exercised without HTTP.
func newTestServices(t *testing.T) (*Services, context.Context) {
	t.Helper()
	st, err := store.Open(filepath.Join(t.TempDir(), "svc.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { _ = st.Close() })
	svc := NewWithClock(st, clock.NewFake(time.Unix(1, 0).UTC()))
	return svc, context.Background()
}

// seedTree builds a minimal valid sprinkler tree in a fresh system and returns
// the system id plus the junction + first-sprinkler node ids (the targets for
// a confluence attempt).
func seedTree(t *testing.T, svc *Services, ctx context.Context) (systemID, jctID, sp1ID, srcID string) {
	t.Helper()
	proj, err := svc.CreateProject(ctx, "p", model.HazardLight, 0)
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	sys, err := svc.CreateSystem(ctx, proj.ID, model.KindSprinkler, "s", 0, 21, 13900, 200)
	if err != nil {
		t.Fatalf("create system: %v", err)
	}
	systemID = sys.ID
	src, err := svc.CreateNode(ctx, systemID, model.NodeSource, "src", 0, 0, 0, 0)
	if err != nil {
		t.Fatalf("create source: %v", err)
	}
	srcID = src.ID
	jct, err := svc.CreateNode(ctx, systemID, model.NodeJunction, "jct", 500, 0, 0, 2)
	if err != nil {
		t.Fatalf("create junction: %v", err)
	}
	jctID = jct.ID
	sp1, err := svc.CreateNode(ctx, systemID, model.NodeSprinkler, "sp1", 3000, 80, 500, 3)
	if err != nil {
		t.Fatalf("create sprinkler1: %v", err)
	}
	sp1ID = sp1.ID
	if _, err := svc.CreateNode(ctx, systemID, model.NodeSprinkler, "sp2", 3000, 80, 500, 4); err != nil {
		t.Fatalf("create sprinkler2: %v", err)
	}
	if err := svc.createPipe(ctx, systemID, srcID, jctID); err != nil {
		t.Fatalf("create pipe src→jct: %v", err)
	}
	if err := svc.createPipe(ctx, systemID, jctID, sp1ID); err != nil {
		t.Fatalf("create pipe jct→sp1: %v", err)
	}
	return systemID, jctID, sp1ID, srcID
}

func (s *Services) createPipe(ctx context.Context, systemID, up, down string) error {
	_, err := s.CreatePipe(ctx, systemID, up, down, 40, 40, 3000, 150, 0, 0)
	return err
}

// TestCreatePipeRejectsConfluence verifies that once a node has an upstream
// parent, a second pipe feeding it from a different upstream branch is rejected
// at entry time. This is the录入 gate: a merged network must never be stored.
func TestCreatePipeRejectsConfluence(t *testing.T) {
	svc, ctx := newTestServices(t)
	sid, _, sp1ID, srcID := seedTree(t, svc, ctx)

	// Add a second junction off the source to serve as the second upstream branch.
	jct2, err := svc.CreateNode(ctx, sid, model.NodeJunction, "jct2", 500, 0, 0, 5)
	if err != nil {
		t.Fatalf("create jct2: %v", err)
	}
	if err := svc.createPipe(ctx, sid, srcID, jct2.ID); err != nil {
		t.Fatalf("create pipe src→jct2: %v", err)
	}

	// Now attempt the confluence: jct2 → sp1, but sp1 already has jct as parent.
	// This must be rejected (ErrConflict → 409 at the HTTP layer).
	err = svc.createPipe(ctx, sid, jct2.ID, sp1ID)
	if err == nil {
		t.Fatalf("confluence pipe must be rejected at entry, got nil error")
	}
	if !errors.Is(err, store.ErrConflict) {
		t.Fatalf("confluence pipe want ErrConflict, got %v", err)
	}
}

// TestCreatePipeRejectsReverseCycle verifies a pipe that reverses an existing
// parent→child relationship (creating a 2-cycle) is rejected at entry.
func TestCreatePipeRejectsReverseCycle(t *testing.T) {
	svc, ctx := newTestServices(t)
	sid, jctID, _, srcID := seedTree(t, svc, ctx)

	// src→jct exists; adding jct→src reverses it → cycle, rejected.
	err := svc.createPipe(ctx, sid, jctID, srcID)
	if err == nil {
		t.Fatalf("reverse-cycle pipe must be rejected at entry, got nil")
	}
	if !errors.Is(err, store.ErrConflict) {
		t.Fatalf("reverse-cycle pipe want ErrConflict, got %v", err)
	}
}

// seedConfluenceDirectly bypasses the CreatePipe entry gate and writes a
// confluence network (one sprinkler fed by two upstream branches) straight to
// the store. This simulates a legacy/corrupted DB so the subsequent 校核 paths
// (Calculate, RunCompliance) can be proven to reject it rather than accept it.
func seedConfluenceDirectly(t *testing.T, svc *Services, ctx context.Context) (systemID, remoteID string) {
	t.Helper()
	proj, err := svc.CreateProject(ctx, "p", model.HazardLight, 0)
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	sys, err := svc.CreateSystem(ctx, proj.ID, model.KindSprinkler, "s", 0, 21, 13900, 200)
	if err != nil {
		t.Fatalf("create system: %v", err)
	}
	systemID = sys.ID
	// Nodes: source, two junctions, and one sprinkler fed by BOTH junctions.
	mkNode := func(nt model.NodeType, label string, seq int64, k int64) *model.Node {
		n := &model.Node{ID: label, SystemID: systemID, Type: nt, Label: label, Seq: seq, KFactor: k}
		if err := svc.st.CreateNode(ctx, nil, n); err != nil {
			t.Fatalf("create node %s: %v", label, err)
		}
		return n
	}
	src := mkNode(model.NodeSource, "src", 0, 0)
	j1 := mkNode(model.NodeJunction, "j1", 1, 0)
	j2 := mkNode(model.NodeJunction, "j2", 2, 0)
	sp := mkNode(model.NodeSprinkler, "sp", 3, 80)
	sp.DesignMinPressure = 500
	remoteID = sp.ID
	// Pipes: src→j1, src→j2, j1→sp, j2→sp (sp has two parents → confluence).
	mkPipe := func(id, up, down string) {
		p := &model.PipeSegment{ID: id, SystemID: systemID, UpstreamNodeID: up, DownstreamNodeID: down,
			NominalDiaMM: 40, InnerDiaMM: 40, LengthMM: 3000, CFactor: 150}
		if err := svc.st.CreatePipe(ctx, nil, p); err != nil {
			t.Fatalf("create pipe %s: %v", id, err)
		}
	}
	mkPipe("p1", src.ID, j1.ID)
	mkPipe("p2", src.ID, j2.ID)
	mkPipe("p3", j1.ID, sp.ID)
	mkPipe("p4", j2.ID, sp.ID) // second feeder → confluence
	// Point the remote node so Calculate proceeds to the build step.
	if _, err := svc.SetRemoteNode(ctx, systemID, remoteID); err != nil {
		t.Fatalf("set remote: %v", err)
	}
	return systemID, remoteID
}

// TestCalculateRejectsConfluence proves the subsequent-校核 path: even if a
// confluence network is already in the store (bypassing the entry gate),
// Calculate refuses to build/compute it rather than producing distorted flow.
func TestCalculateRejectsConfluence(t *testing.T) {
	svc, ctx := newTestServices(t)
	sid, _ := seedConfluenceDirectly(t, svc, ctx)

	_, _, err := svc.Calculate(ctx, sid)
	if err == nil {
		t.Fatalf("Calculate must reject a confluence network, got nil")
	}
	if !errors.Is(err, store.ErrNetworkCycle) {
		t.Fatalf("Calculate want ErrNetworkCycle for confluence, got %v", err)
	}
}

// TestRunComplianceRejectsConfluence proves the compliance 校核 path: a
// confluence network's tree-integrity rule fails (and the run does not panic),
// so the design cannot be treated as compliant.
func TestRunComplianceRejectsConfluence(t *testing.T) {
	svc, ctx := newTestServices(t)
	sid, _ := seedConfluenceDirectly(t, svc, ctx)

	checks, err := svc.RunCompliance(ctx, sid)
	if err != nil {
		t.Fatalf("RunCompliance on confluence should not error, got %v", err)
	}
	var treeCheck *model.ComplianceCheck
	for i := range checks {
		if checks[i].RuleCode == "R-tree-integrity" {
			treeCheck = &checks[i]
		}
	}
	if treeCheck == nil {
		t.Fatalf("no R-tree-integrity check emitted")
	}
	if treeCheck.Passed {
		t.Fatalf("R-tree-integrity must FAIL for a confluence network, got pass: %s", treeCheck.Detail)
	}
}

// TestReconcileSkipsConfluence proves the restart-校核 path: ReconcileAll does
// not panic or write a distorted hydraulic result for a confluence network; it
// logs and skips the system (which retains no hydraulic result).
func TestReconcileSkipsConfluence(t *testing.T) {
	svc, ctx := newTestServices(t)
	sid, _ := seedConfluenceDirectly(t, svc, ctx)
	// Advance the system out of draft so reconcile considers it.
	if err := svc.st.UpdateSystemState(ctx, nil, sid, model.StateDesigned, svc.now()); err != nil {
		t.Fatalf("advance state: %v", err)
	}

	n, err := svc.Reconcile().ReconcileAll(ctx)
	if err != nil {
		t.Fatalf("ReconcileAll should not hard-fail on a confluence system: %v", err)
	}
	// The confluence system is skipped (count excludes it), and no hydraulic
	// result is written for it.
	_ = n
	hyd, err := svc.st.GetHydraulicResult(ctx, sid)
	if err != nil && !errors.Is(err, store.ErrNotFound) {
		t.Fatalf("get hydraulic: %v", err)
	}
	if hyd != nil {
		t.Fatalf("confluence system must not have a hydraulic result after reconcile, got %+v", hyd)
	}
}

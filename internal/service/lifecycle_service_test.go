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

// newSvcTestStore opens a fresh SQLite file for a service-level test and wraps
// it in a Services driven by a fixed fake clock, mirroring the selfcheck setup.
func newSvcTestStore(t *testing.T) (*Services, *clock.Fake) {
	t.Helper()
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "svc.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	base, _ := time.Parse(time.RFC3339, "2026-01-15T09:00:00Z")
	clk := clock.NewFake(base)
	return NewWithClock(st, clk), clk
}

// seedAcceptedSystem builds the minimum state to reach the "accepted"
// lifecycle state: a project + system with a hydraulic result and a passing
// hydrostatic test. Returns the system id.
func seedAcceptedSystem(t *testing.T, s *Services) string {
	t.Helper()
	ctx := context.Background()
	proj, err := s.CreateProject(ctx, "p", model.HazardLight, 0)
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	// A hydraulic result must exist before →designed/→submitted. Insert a
	// minimal one directly through the store (the calculation itself is
	// exercised elsewhere; here we only need the precondition satisfied).
	sid := "sys_acc"
	if err := s.st.CreateSystem(ctx, nil, &model.System{
		ID: sid, ProjectID: proj.ID, Kind: model.KindSprinkler, Name: "s",
		BaseElevationMM: 0, DesignAreaDM2: 13900, DesignDensity: 21, PerHeadCoverageDM2: 200,
		State: model.StateDraft, UpdatedAt: s.now(),
	}); err != nil {
		t.Fatalf("create system: %v", err)
	}
	// Stub a hydraulic result so the →designed gate passes.
	if err := s.st.UpsertHydraulicResult(ctx, nil, &model.HydraulicResult{
		ID: "hyd_1", SystemID: sid, BaseFlowLPM: 100, BaseRequiredPressure: 500,
		RemotePressureMbar: 500, RemoteFlowLPM: 50, CalcEpoch: s.now(),
	}); err != nil {
		t.Fatalf("upsert hydraulic result: %v", err)
	}
	// Drive to accepted through the legal transitions.
	for _, to := range []model.SystemState{
		model.StateDesigned, model.StateSubmitted, model.StateApproved,
		model.StateInstalled, model.StateHydrostatic,
	} {
		if _, _, err := s.Transition(ctx, sid, to, "step"); err != nil {
			t.Fatalf("transition to %s: %v", to, err)
		}
	}
	// Record a passing hydrostatic test (required by the →accepted gate).
	if _, err := s.RecordHydrostaticTest(ctx, sid, 34000, 7200, false, 0); err != nil {
		t.Fatalf("record hydrostatic test: %v", err)
	}
	if _, _, err := s.Transition(ctx, sid, model.StateAccepted, "accepted"); err != nil {
		t.Fatalf("transition to accepted: %v", err)
	}
	return sid
}

// TestAcceptanceDefectsPersist verifies that defects recorded on an acceptance
// record survive a read-back — the store must not silently null them out.
func TestAcceptanceDefectsPersist(t *testing.T) {
	s, _ := newSvcTestStore(t)
	ctx := context.Background()
	sid := seedAcceptedSystem(t, s)

	want := []string{"最不利点压力不足", "喷头间距超限", "排水阀未设"}
	if _, err := s.RecordAcceptance(ctx, sid, "pass", "AHJ", want); err != nil {
		t.Fatalf("record acceptance: %v", err)
	}

	got, err := s.GetAcceptanceRecord(ctx, sid)
	if err != nil {
		t.Fatalf("get acceptance record: %v", err)
	}
	if len(got.Defects) != len(want) {
		t.Fatalf("defects not preserved: got %v want %v", got.Defects, want)
	}
	for i, d := range want {
		if got.Defects[i] != d {
			t.Errorf("defect[%d]: got %q want %q", i, got.Defects[i], d)
		}
	}
}

// TestAcceptanceWithDefectsBlocksInService verifies that a passing acceptance
// record that still carries unrectified defects cannot be released to
// in_service, and that clearing the defects (rewriting the record) then
// allows the transition.
func TestAcceptanceWithDefectsBlocksInService(t *testing.T) {
	s, _ := newSvcTestStore(t)
	ctx := context.Background()
	sid := seedAcceptedSystem(t, s)

	// Acceptance passes but with open defects.
	if _, err := s.RecordAcceptance(ctx, sid, "pass", "AHJ", []string{"缺陷A"}); err != nil {
		t.Fatalf("record acceptance with defects: %v", err)
	}
	if _, _, err := s.Transition(ctx, sid, model.StateInService, "in service"); err == nil {
		t.Fatal("expected in_service transition to be blocked by open defects, got nil")
	} else if !errors.Is(err, store.ErrInvariant) {
		t.Errorf("expected ErrInvariant wrapping the defect block, got %v", err)
	}
	// System must still be accepted, not in_service.
	sys, err := s.GetSystem(ctx, sid)
	if err != nil {
		t.Fatalf("get system: %v", err)
	}
	if sys.State != model.StateAccepted {
		t.Fatalf("state=%s want accepted (defects must not release to in_service)", sys.State)
	}

	// Rectify the defects by rewriting the acceptance record empty → now
	// in_service is allowed.
	if _, err := s.RecordAcceptance(ctx, sid, "pass", "AHJ", nil); err != nil {
		t.Fatalf("rewrite acceptance without defects: %v", err)
	}
	sys, _, err = s.Transition(ctx, sid, model.StateInService, "in service")
	if err != nil {
		t.Fatalf("in_service after clearing defects: %v", err)
	}
	if sys.State != model.StateInService {
		t.Errorf("state=%s want in_service", sys.State)
	}
}

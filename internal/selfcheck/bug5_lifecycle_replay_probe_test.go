package selfcheck

import (
	"path/filepath"
	"testing"

	"task141-firehydraulics/internal/clock"
	"task141-firehydraulics/internal/model"
	"task141-firehydraulics/internal/service"
)

func TestBug05_LifecycleTimelineAndRecoveryKeepTerminalState(t *testing.T) {
	dbPath := filepath.Join(t.TempDir(), "timeline.db")
	clk := clock.NewFake(parseTime("2026-02-03T08:00:00Z"))
	srv, st, err := restartServer(dbPath, clk)
	if err != nil {
		t.Fatal(err)
	}
	sid, err := bringToInService(srv, model.HazardOrdinary1, "timeline")
	if err != nil {
		t.Fatal(err)
	}
	var report model.FullReport
	if err := mustDo(srv, "GET", "/api/systems/"+sid+"/full-report", nil, &report); err != nil {
		t.Fatal(err)
	}
	if len(report.Lifecycle) < 6 || report.Lifecycle[len(report.Lifecycle)-1].ToState != model.StateInService {
		t.Fatalf("full report lost chronological terminal lifecycle event: %#v", report.Lifecycle)
	}
	srv.Close()
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	restarted, st2, err := restartServer(dbPath, clk)
	if err != nil {
		t.Fatal(err)
	}
	defer restarted.Close()
	defer st2.Close()
	if _, err := service.NewWithClock(st2, clk).Reconcile().ReconcileAll(ctx()); err != nil {
		t.Fatal(err)
	}
	sys, err := getSystem(restarted, sid)
	if err != nil {
		t.Fatal(err)
	}
	if sys.State != model.StateInService {
		t.Fatalf("restart replay restored %s, want %s", sys.State, model.StateInService)
	}
}

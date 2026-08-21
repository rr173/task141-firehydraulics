package service

import (
	"context"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"task141-firehydraulics/internal/clock"
	"task141-firehydraulics/internal/hazard"
	"task141-firehydraulics/internal/model"
	"task141-firehydraulics/internal/store"
)

// newTestService opens a fresh SQLite file and builds a Services over it with an
// injected fake clock, mirroring the selfcheck setup.
func newTestService(t *testing.T) (*Services, *store.Store) {
	t.Helper()
	dir := t.TempDir()
	st, err := store.Open(filepath.Join(dir, "svc.db"))
	if err != nil {
		t.Fatalf("open store: %v", err)
	}
	t.Cleanup(func() { st.Close() })
	clk := clock.NewFake(time.Date(2026, 1, 15, 9, 0, 0, 0, time.UTC))
	return NewWithClock(st, clk), st
}

// seedSystemWithNetwork builds a project + system + a small sprinkler tree +
// adequate supply and returns the system id, ready for Calculate/RunCompliance.
func seedSystemWithNetwork(t *testing.T, s *Services) string {
	t.Helper()
	ctx := context.Background()
	p, err := s.CreateProject(ctx, "p", model.HazardExtra2, 0)
	if err != nil {
		t.Fatalf("create project: %v", err)
	}
	hz, _ := hazard.Params(model.HazardExtra2)
	sys, err := s.CreateSystem(ctx, p.ID, model.KindSprinkler, "s", 0, hz.DensityMMPerMin, hz.DesignAreaDM2, 50)
	if err != nil {
		t.Fatalf("create system: %v", err)
	}
	// Source + drain + junction + two sprinklers, with pipes source→junction,
	// junction→sprinkler (×2).
	src, err := s.CreateNode(ctx, sys.ID, model.NodeSource, "src", 0, 0, 0, 0)
	if err != nil {
		t.Fatalf("create source: %v", err)
	}
	if _, err := s.CreateNode(ctx, sys.ID, model.NodeDrain, "drain", 0, 0, 0, 1); err != nil {
		t.Fatalf("create drain: %v", err)
	}
	jct, err := s.CreateNode(ctx, sys.ID, model.NodeJunction, "jct", 500, 0, 0, 2)
	if err != nil {
		t.Fatalf("create junction: %v", err)
	}
	sp1, err := s.CreateNode(ctx, sys.ID, model.NodeSprinkler, "sp1", 3000, 80, 500, 3)
	if err != nil {
		t.Fatalf("create sprinkler1: %v", err)
	}
	sp2, err := s.CreateNode(ctx, sys.ID, model.NodeSprinkler, "sp2", 3000, 80, 500, 4)
	if err != nil {
		t.Fatalf("create sprinkler2: %v", err)
	}
	mkPipe := func(up, down string, dia, lenMM int64) {
		if _, err := s.CreatePipe(ctx, sys.ID, up, down, dia, dia, lenMM, 150, 0, 0); err != nil {
			t.Fatalf("create pipe %s→%s: %v", up, down, err)
		}
	}
	mkPipe(src.ID, jct.ID, 80, 6000)
	mkPipe(jct.ID, sp1.ID, 40, 3000)
	mkPipe(jct.ID, sp2.ID, 40, 3000)
	if _, err := s.SetRemoteNode(ctx, sys.ID, sp1.ID); err != nil {
		t.Fatalf("set remote: %v", err)
	}
	// Adequate municipal supply.
	if _, err := s.CreateWaterSupply(ctx, sys.ID, model.SupplyCity, 6000, []model.SupplyPoint{
		{FlowLPM: 500, Pressure: 5500},
		{FlowLPM: 1000, Pressure: 5000},
		{FlowLPM: 2000, Pressure: 4000},
	}); err != nil {
		t.Fatalf("create supply: %v", err)
	}
	return sys.ID
}

// TestConcurrentCalcAndCompliance fires many concurrent Calculate and
// RunCompliance calls at the SAME system from multiple goroutines. Before the
// per-system mutex was actually locked, these interleaved writes to the derived
// tables (hydraulic_results, supply_comparisons, compliance_checks) and the
// RunCompliance reads of the hydraulic result could race the Calculate writes.
// Run with -race to confirm the lock eliminates the data race and that every
// call completes without error and the final derived rows are internally
// consistent.
func TestConcurrentCalcAndCompliance(t *testing.T) {
	s, _ := newTestService(t)
	sid := seedSystemWithNetwork(t, s)
	ctx := context.Background()

	const goroutines = 16
	var wg sync.WaitGroup
	errs := make(chan error, goroutines)
	for i := 0; i < goroutines; i++ {
		i := i
		wg.Add(1)
		go func() {
			defer wg.Done()
			if i%2 == 0 {
				_, _, err := s.Calculate(ctx, sid)
				errs <- err
			} else {
				_, err := s.RunCompliance(ctx, sid)
				errs <- err
			}
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent calc/compliance: %v", err)
		}
	}

	// After the storm, the persisted derived rows must be self-consistent:
	// the hydraulic result exists, the supply comparison references the same
	// base flow, and compliance checks exist.
	hyd, err := s.GetHydraulicResult(ctx, sid)
	if err != nil {
		t.Fatalf("get hydraulic: %v", err)
	}
	if hyd.BaseFlowLPM <= 0 {
		t.Fatalf("base flow not positive: %d", hyd.BaseFlowLPM)
	}
	cmp, err := s.GetSupplyComparison(ctx, sid)
	if err != nil {
		t.Fatalf("get comparison: %v", err)
	}
	if cmp.BaseFlowLPM != hyd.BaseFlowLPM {
		t.Fatalf("base flow mismatch: hyd=%d cmp=%d", hyd.BaseFlowLPM, cmp.BaseFlowLPM)
	}
	checks, err := s.ListComplianceChecks(ctx, sid)
	if err != nil {
		t.Fatalf("list compliance: %v", err)
	}
	if len(checks) == 0 {
		t.Fatalf("no compliance checks after concurrent runs")
	}
}

// TestCalculateSerializesOnSameSystem confirms the per-system lock actually
// serializes: with N concurrent Calculate calls, none should error and the
// final stored result must be a valid, single coherent calculation (not a
// half-written row).
func TestCalculateSerializesOnSameSystem(t *testing.T) {
	s, _ := newTestService(t)
	sid := seedSystemWithNetwork(t, s)
	ctx := context.Background()

	const n = 12
	var wg sync.WaitGroup
	errs := make(chan error, n)
	for i := 0; i < n; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			_, _, err := s.Calculate(ctx, sid)
			errs <- err
		}()
	}
	wg.Wait()
	close(errs)
	for err := range errs {
		if err != nil {
			t.Fatalf("concurrent calculate: %v", err)
		}
	}
	hyd, err := s.GetHydraulicResult(ctx, sid)
	if err != nil {
		t.Fatalf("get hydraulic: %v", err)
	}
	if hyd.BaseFlowLPM <= 0 || hyd.BaseRequiredPressure <= 0 || hyd.RemotePressureMbar < 500 {
		t.Fatalf("inconsistent hydraulic result after concurrent calc: %+v", hyd)
	}
}

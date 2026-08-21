package selfcheck

import (
	"path/filepath"
	"task141-firehydraulics/internal/clock"
	"task141-firehydraulics/internal/model"
	"task141-firehydraulics/internal/service"
	"testing"
)

func TestBug07_HydraulicSnapshotKeepsEveryBranchLiveAndAfterRecovery(t *testing.T) {
	db := filepath.Join(t.TempDir(), "snapshot.db")
	clk := clock.NewFake(parseTime("2026-02-05T08:00:00Z"))
	srv, st, err := restartServer(db, clk)
	if err != nil {
		t.Fatal(err)
	}
	_, sid, _, remote, err := buildSimpleTree(srv, model.HazardExtra2, 95, 23200, 50)
	if err != nil {
		t.Fatal(err)
	}
	if err := addAdequateSupply(srv, sid); err != nil {
		t.Fatal(err)
	}
	live, _, err := callCalc(srv, sid)
	if err != nil {
		t.Fatal(err)
	}
	if !hasNodeResult(live, remote) || len(live.Nodes) < 4 {
		t.Fatalf("live calculation omitted network branches: %#v", live.Nodes)
	}
	srv.Close()
	if err := st.Close(); err != nil {
		t.Fatal(err)
	}
	restarted, st2, err := restartServer(db, clk)
	if err != nil {
		t.Fatal(err)
	}
	defer restarted.Close()
	defer st2.Close()
	if _, err := service.NewWithClock(st2, clk).Reconcile().ReconcileAll(ctx()); err != nil {
		t.Fatal(err)
	}
	recovered, err := getHydraulic(restarted, sid)
	if err != nil || recovered == nil || !hasNodeResult(recovered, remote) || len(recovered.Nodes) < 4 {
		t.Fatalf("recovered calculation omitted network branches: %#v err=%v", recovered, err)
	}
}
func hasNodeResult(r *model.HydraulicResult, id string) bool {
	for _, n := range r.Nodes {
		if n.NodeID == id {
			return true
		}
	}
	return false
}

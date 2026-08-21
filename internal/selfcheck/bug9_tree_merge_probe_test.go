package selfcheck

import (
	"task141-firehydraulics/internal/clock"
	"task141-firehydraulics/internal/model"
	"testing"
)

func TestBug09_MergedSprinklerBranchIsRejectedBeforeCalculation(t *testing.T) {
	clk := clock.NewFake(parseTime("2026-02-07T08:00:00Z"))
	srv, err := newServer(t.TempDir()+"/tree.db", clk)
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()
	_, sid, source, remote, err := buildSimpleTree(srv, model.HazardExtra2, 95, 23200, 50)
	if err != nil {
		t.Fatal(err)
	}
	err = expectCode(srv, "POST", "/api/systems/"+sid+"/pipes", map[string]any{"upstream_node_id": source, "downstream_node_id": remote, "nominal_dia_mm": 40, "inner_dia_mm": 40, "length_mm": 1000, "c_factor": 150, "fitting_equiv_mm": 0, "seq": 99}, 422)
	if err != nil {
		t.Fatal(err)
	}
}

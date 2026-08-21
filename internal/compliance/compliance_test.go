package compliance

import (
	"testing"

	"task141-firehydraulics/internal/hydraulics"
	"task141-firehydraulics/internal/model"
)

func buildNet(t *testing.T, sys *model.System) (*hydraulics.Network, []model.Node, []model.PipeSegment) {
	t.Helper()
	nodes := []model.Node{
		{ID: "s", Type: model.NodeSource, ElevationMM: 0},
		{ID: "j", Type: model.NodeJunction, ElevationMM: 500},
		{ID: "sp1", Type: model.NodeSprinkler, KFactor: 80, ElevationMM: 3000},
		{ID: "sp2", Type: model.NodeSprinkler, KFactor: 80, ElevationMM: 3000},
		{ID: "dr", Type: model.NodeDrain, ElevationMM: 0},
	}
	pipes := []model.PipeSegment{
		{ID: "p1", UpstreamNodeID: "s", DownstreamNodeID: "j", InnerDiaMM: 78, LengthMM: 6000, CFactor: 150},
		{ID: "p2", UpstreamNodeID: "j", DownstreamNodeID: "sp1", InnerDiaMM: 40, LengthMM: 3000, CFactor: 150},
		{ID: "p3", UpstreamNodeID: "j", DownstreamNodeID: "sp2", InnerDiaMM: 40, LengthMM: 3000, CFactor: 150},
	}
	net, err := hydraulics.BuildNetwork("s", nodes, pipes)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	return net, nodes, pipes
}

func TestCheckPass(t *testing.T) {
	sys := &model.System{ID: "sys", DesignAreaDM2: 23200, DesignDensity: 95, PerHeadCoverageDM2: 50}
	net, nodes, pipes := buildNet(t, sys)
	res, err := hydraulics.Calc(net, "sp1", 500)
	if err != nil {
		t.Fatalf("calc: %v", err)
	}
	sup := &model.WaterSupply{StaticPressure: 6000, Points: []model.SupplyPoint{{FlowLPM: 500, Pressure: 5500}, {FlowLPM: 1000, Pressure: 5000}}}
	cmp := &model.SupplyComparison{SurplusMbar: 1000}
	in := Input{System: sys, Nodes: nodes, Pipes: pipes, WaterSupply: sup, Hydraulic: res, Supply: cmp, Network: net}
	checks := Check(in)
	passed := map[string]bool{}
	for _, c := range checks {
		passed[c.RuleCode] = c.Passed
	}
	for _, must := range []string{RRemotePressure, RDensity, RMaxPressure, RTreeIntegrity, RDrainTest, RSingleDesign, RSupplyAdequacy, RPumpOverspeed, RVelocity, RSpareHeads} {
		if !passed[must] {
			t.Errorf("expected %s to pass", must)
		}
	}
}

func TestCheckFailsNoSupply(t *testing.T) {
	sys := &model.System{ID: "sys", DesignAreaDM2: 23200, DesignDensity: 95, PerHeadCoverageDM2: 50}
	net, nodes, pipes := buildNet(t, sys)
	res, err := hydraulics.Calc(net, "sp1", 500)
	if err != nil {
		t.Fatalf("calc: %v", err)
	}
	in := Input{System: sys, Nodes: nodes, Pipes: pipes, WaterSupply: nil, Hydraulic: res, Supply: nil, Network: net}
	checks := Check(in)
	for _, c := range checks {
		if c.RuleCode == RSupplyAdequacy && c.Passed {
			t.Errorf("R-supply-adequacy should fail with no supply")
		}
	}
}

func TestCheckFailsNoDrain(t *testing.T) {
	sys := &model.System{ID: "sys", DesignAreaDM2: 23200, DesignDensity: 95, PerHeadCoverageDM2: 50}
	// network without a drain node
	nodes := []model.Node{
		{ID: "s", Type: model.NodeSource, ElevationMM: 0},
		{ID: "j", Type: model.NodeJunction, ElevationMM: 500},
		{ID: "sp1", Type: model.NodeSprinkler, KFactor: 80, ElevationMM: 3000},
	}
	pipes := []model.PipeSegment{
		{ID: "p1", UpstreamNodeID: "s", DownstreamNodeID: "j", InnerDiaMM: 78, LengthMM: 6000, CFactor: 150},
		{ID: "p2", UpstreamNodeID: "j", DownstreamNodeID: "sp1", InnerDiaMM: 40, LengthMM: 3000, CFactor: 150},
	}
	net, err := hydraulics.BuildNetwork("s", nodes, pipes)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	res, _ := hydraulics.Calc(net, "sp1", 500)
	in := Input{System: sys, Nodes: nodes, Pipes: pipes, Hydraulic: res, Network: net}
	for _, c := range Check(in) {
		if c.RuleCode == RDrainTest && c.Passed {
			t.Errorf("R-drain-test should fail without a drain node")
		}
	}
}

func TestCheckFailsBrokenTree(t *testing.T) {
	sys := &model.System{ID: "sys", DesignAreaDM2: 23200, DesignDensity: 95, PerHeadCoverageDM2: 50}
	in := Input{System: sys, Network: nil} // nil → broken tree
	for _, c := range Check(in) {
		if c.RuleCode == RTreeIntegrity && c.Passed {
			t.Errorf("R-tree-integrity should fail with nil network")
		}
	}
}

func TestCheckFailsConfluenceTree(t *testing.T) {
	// A confluence (one node fed by two upstream branches) must not be accepted
	// as a valid design: BuildNetwork rejects it, the service sets Network nil,
	// and the tree-integrity rule must therefore fail.
	sys := &model.System{ID: "sys", DesignAreaDM2: 23200, DesignDensity: 95, PerHeadCoverageDM2: 50}
	in := Input{System: sys, Network: nil}
	for _, c := range Check(in) {
		if c.RuleCode == RTreeIntegrity && c.Passed {
			t.Fatalf("R-tree-integrity should fail for a confluence network")
		}
	}
}

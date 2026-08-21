package hydraulics

import (
	"math"
	"testing"

	"task141-firehydraulics/internal/model"
)

func TestSprinklerFlow(t *testing.T) {
	// Q = k·√P. k=80, P=1 bar (1000 mbar) → Q = 80 L/min.
	got := SprinklerFlow(80, 1000)
	if got != 80 {
		t.Errorf("SprinklerFlow(80,1000)=%d want 80", got)
	}
	// k=80, P=0.25 bar (250 mbar) → Q = 80·0.5 = 40 L/min.
	got = SprinklerFlow(80, 250)
	if got != 40 {
		t.Errorf("SprinklerFlow(80,250)=%d want 40", got)
	}
	// zero pressure → 0 flow
	if q := SprinklerFlow(80, 0); q != 0 {
		t.Errorf("SprinklerFlow(80,0)=%d want 0", q)
	}
}

func TestHazenWilliamsLossMonotone(t *testing.T) {
	// Friction loss increases with flow (other params fixed).
	lo := HazenWilliamsLossMbar(3000, 40, 150, 50)
	hi := HazenWilliamsLossMbar(3000, 40, 150, 100)
	if lo <= 0 || hi <= lo {
		t.Errorf("loss not monotone in flow: lo=%d hi=%d", lo, hi)
	}
	// Friction loss decreases with diameter (other params fixed).
	dlo := HazenWilliamsLossMbar(3000, 25, 150, 100)
	dhi := HazenWilliamsLossMbar(3000, 50, 150, 100)
	if dlo <= dhi {
		t.Errorf("loss not decreasing in diameter: d25=%d d50=%d", dlo, dhi)
	}
}

func TestElevationPressure(t *testing.T) {
	// 1 m height ≈ 98.0665 mbar. 1000 mm → ~98 mbar.
	p := ElevationPressureMbar(1000)
	if math.Abs(float64(p)-98.0665) > 1.0 {
		t.Errorf("ElevationPressureMbar(1000)=%d want ~98", p)
	}
}

func TestPipeInnerDiameter(t *testing.T) {
	cases := map[int64]int64{
		25: 26, 32: 34, 40: 40, 50: 52, 65: 62, 80: 78, 100: 102, 150: 154,
	}
	for nominal, want := range cases {
		if got := PipeInnerDiameter(nominal); got != want {
			t.Errorf("PipeInnerDiameter(%d)=%d want %d", nominal, got, want)
		}
	}
}

func TestBuildNetworkCycleRejected(t *testing.T) {
	nodes := []model.Node{
		{ID: "s", Type: model.NodeSource},
		{ID: "a", Type: model.NodeJunction},
		{ID: "b", Type: model.NodeJunction},
	}
	// s→a, a→b, b→a would be a cycle (a has two parents: s and b).
	pipes := []model.PipeSegment{
		{ID: "p1", UpstreamNodeID: "s", DownstreamNodeID: "a"},
		{ID: "p2", UpstreamNodeID: "a", DownstreamNodeID: "b"},
		{ID: "p3", UpstreamNodeID: "b", DownstreamNodeID: "a"},
	}
	_, err := BuildNetwork("s", nodes, pipes)
	if err == nil {
		t.Fatalf("expected cycle error, got nil")
	}
}

func TestBuildNetworkDisconnected(t *testing.T) {
	nodes := []model.Node{
		{ID: "s", Type: model.NodeSource},
		{ID: "a", Type: model.NodeSprinkler, KFactor: 80},
		{ID: "z", Type: model.NodeSprinkler, KFactor: 80}, // unreachable
	}
	pipes := []model.PipeSegment{
		{ID: "p1", UpstreamNodeID: "s", DownstreamNodeID: "a"},
	}
	_, err := BuildNetwork("s", nodes, pipes)
	if err == nil {
		t.Fatalf("expected disconnect error for node z, got nil")
	}
}

func TestCalcSimpleTree(t *testing.T) {
	// source → junction → sprinkler(remote). Two sprinklers off the junction.
	nodes := []model.Node{
		{ID: "s", Type: model.NodeSource, ElevationMM: 0},
		{ID: "j", Type: model.NodeJunction, ElevationMM: 500},
		{ID: "sp1", Type: model.NodeSprinkler, KFactor: 80, ElevationMM: 3000},
		{ID: "sp2", Type: model.NodeSprinkler, KFactor: 80, ElevationMM: 3000},
	}
	pipes := []model.PipeSegment{
		{ID: "p1", UpstreamNodeID: "s", DownstreamNodeID: "j", InnerDiaMM: 78, LengthMM: 6000, CFactor: 150},
		{ID: "p2", UpstreamNodeID: "j", DownstreamNodeID: "sp1", InnerDiaMM: 40, LengthMM: 3000, CFactor: 150},
		{ID: "p3", UpstreamNodeID: "j", DownstreamNodeID: "sp2", InnerDiaMM: 40, LengthMM: 3000, CFactor: 150},
	}
	net, err := BuildNetwork("s", nodes, pipes)
	if err != nil {
		t.Fatalf("build: %v", err)
	}
	res, err := Calc(net, "sp1", 500)
	if err != nil {
		t.Fatalf("calc: %v", err)
	}
	if res.RemotePressureMbar < 500 {
		t.Errorf("remote pressure %d < 500", res.RemotePressureMbar)
	}
	if res.RemoteFlowLPM <= 0 {
		t.Errorf("remote flow not positive: %d", res.RemoteFlowLPM)
	}
	if res.BaseFlowLPM <= res.RemoteFlowLPM {
		t.Errorf("base flow %d should exceed single remote flow %d", res.BaseFlowLPM, res.RemoteFlowLPM)
	}
	if res.BaseRequiredPressure <= res.RemotePressureMbar {
		t.Errorf("base required pressure %d should exceed remote pressure %d", res.BaseRequiredPressure, res.RemotePressureMbar)
	}
}

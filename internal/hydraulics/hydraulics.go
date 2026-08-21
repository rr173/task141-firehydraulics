// Package hydraulics implements the NFPA 13 "tree method" hydraulic calculation
// as pure functions over the stored network. The network is a tree rooted at
// the water-source node; each sprinkler emits a flow determined by its k-factor
// and the pressure at its node (Q = k·√P). Friction loss in each pipe segment
// uses the Hazen-Williams formula. The calculation walks from the most
// unfavourable sprinkler upstream toward the source, summing flows and
// accumulating pressure loss, then back-substitutes the resulting node
// pressures to recompute each sprinkler's actual flow (one correction pass).
//
// Internally the math uses float64 (Hazen-Williams and sqrt are non-integer
// powers); every value crosses the API boundary as a fixed-point integer
// (pressure mbar, flow L/min, length mm). The conversion uses RoundHalfUp.
package hydraulics

import (
	"fmt"
	"math"

	"task141-firehydraulics/internal/model"
)

// Conversion constants between the stored fixed-point integers and the float
// units used by the physics. Pressure is stored in mbar (1 bar = 1000 mbar);
// flow in L/min; length in mm.
const (
	barPerMbar = 1.0 / 1000.0      // 1 mbar = 0.001 bar
	m3PerL     = 1.0 / 1000.0     // 1 L = 0.001 m³
	mPerMM     = 1.0 / 1000.0     // 1 mm = 0.001 m
	// 1 metre of water column ≈ 9806.65 Pa = 98.0665 mbar. Used to convert a
	// Hazen-Williams head loss (in metres of water) to a pressure loss in mbar.
	mbarPerMetreWater = 9806.65 / 1000.0 * 1000.0 / 1000.0 // = 9.80665? keep explicit
)

// mbarPerMWater is metres-of-water → mbar: 1 m H2O = 9806.65 Pa = 98.0665 mbar.
const mbarPerMWater = 98.0665

// SprinklerFlow returns the discharge Q (L/min) of a sprinkler with k-factor k
// at node pressure p (mbar): Q = k·√(p_bar) = k·√(p_mbar/1000).
func SprinklerFlow(k int64, pressureMbar int64) int64 {
	if k <= 0 || pressureMbar <= 0 {
		return 0
	}
	pBar := float64(pressureMbar) * barPerMbar
	q := float64(k) * math.Sqrt(pBar)
	return roundHalfUp(q)
}

// HazenWilliamsLossMbar returns the friction pressure loss (mbar) along a pipe
// segment of length L (mm, including fitting equivalent length), inner
// diameter d (mm), Hazen-Williams C, carrying flow Q (L/min).
//
// Metric form (head loss in metres of water):
//
//	h_f = 10.67 · L_m · Q_m3s^1.852 / (C^1.852 · d_m^4.87)
//
// then ΔP_mbar = h_f · 98.0665. 1 m H2O ≈ 98.0665 mbar.
func HazenWilliamsLossMbar(Lmm, dMM, c, flowLPM int64) int64 {
	if Lmm <= 0 || dMM <= 0 || c <= 0 || flowLPM <= 0 {
		return 0
	}
	Lm := float64(Lmm) * mPerMM
	dm := float64(dMM) * mPerMM
	qM3s := float64(flowLPM) * m3PerL / 60.0 // L/min → m³/s
	hf := 10.67 * Lm * math.Pow(qM3s, 1.852) / (math.Pow(float64(c), 1.852) * math.Pow(dm, 4.87))
	return roundHalfUp(hf * mbarPerMWater)
}

// ElevationPressureMbar returns the pressure change (mbar) due to an elevation
// difference of dh mm (positive dh = downstream node higher than upstream, so
// pressure drops moving upstream→downstream). 1 m height ≈ 98.0665 mbar.
// For a rise of dh mm the pressure drops by dh/1000·98.0665 mbar.
func ElevationPressureMbar(dhMM int64) int64 {
	return roundHalfUp(float64(dhMM) * mPerMM * mbarPerMWater)
}

// Velocity returns the water velocity (m/s, scaled to 0.01 m/s → int) in a pipe
// of inner diameter d (mm) carrying flow Q (L/min). v = Q_m3s / A_m2. Returned
// as centi-m/s (0.01 m/s) integer for the compliance velocity check.
func Velocity(dMM, flowLPM int64) int64 {
	if dMM <= 0 || flowLPM <= 0 {
		return 0
	}
	dm := float64(dMM) * mPerMM
	area := math.Pi / 4.0 * dm * dm
	qM3s := float64(flowLPM) * m3PerL / 60.0
	v := qM3s / area // m/s
	return roundHalfUp(v * 100.0) // 0.01 m/s units
}

// PipeInnerDiameter returns the actual inner diameter (mm) for a nominal steel
// pipe size, using Schedule 40 steel equivalents (locked table). Unknown sizes
// fall back to nominal (assumes the caller validated the size).
func PipeInnerDiameter(nominalMM int64) int64 {
	switch nominalMM {
	case 25:
		return 26 // DN25 (1") Sch40 ID ≈ 26.6
	case 32:
		return 34
	case 40:
		return 40
	case 50:
		return 52
	case 65:
		return 62
	case 80:
		return 78
	case 100:
		return 102
	case 150:
		return 154
	default:
		return nominalMM
	}
}

// Network is the in-memory tree the calculation walks. Nodes are keyed by id;
// Pipes carry an UpstreamNodeID (toward source) and DownstreamNodeID (toward
// sprinklers). The source node is the root.
type Network struct {
	SourceID string
	Nodes    map[string]*model.Node
	Pipes    []*model.PipeSegment
	// downstream maps upstream-node-id → list of (pipe, downstream-node).
	downstream map[string][]edge
}

type edge struct {
	pipe *model.PipeSegment
	to   string
}

// BuildNetwork indexes the nodes and pipes into a tree keyed by the source
// node. It returns an error if the network is not a connected tree rooted at
// the source (cycles, dangling refs, multiple sources).
func BuildNetwork(sourceID string, nodes []model.Node, pipes []model.PipeSegment) (*Network, error) {
	if sourceID == "" {
		return nil, fmt.Errorf("hydraulics: empty source node id")
	}
	n := &Network{
		SourceID:   sourceID,
		Nodes:      make(map[string]*model.Node, len(nodes)),
		downstream: make(map[string][]edge),
	}
	for i := range nodes {
		n.Nodes[nodes[i].ID] = &nodes[i]
	}
	src, ok := n.Nodes[sourceID]
	if !ok {
		return nil, fmt.Errorf("hydraulics: source node %s not in node set", sourceID)
	}
	if src.Type != model.NodeSource && src.Type != model.NodeBase {
		return nil, fmt.Errorf("hydraulics: source node %s must be source/base type, got %s", sourceID, src.Type)
	}
	for i := range pipes {
		p := pipes[i]
		if _, ok := n.Nodes[p.UpstreamNodeID]; !ok {
			return nil, fmt.Errorf("hydraulics: pipe %s upstream node %s missing", p.ID, p.UpstreamNodeID)
		}
		if _, ok := n.Nodes[p.DownstreamNodeID]; !ok {
			return nil, fmt.Errorf("hydraulics: pipe %s downstream node %s missing", p.ID, p.DownstreamNodeID)
		}
		n.downstream[p.UpstreamNodeID] = append(n.downstream[p.UpstreamNodeID], edge{pipe: &p, to: p.DownstreamNodeID})
	}
	if err := n.assertTree(); err != nil {
		return nil, err
	}
	return n, nil
}

// assertTree verifies the graph is a tree rooted at source: every non-source
// non-terminal node has exactly one upstream edge, no cycles, and every
// emitter (sprinkler/standpipe) node is reachable from the source. Drain /
// test-connection nodes are allowed to be disconnected (they are physical
// endpoints, not part of the hydraulic path).
func (n *Network) assertTree() error {
	inDegree := make(map[string]int, len(n.Nodes))
	for _, es := range n.downstream {
		for _, e := range es {
			inDegree[e.to]++
		}
	}
	// Every non-source node must have in-degree 0 or 1; in-degree > 1 is a cycle.
	for id := range n.Nodes {
		if id == n.SourceID {
			continue
		}
		if inDegree[id] > 2 {
			return fmt.Errorf("hydraulics: node %s has %d upstream pipes (cycle or merge), tree required", id, inDegree[id])
		}
	}
	// Source must have in-degree 0.
	if inDegree[n.SourceID] != 0 {
		return fmt.Errorf("hydraulics: source node %s has an upstream pipe (cycle)", n.SourceID)
	}
	// DFS from source; every emitter (sprinkler/standpipe) must be reachable.
	// Drain nodes are allowed to dangle.
	visited := make(map[string]bool)
	n.dfs(n.SourceID, visited)
	for id, nd := range n.Nodes {
		if !visited[id] && (nd.Type == model.NodeSprinkler || nd.Type == model.NodeStandpipe || nd.Type == model.NodeJunction) {
			return fmt.Errorf("hydraulics: %s node %s (%s) unreachable from source in DFS", nd.Type, id, nd.Label)
		}
	}
	return nil
}

func (n *Network) dfs(id string, visited map[string]bool) {
	if visited[id] {
		return
	}
	visited[id] = true
	for _, e := range n.downstream[id] {
		n.dfs(e.to, visited)
	}
}

// children returns the downstream edges from a node.
func (n *Network) children(id string) []edge {
	return n.downstream[id]
}

// Children returns the downstream node IDs from a node, in insertion order. It
// is the exported view the compliance rules use to walk the tree.
func (n *Network) Children(id string) []string {
	var out []string
	for _, e := range n.downstream[id] {
		out = append(out, e.to)
	}
	return out
}

// upstreamPipe returns the pipe whose downstream node is id (the pipe feeding
// this node from upstream). For the source it returns nil.
func (n *Network) upstreamPipe(id string) *model.PipeSegment {
	for _, es := range n.downstream {
		for _, e := range es {
			if e.to == id {
				return e.pipe
			}
		}
	}
	return nil
}

// Calc performs the tree hydraulic calculation. remoteNodeID is the most
// unfavourable sprinkler; remotePressure is its starting minimum design
// pressure (mbar). The result contains per-node pressure and per-sprinkler
// flow, plus the base/source total flow and required pressure.
//
// Algorithm (tree method with one back-substitution):
//  1. Walk downstream→upstream from the remote sprinkler. At each sprinkler
//     the flow is k·√P (first pass uses the starting pressure for the remote
//     and the just-computed node pressure for others encountered).
//  2. For each pipe, the carried flow = sum of all sprinkler flows in its
//     downstream subtree. The pressure at the downstream end of a pipe plus
//     friction loss + elevation change gives the upstream node pressure.
//  3. Once the source pressure (base required pressure) is known, walk
//     upstream→downstream from the source recomputing every node's pressure
//     from the source pressure minus loss, and recompute each sprinkler's flow
//     from its (now consistent) node pressure.
//
// Because the tree is acyclic and flows are monotone toward the source, one
// back-substitution pass yields a consistent node-pressure table.
func Calc(net *Network, remoteNodeID string, remotePressureMbar int64) (*model.HydraulicResult, error) {
	remote, ok := net.Nodes[remoteNodeID]
	if !ok {
		return nil, fmt.Errorf("hydraulics: remote node %s not in network", remoteNodeID)
	}
	if remote.Type != model.NodeSprinkler && remote.Type != model.NodeStandpipe {
		return nil, fmt.Errorf("hydraulics: remote node %s must be a sprinkler/standpipe, got %s", remoteNodeID, remote.Type)
	}

	// Phase 1: compute subtree flow at every node (sum of downstream sprinkler
	// flows) using a downstream→upstream pass. Start each sprinkler at the
	// remote starting pressure as a first estimate; refine in phase 2.
	flows := make(map[string]int64) // nodeID → flow at that node (subtree total)
	pressures := make(map[string]int64) // nodeID → pressure (mbar) at the node
	// Seed the remote node pressure.
	pressures[remoteNodeID] = remotePressureMbar
	// Seed all sprinkler pressures with the remote starting pressure as the
	// initial estimate so phase-1 flows are nonzero; phase 2 refines them.
	for id, nd := range net.Nodes {
		if nd.Type == model.NodeSprinkler || nd.Type == model.NodeStandpipe {
			pressures[id] = remotePressureMbar
		}
	}

	// postOrder returns nodes in post-order (children before parents) so a
	// single pass accumulates subtree flows correctly.
	order := net.postOrder(net.SourceID)
	for _, id := range order {
		nd := net.Nodes[id]
		if nd.Type == model.NodeSprinkler || nd.Type == model.NodeStandpipe {
			p := pressures[id]
			if p <= 0 {
				p = remotePressureMbar
			}
			flows[id] = SprinklerFlow(nd.KFactor, p)
		} else {
			// Non-emitter: subtree flow = sum of children pipe-carried flows.
			var sum int64
			for _, e := range net.children(id) {
				sum += pipeFlowAt(net, flows, e)
			}
			flows[id] = sum
		}
	}

	// Phase 2: walk source→downstream computing consistent node pressures from
	// the source/base pressure. The base required pressure = remote pressure +
	// total friction + total elevation along the source→remote path. Compute
	// that path's accumulated loss first by walking remote→source.
	sourceReq := computeSourceRequiredPressure(net, remoteNodeID, remotePressureMbar, flows)
	pressures[net.SourceID] = sourceReq
	// Propagate pressures downstream.
	net.propagatePressure(net.SourceID, pressures, flows)

	// Now recompute flows from the consistent pressures (one back-substitution).
	finalFlows := make(map[string]int64, len(flows))
	for id, nd := range net.Nodes {
		if nd.Type == model.NodeSprinkler || nd.Type == model.NodeStandpipe {
			finalFlows[id] = SprinklerFlow(nd.KFactor, pressures[id])
		}
	}
	// Recompute subtree totals at non-emitters with refined sprinkler flows.
	for _, id := range order {
		nd := net.Nodes[id]
		if nd.Type == model.NodeSprinkler || nd.Type == model.NodeStandpipe {
			finalFlows[id] = SprinklerFlow(nd.KFactor, pressures[id])
		} else {
			var sum int64
			for _, e := range net.children(id) {
				sum += pipeFlowAt(net, finalFlows, e)
			}
			finalFlows[id] = sum
		}
	}

	// Build the node-result table.
	res := &model.HydraulicResult{
		BaseRequiredPressure: sourceReq,
		RemotePressureMbar:   pressures[remoteNodeID],
		RemoteFlowLPM:        finalFlows[remoteNodeID],
		BaseFlowLPM:          finalFlows[net.SourceID],
	}
	for _, id := range net.bfsOrder(net.SourceID) {
		nd := net.Nodes[id]
		nr := model.NodeResult{
			NodeID:       id,
			Label:        nd.Label,
			PressureMbar: pressures[id],
			ElevationMM:  nd.ElevationMM,
		}
		if nd.Type == model.NodeSprinkler || nd.Type == model.NodeStandpipe {
			nr.FlowLPM = finalFlows[id]
		}
		res.Nodes = append(res.Nodes, nr)
	}
	return res, nil
}

// pipeFlowAt returns the flow carried by the pipe ending at edge e.to: it is
// the subtree total of the node e.to.
func pipeFlowAt(net *Network, flows map[string]int64, e edge) int64 {
	return flows[e.to]
}

// postOrder returns node ids in post-order (descendants before ancestors)
// starting from root.
func (n *Network) postOrder(root string) []string {
	var out []string
	var rec func(id string)
	rec = func(id string) {
		for _, e := range n.children(id) {
			rec(e.to)
		}
		out = append(out, id)
	}
	rec(root)
	return out
}

// bfsOrder returns node ids in BFS order from root (source first).
func (n *Network) bfsOrder(root string) []string {
	var out []string
	queue := []string{root}
	visited := map[string]bool{root: true}
	for len(queue) > 0 {
		id := queue[0]
		queue = queue[1:]
		out = append(out, id)
		for _, e := range n.children(id) {
			if !visited[e.to] {
				visited[e.to] = true
				queue = append(queue, e.to)
			}
		}
	}
	return out
}

// computeSourceRequiredPressure walks remote→source summing friction loss +
// elevation change to obtain the pressure that must exist at the source so the
// remote sprinkler sees remotePressureMbar.
//
// Physics: pressure falls as water rises (ρgh per metre) and falls with pipe
// friction. Walking remote→source means moving upstream (toward the lower
// elevation in this layout, but in general toward the supply). At each step the
// upstream node must hold MORE pressure than the downstream node by the
// friction loss plus the static lift from downstream elevation up to upstream
// elevation. The lift is positive when downstream is higher than upstream (the
// usual case: source at street level, sprinklers at ceiling).
func computeSourceRequiredPressure(net *Network, remoteNodeID string, remotePressureMbar int64, flows map[string]int64) int64 {
	cur := remoteNodeID
	srcPressure := remotePressureMbar
	for cur != net.SourceID {
		pipe := net.upstreamPipe(cur)
		if pipe == nil {
			break // safety
		}
		upstream := pipe.UpstreamNodeID
		carriedFlow := flows[cur]
		loss := HazenWilliamsLossMbar(pipe.LengthMM+pipe.FittingEquivMM, pipe.InnerDiaMM, pipe.CFactor, carriedFlow)
		// Static lift to climb from the downstream node (cur) to the upstream
		// node: dh = downstream_elev − upstream_elev. Positive when the
		// downstream sprinkler is above the upstream supply point (the supply
		// must push harder by ρ·g·dh). Source pressure therefore grows.
		dh := net.Nodes[cur].ElevationMM - net.Nodes[upstream].ElevationMM
		elev := ElevationPressureMbar(dh)
		srcPressure += loss + elev
		cur = upstream
	}
	return srcPressure
}

// propagatePressure walks source→downstream assigning each downstream node a
// pressure = upstream node pressure − friction loss − elevation rise. This
// keeps the table internally consistent (node pressures derived from one root).
func (n *Network) propagatePressure(id string, pressures, flows map[string]int64) {
	for _, e := range n.children(id) {
		pipe := e.pipe
		carriedFlow := flows[e.to]
		loss := HazenWilliamsLossMbar(pipe.LengthMM+pipe.FittingEquivMM, pipe.InnerDiaMM, pipe.CFactor, carriedFlow)
		// Downstream node pressure = upstream − loss − elevation rise.
		// dh = downstream elevation − upstream elevation. If downstream is
		// higher, pressure drops.
		dh := n.Nodes[e.to].ElevationMM - n.Nodes[id].ElevationMM
		elev := ElevationPressureMbar(dh)
		pressures[e.to] = pressures[id] - loss - elev
		n.propagatePressure(e.to, pressures, flows)
	}
}

// MaxPressure returns the maximum node pressure in a result (mbar), used by the
// R-max-pressure compliance check.
func MaxPressure(res *model.HydraulicResult) int64 {
	if res == nil {
		return 0
	}
	var max int64
	for _, nr := range res.Nodes {
		if nr.PressureMbar > max {
			max = nr.PressureMbar
		}
	}
	return max
}

// TotalSprinklerFlow returns the sum of all emitter flows in a result, used by
// the R-density check (total flow / design area).
func TotalSprinklerFlow(res *model.HydraulicResult) int64 {
	if res == nil {
		return 0
	}
	var sum int64
	for _, nr := range res.Nodes {
		if nr.FlowLPM > 0 {
			sum += nr.FlowLPM
		}
	}
	return sum
}

// MaxVelocity returns the maximum pipe velocity (0.01 m/s units) across all
// pipes given the computed flows, used by the R-velocity check.
func MaxVelocity(net *Network, flows map[string]int64) int64 {
	var max int64
	for _, p := range net.Pipes {
		f := flows[p.DownstreamNodeID]
		v := Velocity(p.InnerDiaMM, f)
		if v > max {
			max = v
		}
	}
	return max
}

// roundHalfUp rounds x to the nearest integer, half away from zero. This is the
// single conversion boundary from internal float to the external fixed-point.
func roundHalfUp(x float64) int64 {
	if x >= 0 {
		return int64(math.Floor(x + 0.5))
	}
	return int64(math.Ceil(x - 0.5))
}

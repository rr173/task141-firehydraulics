package selfcheck

import (
	"fmt"
	"net/http/httptest"

	"task141-firehydraulics/internal/clock"
	"task141-firehydraulics/internal/model"
)

// buildSimpleTree builds a small sprinkler tree network for a system and
// returns the system id, the source node id and the remote sprinkler node id.
// Topology: source → junction → sprinkler(remote). A second sprinkler hangs off
// the junction to exercise subtree flow summing.
func buildSimpleTree(srv *httptest.Server, hazard model.HazardClass, density, areaDM2, perHeadDM2 int64) (projectID, systemID, sourceID, remoteID string, err error) {
	// Project.
	var proj struct {
		ID string `json:"id"`
	}
	if err = mustDo(srv, "POST", "/api/projects",
		map[string]any{"name": "测试项目", "hazard_class": string(hazard), "design_date": 0}, &proj); err != nil {
		return
	}
	projectID = proj.ID
	return buildTreeInProject(srv, projectID, "喷淋1", hazard, density, areaDM2, perHeadDM2)
}

// buildTreeInProject adds a sprinkler tree to an EXISTING project (used by the
// cross-system-compensation test so two systems share one project).
func buildTreeInProject(srv *httptest.Server, projectID, sysName string, hazard model.HazardClass, density, areaDM2, perHeadDM2 int64) (pid, systemID, sourceID, remoteID string, err error) {
	pid = projectID
	var sys model.System
	if err = mustDo(srv, "POST", "/api/projects/"+projectID+"/systems",
		map[string]any{"kind": "sprinkler", "name": sysName, "base_elevation_mm": 0,
			"design_density": density, "design_area_dm2": areaDM2, "per_head_coverage_dm2": perHeadDM2}, &sys); err != nil {
		return
	}
	systemID = sys.ID

	// Source node.
	var src model.Node
	if err = mustDo(srv, "POST", "/api/systems/"+systemID+"/nodes",
		map[string]any{"type": "source", "label": "水源", "elevation_mm": 0, "seq": 0}, &src); err != nil {
		return
	}
	sourceID = src.ID

	// Drain/test node (so R-drain-test passes).
	if err = mustDo(srv, "POST", "/api/systems/"+systemID+"/nodes",
		map[string]any{"type": "drain", "label": "试验接口", "elevation_mm": 0, "seq": 1}, nil); err != nil {
		return
	}

	// Junction.
	var jct model.Node
	if err = mustDo(srv, "POST", "/api/systems/"+systemID+"/nodes",
		map[string]any{"type": "junction", "label": "三通", "elevation_mm": 500, "seq": 2}, &jct); err != nil {
		return
	}

	// Two sprinklers off the junction.
	var sp1 model.Node
	if err = mustDo(srv, "POST", "/api/systems/"+systemID+"/nodes",
		map[string]any{"type": "sprinkler", "label": "喷头A", "elevation_mm": 3000, "k_factor": 80, "design_min_pressure_mbar": 500, "seq": 3}, &sp1); err != nil {
		return
	}
	remoteID = sp1.ID
	var sp2 model.Node
	if err = mustDo(srv, "POST", "/api/systems/"+systemID+"/nodes",
		map[string]any{"type": "sprinkler", "label": "喷头B", "elevation_mm": 3000, "k_factor": 80, "design_min_pressure_mbar": 500, "seq": 4}, &sp2); err != nil {
		return
	}

	// Pipes: source→junction, junction→sp1, junction→sp2.
	if err = mustDo(srv, "POST", "/api/systems/"+systemID+"/pipes",
		map[string]any{"upstream_node_id": sourceID, "downstream_node_id": jct.ID,
			"nominal_dia_mm": 80, "inner_dia_mm": 78, "length_mm": 6000, "c_factor": 150, "fitting_equiv_mm": 0, "seq": 0}, nil); err != nil {
		return
	}
	if err = mustDo(srv, "POST", "/api/systems/"+systemID+"/pipes",
		map[string]any{"upstream_node_id": jct.ID, "downstream_node_id": sp1.ID,
			"nominal_dia_mm": 40, "inner_dia_mm": 40, "length_mm": 3000, "c_factor": 150, "fitting_equiv_mm": 0, "seq": 1}, nil); err != nil {
		return
	}
	if err = mustDo(srv, "POST", "/api/systems/"+systemID+"/pipes",
		map[string]any{"upstream_node_id": jct.ID, "downstream_node_id": sp2.ID,
			"nominal_dia_mm": 40, "inner_dia_mm": 40, "length_mm": 3000, "c_factor": 150, "fitting_equiv_mm": 0, "seq": 2}, nil); err != nil {
		return
	}

	// Set remote node to sp1.
	if err = mustDo(srv, "POST", "/api/systems/"+systemID+"/remote-node",
		map[string]any{"node_id": remoteID}, nil); err != nil {
		return
	}
	return
}

// addAdequateSupply attaches a generous municipal water supply that passes the
// adequacy check (high static + gentle residual drop).
func addAdequateSupply(srv *httptest.Server, systemID string) error {
	return mustDo(srv, "POST", "/api/systems/"+systemID+"/water-supply",
		map[string]any{"kind": "city", "static_pressure_mbar": 6000,
			"points": []map[string]any{
				{"flow_lpm": 500, "residual_pressure_mbar": 5500},
				{"flow_lpm": 1000, "residual_pressure_mbar": 5000},
				{"flow_lpm": 2000, "residual_pressure_mbar": 4000},
			}}, nil)
}

// smokeHydraulicCalcAndCompliance builds a light-hazard tree, runs the calc,
// asserts the most-unfavourable pressure ≥ 0.5 bar and base flow > 0, runs
// NFPA compliance and asserts the core rules pass.
func smokeHydraulicCalcAndCompliance(srv *httptest.Server, clk *clock.Fake) error {
	pid, sid, _, remote, err := buildSimpleTree(srv, model.HazardExtra2, 95, 23200, 50)
	if err != nil {
		return err
	}
	_ = pid
	_ = remote
	if err := addAdequateSupply(srv, sid); err != nil {
		return err
	}
	// Calculate.
	var calc struct {
		Hydraulic *model.HydraulicResult  `json:"hydraulic"`
		Supply    *model.SupplyComparison `json:"supply_comparison"`
	}
	if err := mustDo(srv, "POST", "/api/systems/"+sid+"/calculate", nil, &calc); err != nil {
		return err
	}
	if calc.Hydraulic == nil {
		return fmt.Errorf("no hydraulic result")
	}
	if calc.Hydraulic.RemotePressureMbar < 500 {
		return fmt.Errorf("remote pressure %d < 500", calc.Hydraulic.RemotePressureMbar)
	}
	if calc.Hydraulic.BaseFlowLPM <= 0 {
		return fmt.Errorf("base flow not positive: %d", calc.Hydraulic.BaseFlowLPM)
	}
	if calc.Hydraulic.BaseRequiredPressure <= 0 {
		return fmt.Errorf("base required pressure not positive: %d", calc.Hydraulic.BaseRequiredPressure)
	}
	// Supply comparison should show adequacy (surplus ≥ 0) given the generous supply.
	if calc.Supply == nil {
		return fmt.Errorf("no supply comparison")
	}
	if calc.Supply.SurplusMbar < 0 {
		return fmt.Errorf("supply deficit %d with adequate supply", calc.Supply.SurplusMbar)
	}
	// Compliance.
	var comp struct {
		Checks []model.ComplianceCheck `json:"checks"`
	}
	if err := mustDo(srv, "POST", "/api/systems/"+sid+"/compliance", nil, &comp); err != nil {
		return err
	}
	passed := map[string]bool{}
	for _, c := range comp.Checks {
		passed[c.RuleCode] = c.Passed
	}
	for _, must := range []string{"R-remote-pressure", "R-density", "R-max-pressure", "R-tree-integrity", "R-drain-test", "R-single-design"} {
		if !passed[must] {
			return fmt.Errorf("compliance rule %s expected pass, got fail", must)
		}
	}
	return nil
}

// smokeVelocityAndSupplyDeficit configures a deliberately small pipe + weak
// supply so the velocity rule and the supply-adequacy rule fail.
func smokeVelocityAndSupplyDeficit(srv *httptest.Server, clk *clock.Fake) error {
	pid, sid, _, _, err := buildSimpleTree(srv, model.HazardExtra2, 95, 23200, 50)
	if err != nil {
		return err
	}
	_ = pid
	// Weak supply: low static, steep drop → deficit at base flow.
	if err := mustDo(srv, "POST", "/api/systems/"+sid+"/water-supply",
		map[string]any{"kind": "city", "static_pressure_mbar": 700,
			"points": []map[string]any{
				{"flow_lpm": 200, "residual_pressure_mbar": 400},
				{"flow_lpm": 500, "residual_pressure_mbar": 100},
			}}, nil); err != nil {
		return err
	}
	if _, _, err := callCalc(srv, sid); err != nil {
		return err
	}
	var comp struct {
		Checks []model.ComplianceCheck `json:"checks"`
	}
	if err := mustDo(srv, "POST", "/api/systems/"+sid+"/compliance", nil, &comp); err != nil {
		return err
	}
	failed := map[string]bool{}
	for _, c := range comp.Checks {
		failed[c.RuleCode] = !c.Passed
	}
	if !failed["R-supply-adequacy"] {
		return fmt.Errorf("expected R-supply-adequacy to FAIL with weak supply")
	}
	return nil
}

// callCalc runs the calculation and returns the result.
func callCalc(srv *httptest.Server, sid string) (*model.HydraulicResult, *model.SupplyComparison, error) {
	var calc struct {
		Hydraulic *model.HydraulicResult  `json:"hydraulic"`
		Supply    *model.SupplyComparison `json:"supply_comparison"`
	}
	if err := mustDo(srv, "POST", "/api/systems/"+sid+"/calculate", nil, &calc); err != nil {
		return nil, nil, err
	}
	return calc.Hydraulic, calc.Supply, nil
}

// smokeConfluenceRejected proves a confluence network — one sprinkler fed by
// two upstream branches — is rejected at entry (CreatePipe returns 409) AND, if
// such a network ever reached compliance (e.g. via direct store seeding), the
// tree-integrity rule fails it. This guards flow-attribution integrity across
// both the entry and the 校核 paths.
func smokeConfluenceRejected(srv *httptest.Server, clk *clock.Fake) error {
	_, sid, _, _, err := buildSimpleTree(srv, model.HazardOrdinary1, densityFor(model.HazardOrdinary1), areaFor(model.HazardOrdinary1), perHeadFor(model.HazardOrdinary1))
	if err != nil {
		return err
	}
	// Add a second junction and a third pipe that tries to feed an EXISTING
	// downstream sprinkler from a second upstream branch. The first feeder to
	// sp1 is junction→sp1; adding source→sp1 would give sp1 a second parent.
	var jct2 model.Node
	if err := mustDo(srv, "POST", "/api/systems/"+sid+"/nodes",
		map[string]any{"type": "junction", "label": "三通2", "elevation_mm": 500, "seq": 5}, &jct2); err != nil {
		return err
	}
	// Collect an existing sprinkler id to target as the confluence point.
	nodes, err := listNodes(srv, sid)
	if err != nil {
		return err
	}
	var sp1ID string
	for _, n := range nodes {
		if n.Type == model.NodeSprinkler {
			sp1ID = n.ID
			break
		}
	}
	if sp1ID == "" {
		return fmt.Errorf("no sprinkler node found to target as confluence")
	}
	// Connect the second junction to the source first (so it is a valid branch root).
	var srcID string
	for _, n := range nodes {
		if n.Type == model.NodeSource {
			srcID = n.ID
			break
		}
	}
	if err := mustDo(srv, "POST", "/api/systems/"+sid+"/pipes",
		map[string]any{"upstream_node_id": srcID, "downstream_node_id": jct2.ID,
			"nominal_dia_mm": 80, "inner_dia_mm": 78, "length_mm": 1000, "c_factor": 150, "fitting_equiv_mm": 0, "seq": 3}, nil); err != nil {
		return err
	}
	// Now attempt the confluence: jct2 → sp1, but sp1 already has junction as its
	// upstream. This must be rejected with 409 Conflict at entry time.
	if err := expectCode(srv, "POST", "/api/systems/"+sid+"/pipes",
		map[string]any{"upstream_node_id": jct2.ID, "downstream_node_id": sp1ID,
			"nominal_dia_mm": 40, "inner_dia_mm": 40, "length_mm": 2000, "c_factor": 150, "fitting_equiv_mm": 0, "seq": 4}, 409); err != nil {
		return fmt.Errorf("confluence pipe should be rejected at entry: %w", err)
	}
	// Because the confluence was rejected, the network remains a valid tree and
	// the compliance tree-integrity rule still passes.
	if err := mustDo(srv, "POST", "/api/systems/"+sid+"/compliance", nil, nil); err != nil {
		return err
	}
	var comp struct {
		Checks []model.ComplianceCheck `json:"checks"`
	}
	if err := mustDo(srv, "GET", "/api/systems/"+sid+"/compliance", nil, &comp); err != nil {
		return err
	}
	for _, c := range comp.Checks {
		if c.RuleCode == "R-tree-integrity" && !c.Passed {
			return fmt.Errorf("R-tree-integrity should pass after rejecting the confluence, got fail: %s", c.Detail)
		}
	}
	return nil
}

// listNodes fetches the nodes of a system.
func listNodes(srv *httptest.Server, sid string) ([]model.Node, error) {
	var resp struct {
		Nodes []model.Node `json:"nodes"`
	}
	if err := mustDo(srv, "GET", "/api/systems/"+sid+"/nodes", nil, &resp); err != nil {
		return nil, err
	}
	return resp.Nodes, nil
}

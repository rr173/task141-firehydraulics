package store

// schema is the SQLite DDL applied on Open. Every table stores authoritative
// inputs (rows the user authored) or an append-only event stream; derived
// tables (hydraulic_results, supply_comparisons, compliance_checks) are
// recomputable from the inputs via ReconcileAll.
const schema = `
CREATE TABLE IF NOT EXISTS projects (
	id TEXT PRIMARY KEY,
	name TEXT NOT NULL,
	hazard_class TEXT NOT NULL,
	design_date_epoch INTEGER NOT NULL,
	created_at INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS systems (
	id TEXT PRIMARY KEY,
	project_id TEXT NOT NULL REFERENCES projects(id) ON DELETE CASCADE,
	kind TEXT NOT NULL,
	name TEXT NOT NULL,
	base_elevation_mm INTEGER NOT NULL,
	design_area_dm2 INTEGER NOT NULL,
	design_density INTEGER NOT NULL,
	per_head_coverage_dm2 INTEGER NOT NULL,
	remote_node_id TEXT,
	state TEXT NOT NULL DEFAULT 'draft',
	water_supply_id TEXT,
	pump_id TEXT,
	created_at INTEGER NOT NULL,
	updated_at INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_systems_project ON systems(project_id);

CREATE TABLE IF NOT EXISTS nodes (
	id TEXT PRIMARY KEY,
	system_id TEXT NOT NULL REFERENCES systems(id) ON DELETE CASCADE,
	type TEXT NOT NULL,
	label TEXT NOT NULL,
	elevation_mm INTEGER NOT NULL,
	k_factor INTEGER NOT NULL DEFAULT 0,
	design_min_pressure_mbar INTEGER NOT NULL DEFAULT 0,
	seq INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_nodes_system ON nodes(system_id);

CREATE TABLE IF NOT EXISTS pipe_segments (
	id TEXT PRIMARY KEY,
	system_id TEXT NOT NULL REFERENCES systems(id) ON DELETE CASCADE,
	upstream_node_id TEXT NOT NULL REFERENCES nodes(id),
	downstream_node_id TEXT NOT NULL REFERENCES nodes(id),
	nominal_dia_mm INTEGER NOT NULL,
	inner_dia_mm INTEGER NOT NULL,
	length_mm INTEGER NOT NULL,
	c_factor INTEGER NOT NULL,
	fitting_equiv_mm INTEGER NOT NULL DEFAULT 0,
	seq INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_pipes_system ON pipe_segments(system_id);

CREATE TABLE IF NOT EXISTS water_supplies (
	id TEXT PRIMARY KEY,
	system_id TEXT NOT NULL REFERENCES systems(id) ON DELETE CASCADE,
	kind TEXT NOT NULL,
	static_pressure_mbar INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_supply_system ON water_supplies(system_id);

CREATE TABLE IF NOT EXISTS water_supply_points (
	id TEXT PRIMARY KEY,
	supply_id TEXT NOT NULL REFERENCES water_supplies(id) ON DELETE CASCADE,
	flow_lpm INTEGER NOT NULL,
	residual_pressure_mbar INTEGER NOT NULL,
	seq INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_supply_points_supply ON water_supply_points(supply_id);

CREATE TABLE IF NOT EXISTS fire_pumps (
	id TEXT PRIMARY KEY,
	system_id TEXT NOT NULL REFERENCES systems(id) ON DELETE CASCADE,
	rated_flow_lpm INTEGER NOT NULL,
	rated_head_mbar INTEGER NOT NULL,
	churn_pressure_mbar INTEGER NOT NULL,
	hundred_fifty_flow_lpm INTEGER NOT NULL,
	hundred_fifty_head_mbar INTEGER NOT NULL,
	driver_type TEXT NOT NULL,
	rated_rpm INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_pumps_system ON fire_pumps(system_id);

CREATE TABLE IF NOT EXISTS hydraulic_results (
	id TEXT PRIMARY KEY,
	system_id TEXT NOT NULL UNIQUE REFERENCES systems(id) ON DELETE CASCADE,
	base_flow_lpm INTEGER NOT NULL,
	base_required_pressure_mbar INTEGER NOT NULL,
	remote_pressure_mbar INTEGER NOT NULL,
	remote_flow_lpm INTEGER NOT NULL,
	payload_json TEXT NOT NULL,
	calc_epoch INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS supply_comparisons (
	id TEXT PRIMARY KEY,
	system_id TEXT NOT NULL UNIQUE REFERENCES systems(id) ON DELETE CASCADE,
	base_flow_lpm INTEGER NOT NULL,
	available_pressure_mbar INTEGER NOT NULL,
	required_pressure_mbar INTEGER NOT NULL,
	surplus_mbar INTEGER NOT NULL,
	needs_pump INTEGER NOT NULL,
	pump_added INTEGER NOT NULL,
	checked_epoch INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS compliance_checks (
	id TEXT PRIMARY KEY,
	system_id TEXT NOT NULL REFERENCES systems(id) ON DELETE CASCADE,
	rule_code TEXT NOT NULL,
	passed INTEGER NOT NULL,
	detail TEXT NOT NULL,
	checked_epoch INTEGER NOT NULL,
	UNIQUE(system_id, rule_code)
);
CREATE INDEX IF NOT EXISTS idx_compliance_system ON compliance_checks(system_id);

CREATE TABLE IF NOT EXISTS hydrostatic_tests (
	id TEXT PRIMARY KEY,
	system_id TEXT NOT NULL REFERENCES systems(id) ON DELETE CASCADE,
	test_pressure_mbar INTEGER NOT NULL,
	hold_seconds INTEGER NOT NULL,
	leaked INTEGER NOT NULL,
	passed INTEGER NOT NULL,
	test_epoch INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_hydro_system ON hydrostatic_tests(system_id);

CREATE TABLE IF NOT EXISTS acceptance_records (
	id TEXT PRIMARY KEY,
	system_id TEXT NOT NULL UNIQUE REFERENCES systems(id) ON DELETE CASCADE,
	conclusion TEXT NOT NULL,
	defects_json TEXT NOT NULL,
	accepted_by TEXT NOT NULL,
	accepted_epoch INTEGER NOT NULL
);

CREATE TABLE IF NOT EXISTS inspection_records (
	id TEXT PRIMARY KEY,
	system_id TEXT NOT NULL REFERENCES systems(id) ON DELETE CASCADE,
	kind TEXT NOT NULL,
	result TEXT NOT NULL,
	detail TEXT NOT NULL,
	inspected_epoch INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_inspec_system ON inspection_records(system_id);

CREATE TABLE IF NOT EXISTS impairments (
	id TEXT PRIMARY KEY,
	system_id TEXT NOT NULL REFERENCES systems(id) ON DELETE CASCADE,
	scope TEXT NOT NULL,
	reason TEXT NOT NULL,
	started_epoch INTEGER NOT NULL,
	expected_restore_epoch INTEGER NOT NULL,
	actual_restore_epoch INTEGER NOT NULL DEFAULT 0,
	status TEXT NOT NULL DEFAULT 'active'
);
CREATE INDEX IF NOT EXISTS idx_impair_system ON impairments(system_id);

CREATE TABLE IF NOT EXISTS compensating_measures (
	id TEXT PRIMARY KEY,
	impairment_id TEXT NOT NULL REFERENCES impairments(id) ON DELETE CASCADE,
	kind TEXT NOT NULL,
	owner TEXT NOT NULL,
	started_epoch INTEGER NOT NULL,
	ended_epoch INTEGER NOT NULL DEFAULT 0
);
CREATE INDEX IF NOT EXISTS idx_comp_impair ON compensating_measures(impairment_id);

CREATE TABLE IF NOT EXISTS lifecycle_events (
	id TEXT PRIMARY KEY,
	system_id TEXT NOT NULL REFERENCES systems(id) ON DELETE CASCADE,
	from_state TEXT NOT NULL,
	to_state TEXT NOT NULL,
	reason TEXT NOT NULL,
	event_epoch INTEGER NOT NULL
);
CREATE INDEX IF NOT EXISTS idx_events_system ON lifecycle_events(system_id);
CREATE INDEX IF NOT EXISTS idx_events_epoch ON lifecycle_events(event_epoch);
`

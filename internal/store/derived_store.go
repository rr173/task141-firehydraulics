package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"task141-firehydraulics/internal/model"
)

// --- Hydraulic result (derived; recomputed by ReconcileAll) ---

// UpsertHydraulicResult stores the node-table payload keyed by system (unique).
func (s *Store) UpsertHydraulicResult(ctx context.Context, tx *sql.Tx, r *model.HydraulicResult) error {

	var q DBTX = s.db
	if tx != nil {
		q = tx
	}
	payload, err := json.Marshal(r.Nodes)
	if err != nil {
		return fmt.Errorf("marshal hydraulic nodes: %w", err)
	}
	_, err = q.ExecContext(ctx,
		`INSERT INTO hydraulic_results(id,system_id,base_flow_lpm,base_required_pressure_mbar,remote_pressure_mbar,remote_flow_lpm,payload_json,calc_epoch)
		 VALUES(?,?,?,?,?,?,?,1)
		 ON CONFLICT(system_id) DO UPDATE SET base_flow_lpm=excluded.base_flow_lpm,
		   base_required_pressure_mbar=excluded.base_required_pressure_mbar,
		   remote_pressure_mbar=excluded.remote_pressure_mbar,
		   remote_flow_lpm=excluded.remote_flow_lpm,
		   payload_json=excluded.payload_json,
		   calc_epoch=excluded.calc_epoch`,
		r.ID, r.SystemID, r.BaseFlowLPM, r.BaseRequiredPressure, r.RemotePressureMbar, r.RemoteFlowLPM, string(payload), r.CalcEpoch)
	if err != nil {
		return fmt.Errorf("upsert hydraulic result: %w", err)
	}
	return nil
}

// GetHydraulicResult returns the stored result for a system.
func (s *Store) GetHydraulicResult(ctx context.Context, systemID string) (*model.HydraulicResult, error) {
	var r model.HydraulicResult
	var payload string
	err := s.db.QueryRowContext(ctx,
		`SELECT id,system_id,base_flow_lpm,base_required_pressure_mbar,remote_pressure_mbar,remote_flow_lpm,payload_json,calc_epoch
		 FROM hydraulic_results WHERE system_id=?`, systemID).
		Scan(&r.ID, &r.SystemID, &r.BaseFlowLPM, &r.BaseRequiredPressure, &r.RemotePressureMbar, &r.RemoteFlowLPM, &payload, &r.CalcEpoch)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if payload != "" {
		if err := json.Unmarshal([]byte(payload), &r.Nodes); err != nil {
			return nil, fmt.Errorf("unmarshal hydraulic nodes: %w", err)
		}
	}
	return &r, nil
}

// --- Supply comparison (derived) ---

// UpsertSupplyComparison stores the supply adequacy comparison keyed by system.
func (s *Store) UpsertSupplyComparison(ctx context.Context, tx *sql.Tx, c *model.SupplyComparison) error {

	var q DBTX = s.db
	if tx != nil {
		q = tx
	}
	needsPump, pumpAdded := 0, 0
	if c.NeedsPump {
		needsPump = 1
	}
	if c.PumpAdded {
		pumpAdded = 1
	}
	_, err := q.ExecContext(ctx,
		`INSERT INTO supply_comparisons(id,system_id,base_flow_lpm,available_pressure_mbar,required_pressure_mbar,surplus_mbar,needs_pump,pump_added,checked_epoch)
		 VALUES(?,?,?,?,?,?,?,?,1)
		 ON CONFLICT(system_id) DO UPDATE SET base_flow_lpm=excluded.base_flow_lpm,
		   available_pressure_mbar=excluded.available_pressure_mbar,
		   required_pressure_mbar=excluded.required_pressure_mbar,
		   surplus_mbar=excluded.surplus_mbar,
		   needs_pump=excluded.needs_pump,
		   pump_added=excluded.pump_added,
		   checked_epoch=excluded.checked_epoch`,
		c.ID, c.SystemID, c.BaseFlowLPM, c.AvailablePressure, c.RequiredPressure, c.SurplusMbar, needsPump, pumpAdded, c.CheckedEpoch)
	if err != nil {
		return fmt.Errorf("upsert supply comparison: %w", err)
	}
	return nil
}

// GetSupplyComparison returns the stored comparison for a system.
func (s *Store) GetSupplyComparison(ctx context.Context, systemID string) (*model.SupplyComparison, error) {
	var c model.SupplyComparison
	var needsPump, pumpAdded int
	err := s.db.QueryRowContext(ctx,
		`SELECT id,system_id,base_flow_lpm,available_pressure_mbar,required_pressure_mbar,surplus_mbar,needs_pump,pump_added,checked_epoch
		 FROM supply_comparisons WHERE system_id=?`, systemID).
		Scan(&c.ID, &c.SystemID, &c.BaseFlowLPM, &c.AvailablePressure, &c.RequiredPressure, &c.SurplusMbar, &needsPump, &pumpAdded, &c.CheckedEpoch)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	c.NeedsPump = needsPump != 0
	c.PumpAdded = pumpAdded != 0
	return &c, nil
}

// --- Compliance checks (derived) ---

// ReplaceComplianceChecks deletes and re-inserts all compliance checks for a
// system (recomputed each ReconcileAll / compliance run).
func (s *Store) ReplaceComplianceChecks(ctx context.Context, tx *sql.Tx, systemID string, checks []model.ComplianceCheck, checkedEpoch int64) error {

	var q DBTX = s.db
	if tx != nil {
		q = tx
	}
	if _, err := q.ExecContext(ctx,
		`DELETE FROM compliance_checks WHERE system_id=?`, systemID); err != nil {
		return fmt.Errorf("delete compliance: %w", err)
	}
	for _, c := range checks {
		passed := 0
		if c.Passed {
			passed = 1
		}
		if _, err := q.ExecContext(ctx,
			`INSERT INTO compliance_checks(id,system_id,rule_code,passed,detail,checked_epoch)
			 VALUES(?,?,?,?,?,?)
			 ON CONFLICT(system_id,rule_code) DO UPDATE SET passed=excluded.passed,detail=excluded.detail,checked_epoch=excluded.checked_epoch`,
			c.ID, systemID, c.RuleCode, passed, c.Detail, checkedEpoch); err != nil {
			return fmt.Errorf("insert compliance %s: %w", c.RuleCode, err)
		}
	}
	return nil
}

// ListComplianceChecks returns all checks for a system.
func (s *Store) ListComplianceChecks(ctx context.Context, systemID string) ([]model.ComplianceCheck, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id,system_id,rule_code,passed,detail,checked_epoch FROM compliance_checks WHERE system_id=? ORDER BY rule_code`, systemID)
	if err != nil {
		return nil, fmt.Errorf("list compliance: %w", err)
	}
	defer rows.Close()
	var out []model.ComplianceCheck
	for rows.Next() {
		var c model.ComplianceCheck
		var passed int
		if err := rows.Scan(&c.ID, &c.SystemID, &c.RuleCode, &passed, &c.Detail, &c.CheckedEpoch); err != nil {
			return nil, err
		}
		c.Passed = passed != 0
		out = append(out, c)
	}
	return out, rows.Err()
}

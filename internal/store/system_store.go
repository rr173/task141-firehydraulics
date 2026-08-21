package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"task141-firehydraulics/internal/model"
)

// --- System ---

// CreateSystem inserts a system row in draft state.
func (s *Store) CreateSystem(ctx context.Context, tx *sql.Tx, sys *model.System) error {

	var q DBTX = s.db
	if tx != nil {
		q = tx
	}
	_, err := q.ExecContext(ctx,
		`INSERT INTO systems(id,project_id,kind,name,base_elevation_mm,design_area_dm2,design_density,per_head_coverage_dm2,remote_node_id,state,water_supply_id,pump_id,created_at,updated_at)
		 VALUES(?,?,?,?,?,?,?,?,NULL,?,NULL,NULL,?,?)`,
		sys.ID, sys.ProjectID, string(sys.Kind), sys.Name, sys.BaseElevationMM,
		sys.DesignAreaDM2, sys.DesignDensity, sys.PerHeadCoverageDM2, string(sys.State), sys.UpdatedAt, sys.UpdatedAt)
	if err != nil {
		return fmt.Errorf("create system: %w", err)
	}
	return nil
}

// UpdateSystemState writes the state and timestamps.
func (s *Store) UpdateSystemState(ctx context.Context, tx *sql.Tx, id string, state model.SystemState, updatedAt int64) error {

	var q DBTX = s.db
	if tx != nil {
		q = tx
	}
	res, err := q.ExecContext(ctx,
		`UPDATE systems SET state=?, updated_at=? WHERE id=?`,
		string(state), updatedAt, id)
	if err != nil {
		return fmt.Errorf("update system state: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// SetSystemRemoteNode links the most-unfavourable node.
func (s *Store) SetSystemRemoteNode(ctx context.Context, tx *sql.Tx, systemID, remoteNodeID string, updatedAt int64) error {

	var q DBTX = s.db
	if tx != nil {
		q = tx
	}
	res, err := q.ExecContext(ctx,
		`UPDATE systems SET remote_node_id=?, updated_at=? WHERE id=?`,
		remoteNodeID, updatedAt, systemID)
	if err != nil {
		return fmt.Errorf("set remote node: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// SetSystemWaterSupply links a water supply to a system.
func (s *Store) SetSystemWaterSupply(ctx context.Context, tx *sql.Tx, systemID, supplyID string, updatedAt int64) error {

	var q DBTX = s.db
	if tx != nil {
		q = tx
	}
	res, err := q.ExecContext(ctx,
		`UPDATE systems SET water_supply_id=?, updated_at=? WHERE id=?`,
		supplyID, updatedAt, systemID)
	if err != nil {
		return fmt.Errorf("set water supply: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// SetSystemPump links a fire pump to a system.
func (s *Store) SetSystemPump(ctx context.Context, tx *sql.Tx, systemID, pumpID string, updatedAt int64) error {

	var q DBTX = s.db
	if tx != nil {
		q = tx
	}
	res, err := q.ExecContext(ctx,
		`UPDATE systems SET pump_id=?, updated_at=? WHERE id=?`,
		pumpID, updatedAt, systemID)
	if err != nil {
		return fmt.Errorf("set pump: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// GetSystem returns one system.
func (s *Store) GetSystem(ctx context.Context, id string) (*model.System, error) {
	var sys model.System
	var kind, state string
	var remoteNode, supply, pump sql.NullString
	err := s.db.QueryRowContext(ctx,
		`SELECT id,project_id,kind,name,base_elevation_mm,design_area_dm2,design_density,per_head_coverage_dm2,remote_node_id,state,water_supply_id,pump_id,updated_at
		 FROM systems WHERE id=?`, id).
		Scan(&sys.ID, &sys.ProjectID, &kind, &sys.Name, &sys.BaseElevationMM,
			&sys.DesignAreaDM2, &sys.DesignDensity, &sys.PerHeadCoverageDM2, &remoteNode, &state, &supply, &pump, &sys.UpdatedAt)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	sys.Kind = model.SystemKind(kind)
	sys.State = model.SystemState(state)
	sys.RemoteNodeID = remoteNode.String
	sys.WaterSupplyID = supply.String
	sys.PumpID = pump.String
	return &sys, nil
}

// ListSystemsByProject returns all systems of a project.
func (s *Store) ListSystemsByProject(ctx context.Context, projectID string) ([]model.System, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id,project_id,kind,name,base_elevation_mm,design_area_dm2,design_density,per_head_coverage_dm2,remote_node_id,state,water_supply_id,pump_id,updated_at
		 FROM systems WHERE project_id=? ORDER BY created_at`, projectID)
	if err != nil {
		return nil, fmt.Errorf("list systems: %w", err)
	}
	defer rows.Close()
	var out []model.System
	for rows.Next() {
		var sys model.System
		var kind, state string
		var remoteNode, supply, pump sql.NullString
		if err := rows.Scan(&sys.ID, &sys.ProjectID, &kind, &sys.Name, &sys.BaseElevationMM,
			&sys.DesignAreaDM2, &sys.DesignDensity, &sys.PerHeadCoverageDM2, &remoteNode, &state, &supply, &pump, &sys.UpdatedAt); err != nil {
			return nil, err
		}
		sys.Kind = model.SystemKind(kind)
		sys.State = model.SystemState(state)
		sys.RemoteNodeID = remoteNode.String
		sys.WaterSupplyID = supply.String
		sys.PumpID = pump.String
		out = append(out, sys)
	}
	return out, rows.Err()
}

// AllSystems returns every system (for ReconcileAll).
func (s *Store) AllSystems(ctx context.Context) ([]model.System, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id,project_id,kind,name,base_elevation_mm,design_area_dm2,design_density,per_head_coverage_dm2,remote_node_id,state,water_supply_id,pump_id,updated_at
		 FROM systems ORDER BY updated_at`)
	if err != nil {
		return nil, fmt.Errorf("all systems: %w", err)
	}
	defer rows.Close()
	var out []model.System
	for rows.Next() {
		var sys model.System
		var kind, state string
		var remoteNode, supply, pump sql.NullString
		if err := rows.Scan(&sys.ID, &sys.ProjectID, &kind, &sys.Name, &sys.BaseElevationMM,
			&sys.DesignAreaDM2, &sys.DesignDensity, &sys.PerHeadCoverageDM2, &remoteNode, &state, &supply, &pump, &sys.UpdatedAt); err != nil {
			return nil, err
		}
		sys.Kind = model.SystemKind(kind)
		sys.State = model.SystemState(state)
		sys.RemoteNodeID = remoteNode.String
		sys.WaterSupplyID = supply.String
		sys.PumpID = pump.String
		out = append(out, sys)
	}
	return out, rows.Err()
}

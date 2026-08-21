package store

import (
	"context"
	"database/sql"
	"fmt"

	"task141-firehydraulics/internal/model"
)

// --- Restart recovery / reconcile ---

// ListHydraulicResultSystemIDs returns the system ids that have a stored
// hydraulic result (so ReconcileAll can recompute each).
func (s *Store) ListHydraulicResultSystemIDs(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT system_id FROM hydraulic_results`)
	if err != nil {
		return nil, fmt.Errorf("list hydraulic system ids: %w", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// ListSupplyComparisonSystemIDs returns the system ids that have a stored
// supply comparison.
func (s *Store) ListSupplyComparisonSystemIDs(ctx context.Context) ([]string, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT system_id FROM supply_comparisons`)
	if err != nil {
		return nil, fmt.Errorf("list supply system ids: %w", err)
	}
	defer rows.Close()
	var out []string
	for rows.Next() {
		var id string
		if err := rows.Scan(&id); err != nil {
			return nil, err
		}
		out = append(out, id)
	}
	return out, rows.Err()
}

// DeleteHydraulicResult removes the stored result for a system (used when the
// network is reconfigured and the result must be invalidated).
func (s *Store) DeleteHydraulicResult(ctx context.Context, tx *sql.Tx, systemID string) error {

	var q DBTX = s.db
	if tx != nil {
		q = tx
	}
	if _, err := q.ExecContext(ctx,
		`DELETE FROM hydraulic_results WHERE system_id=?`, systemID); err != nil {
		return fmt.Errorf("delete hydraulic result: %w", err)
	}
	return nil
}

// DeleteSupplyComparison removes the stored comparison for a system.
func (s *Store) DeleteSupplyComparison(ctx context.Context, tx *sql.Tx, systemID string) error {

	var q DBTX = s.db
	if tx != nil {
		q = tx
	}
	if _, err := q.ExecContext(ctx,
		`DELETE FROM supply_comparisons WHERE system_id=?`, systemID); err != nil {
		return fmt.Errorf("delete supply comparison: %w", err)
	}
	return nil
}

// SetSystemStateRaw writes the state column directly (used by ReconcileAll to
// correct a persisted state that disagrees with the event stream).
func (s *Store) SetSystemStateRaw(ctx context.Context, tx *sql.Tx, id string, state model.SystemState, updatedAt int64) error {

	var q DBTX = s.db
	if tx != nil {
		q = tx
	}
	if _, err := q.ExecContext(ctx,
		`UPDATE systems SET state=?, updated_at=? WHERE id=?`,
		string(state), updatedAt, id); err != nil {
		return fmt.Errorf("set system state raw: %w", err)
	}
	return nil
}

// ListActiveImpairments returns impairments still in 'active' status (for
// restart recovery to auto-restore past-due ones).
func (s *Store) ListActiveImpairments(ctx context.Context) ([]model.Impairment, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id,system_id,scope,reason,started_epoch,expected_restore_epoch,actual_restore_epoch,status
		 FROM impairments WHERE status='active' ORDER BY started_epoch`)
	if err != nil {
		return nil, fmt.Errorf("list active impairments: %w", err)
	}
	defer rows.Close()
	var out []model.Impairment
	for rows.Next() {
		var im model.Impairment
		var status string
		if err := rows.Scan(&im.ID, &im.SystemID, &im.Scope, &im.Reason, &im.StartedEpoch, &im.ExpectedRestoreEpoch, &im.ActualRestoreEpoch, &status); err != nil {
			return nil, err
		}
		im.Status = model.ImpairmentStatus(status)
		out = append(out, im)
	}
	return out, rows.Err()
}

// SetSystemPumpIDRaw writes the pump_id column directly (used by ReconcileAll).
func (s *Store) SetSystemPumpIDRaw(ctx context.Context, tx *sql.Tx, systemID, pumpID string, updatedAt int64) error {

	var q DBTX = s.db
	if tx != nil {
		q = tx
	}
	if pumpID == "" {
		if _, err := q.ExecContext(ctx,
			`UPDATE systems SET pump_id=NULL, updated_at=? WHERE id=?`, updatedAt, systemID); err != nil {
			return fmt.Errorf("clear pump id: %w", err)
		}
		return nil
	}
	if _, err := q.ExecContext(ctx,
		`UPDATE systems SET pump_id=?, updated_at=? WHERE id=?`, pumpID, updatedAt, systemID); err != nil {
		return fmt.Errorf("set pump id: %w", err)
	}
	return nil
}

// SetSystemSupplyIDRaw writes the water_supply_id column directly.
func (s *Store) SetSystemSupplyIDRaw(ctx context.Context, tx *sql.Tx, systemID, supplyID string, updatedAt int64) error {

	var q DBTX = s.db
	if tx != nil {
		q = tx
	}
	if supplyID == "" {
		if _, err := q.ExecContext(ctx,
			`UPDATE systems SET water_supply_id=NULL, updated_at=? WHERE id=?`, updatedAt, systemID); err != nil {
			return fmt.Errorf("clear supply id: %w", err)
		}
		return nil
	}
	if _, err := q.ExecContext(ctx,
		`UPDATE systems SET water_supply_id=?, updated_at=? WHERE id=?`, supplyID, updatedAt, systemID); err != nil {
		return fmt.Errorf("set supply id: %w", err)
	}
	return nil
}

var _ = (func() bool {
	_ = sql.ErrNoRows
	return true
}())

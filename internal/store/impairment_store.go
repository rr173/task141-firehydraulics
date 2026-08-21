package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"task141-firehydraulics/internal/model"
)

// --- Impairment ---

// CreateImpairment inserts an impairment row (must have ≥1 compensating measure).
func (s *Store) CreateImpairment(ctx context.Context, tx *sql.Tx, im *model.Impairment) error {

	var q DBTX = s.db
	if tx != nil {
		q = tx
	}
	_, err := q.ExecContext(ctx,
		`INSERT INTO impairments(id,system_id,scope,reason,started_epoch,expected_restore_epoch,actual_restore_epoch,status)
		 VALUES(?,?,?,?,?,?,0,?)`,
		im.ID, im.SystemID, im.Scope, im.Reason, im.StartedEpoch, im.ExpectedRestoreEpoch, string(im.Status))
	if err != nil {
		return fmt.Errorf("create impairment: %w", err)
	}
	return nil
}

// CreateCompensatingMeasure inserts one compensating measure.
func (s *Store) CreateCompensatingMeasure(ctx context.Context, tx *sql.Tx, m *model.CompensatingMeasure) error {

	var q DBTX = s.db
	if tx != nil {
		q = tx
	}
	_, err := q.ExecContext(ctx,
		`INSERT INTO compensating_measures(id,impairment_id,kind,owner,started_epoch,ended_epoch) VALUES(?,?,?,?,?,?)`,
		m.ID, m.ImpairmentID, string(m.Kind), m.Owner, m.StartedEpoch, m.EndedEpoch)
	if err != nil {
		return fmt.Errorf("create compensating measure: %w", err)
	}
	return nil
}

// RestoreImpairment marks an impairment restored and sets the actual restore time.
// It does NOT close the compensating measures; pair it with EndCompensatingMeasures
// inside the same transaction so the restore and the measure closure land at one
// instant.
func (s *Store) RestoreImpairment(ctx context.Context, tx *sql.Tx, id string, actualEpoch int64) error {

	var q DBTX = s.db
	if tx != nil {
		q = tx
	}
	res, err := q.ExecContext(ctx,
		`UPDATE impairments SET status='restored', actual_restore_epoch=? WHERE id=?`,
		actualEpoch, id)
	if err != nil {
		return fmt.Errorf("restore impairment: %w", err)
	}
	n, _ := res.RowsAffected()
	if n == 0 {
		return ErrNotFound
	}
	return nil
}

// EndCompensatingMeasures stamps ended_epoch on every still-open measure of an
// impairment (the moment the impairment was restored). The `ended_epoch=0`
// guard makes it idempotent: a repeated restore does not rewrite measures that
// were already closed. Use it together with RestoreImpairment in one InTx so
// the system restore and the compensation closure are atomic.
func (s *Store) EndCompensatingMeasures(ctx context.Context, tx *sql.Tx, impairmentID string, endedEpoch int64) error {

	var q DBTX = s.db
	if tx != nil {
		q = tx
	}
	if _, err := q.ExecContext(ctx,
		`UPDATE compensating_measures SET ended_epoch=? WHERE impairment_id=? AND ended_epoch=0`,
		endedEpoch, impairmentID); err != nil {
		return fmt.Errorf("end compensating measures: %w", err)
	}
	return nil
}

// ListImpairmentsBySystem returns all impairments for a system.
func (s *Store) ListImpairmentsBySystem(ctx context.Context, systemID string) ([]model.Impairment, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id,system_id,scope,reason,started_epoch,expected_restore_epoch,actual_restore_epoch,status FROM impairments WHERE system_id=? ORDER BY started_epoch`, systemID)
	if err != nil {
		return nil, fmt.Errorf("list impairments: %w", err)
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
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range out {
		ms, err := s.listMeasures(ctx, out[i].ID)
		if err != nil {
			return nil, err
		}
		out[i].Measures = ms
	}
	return out, nil
}

// ListImpairmentsByProject returns all impairments across a project's systems.
func (s *Store) ListImpairmentsByProject(ctx context.Context, projectID string) ([]model.Impairment, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT im.id,im.system_id,im.scope,im.reason,im.started_epoch,im.expected_restore_epoch,im.actual_restore_epoch,im.status
		 FROM impairments im JOIN systems s ON s.id=im.system_id
		 WHERE s.project_id=? ORDER BY im.started_epoch`, projectID)
	if err != nil {
		return nil, fmt.Errorf("list impairments by project: %w", err)
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
	if err := rows.Err(); err != nil {
		return nil, err
	}
	for i := range out {
		ms, err := s.listMeasures(ctx, out[i].ID)
		if err != nil {
			return nil, err
		}
		out[i].Measures = ms
	}
	return out, nil
}

// listMeasures returns the compensating measures for an impairment.
func (s *Store) listMeasures(ctx context.Context, impairmentID string) ([]model.CompensatingMeasure, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id,impairment_id,kind,owner,started_epoch,ended_epoch FROM compensating_measures WHERE impairment_id=? ORDER BY started_epoch`, impairmentID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	var out []model.CompensatingMeasure
	for rows.Next() {
		var m model.CompensatingMeasure
		var kind string
		if err := rows.Scan(&m.ID, &m.ImpairmentID, &kind, &m.Owner, &m.StartedEpoch, &m.EndedEpoch); err != nil {
			return nil, err
		}
		m.Kind = model.CompensatingKind(kind)
		out = append(out, m)
	}
	return out, rows.Err()
}

// GetImpairment returns one impairment with its measures.
func (s *Store) GetImpairment(ctx context.Context, id string) (*model.Impairment, error) {
	var im model.Impairment
	var status string
	err := s.db.QueryRowContext(ctx,
		`SELECT id,system_id,scope,reason,started_epoch,expected_restore_epoch,actual_restore_epoch,status FROM impairments WHERE id=?`, id).
		Scan(&im.ID, &im.SystemID, &im.Scope, &im.Reason, &im.StartedEpoch, &im.ExpectedRestoreEpoch, &im.ActualRestoreEpoch, &status)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	im.Status = model.ImpairmentStatus(status)
	ms, err := s.listMeasures(ctx, id)
	if err != nil {
		return nil, err
	}
	im.Measures = ms
	return &im, nil
}

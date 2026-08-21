package store

import (
	"context"
	"database/sql"
	"encoding/json"
	"errors"
	"fmt"

	"task141-firehydraulics/internal/model"
)

// --- Lifecycle events ---

// AppendLifecycleEvent inserts a state-transition event (event stream source
// of truth for restart recovery).
func (s *Store) AppendLifecycleEvent(ctx context.Context, tx *sql.Tx, e *model.LifecycleEvent) error {

	var q DBTX = s.db
	if tx != nil {
		q = tx
	}
	_, err := q.ExecContext(ctx,
		`INSERT INTO lifecycle_events(id,system_id,from_state,to_state,reason,event_epoch) VALUES(?,?,?,?,?,?)`,
		e.ID, e.SystemID, string(e.FromState), string(e.ToState), e.Reason, e.EventEpoch)
	if err != nil {
		return fmt.Errorf("append lifecycle event: %w", err)
	}
	return nil
}

// ListLifecycleEvents returns all transition events for a system in order.
func (s *Store) ListLifecycleEvents(ctx context.Context, systemID string) ([]model.LifecycleEvent, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id,system_id,from_state,to_state,reason,event_epoch FROM lifecycle_events WHERE system_id=? ORDER BY event_epoch, rowid`, systemID)
	if err != nil {
		return nil, fmt.Errorf("list lifecycle events: %w", err)
	}
	defer rows.Close()
	var out []model.LifecycleEvent
	for rows.Next() {
		var e model.LifecycleEvent
		var from, to string
		if err := rows.Scan(&e.ID, &e.SystemID, &from, &to, &e.Reason, &e.EventEpoch); err != nil {
			return nil, err
		}
		e.FromState = model.SystemState(from)
		e.ToState = model.SystemState(to)
		out = append(out, e)
	}
	return out, rows.Err()
}

// --- Hydrostatic test ---

// CreateHydrostaticTest inserts a test record.
func (s *Store) CreateHydrostaticTest(ctx context.Context, tx *sql.Tx, t *model.HydrostaticTest) error {

	var q DBTX = s.db
	if tx != nil {
		q = tx
	}
	leaked, passed := 0, 0
	if t.Leaked {
		leaked = 1
	}
	if t.Passed {
		passed = 1
	}
	_, err := q.ExecContext(ctx,
		`INSERT INTO hydrostatic_tests(id,system_id,test_pressure_mbar,hold_seconds,leaked,passed,test_epoch) VALUES(?,?,?,?,?,?,?)`,
		t.ID, t.SystemID, t.TestPressureMbar, t.HoldSeconds, leaked, passed, t.TestEpoch)
	if err != nil {
		return fmt.Errorf("create hydrostatic test: %w", err)
	}
	return nil
}

// ListHydrostaticTests returns all tests for a system.
func (s *Store) ListHydrostaticTests(ctx context.Context, systemID string) ([]model.HydrostaticTest, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id,system_id,test_pressure_mbar,hold_seconds,leaked,passed,test_epoch FROM hydrostatic_tests WHERE system_id=? ORDER BY test_epoch`, systemID)
	if err != nil {
		return nil, fmt.Errorf("list hydrostatic tests: %w", err)
	}
	defer rows.Close()
	var out []model.HydrostaticTest
	for rows.Next() {
		var t model.HydrostaticTest
		var leaked, passed int
		if err := rows.Scan(&t.ID, &t.SystemID, &t.TestPressureMbar, &t.HoldSeconds, &leaked, &passed, &t.TestEpoch); err != nil {
			return nil, err
		}
		t.Leaked = leaked != 0
		t.Passed = passed != 0
		out = append(out, t)
	}
	return out, rows.Err()
}

// --- Acceptance record ---

// CreateAcceptanceRecord inserts the acceptance record (one per system).
func (s *Store) CreateAcceptanceRecord(ctx context.Context, tx *sql.Tx, a *model.AcceptanceRecord) error {

	var q DBTX = s.db
	if tx != nil {
		q = tx
	}
	defects, _ := json.Marshal([]string{})
	_, err := q.ExecContext(ctx,
		`INSERT INTO acceptance_records(id,system_id,conclusion,defects_json,accepted_by,accepted_epoch)
		 VALUES(?,?,?,?,?,?)
		 ON CONFLICT(system_id) DO UPDATE SET conclusion=excluded.conclusion,defects_json=excluded.defects_json,accepted_by=excluded.accepted_by,accepted_epoch=excluded.accepted_epoch`,
		a.ID, a.SystemID, a.Conclusion, string(defects), a.AcceptedBy, a.AcceptedEpoch)
	if err != nil {
		return fmt.Errorf("create acceptance record: %w", err)
	}
	return nil
}

// GetAcceptanceRecord returns the acceptance record for a system, if any.
func (s *Store) GetAcceptanceRecord(ctx context.Context, systemID string) (*model.AcceptanceRecord, error) {
	var a model.AcceptanceRecord
	var defects string
	err := s.db.QueryRowContext(ctx,
		`SELECT id,system_id,conclusion,defects_json,accepted_by,accepted_epoch FROM acceptance_records WHERE system_id=?`, systemID).
		Scan(&a.ID, &a.SystemID, &a.Conclusion, &defects, &a.AcceptedBy, &a.AcceptedEpoch)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	if defects != "" {
		_ = json.Unmarshal([]byte(defects), &a.Defects)
	}
	return &a, nil
}

// --- Inspection record ---

// CreateInspectionRecord inserts an inspection/test record.
func (s *Store) CreateInspectionRecord(ctx context.Context, tx *sql.Tx, r *model.InspectionRecord) error {

	var q DBTX = s.db
	if tx != nil {
		q = tx
	}
	_, err := q.ExecContext(ctx,
		`INSERT INTO inspection_records(id,system_id,kind,result,detail,inspected_epoch) VALUES(?,?,?,?,?,?)`,
		r.ID, r.SystemID, string(r.Kind), r.Result, r.Detail, r.InspectedEpoch)
	if err != nil {
		return fmt.Errorf("create inspection record: %w", err)
	}
	return nil
}

// ListInspectionRecords returns all inspection records for a system.
func (s *Store) ListInspectionRecords(ctx context.Context, systemID string) ([]model.InspectionRecord, error) {
	rows, err := s.db.QueryContext(ctx,
		`SELECT id,system_id,kind,result,detail,inspected_epoch FROM inspection_records WHERE system_id=? ORDER BY inspected_epoch`, systemID)
	if err != nil {
		return nil, fmt.Errorf("list inspection records: %w", err)
	}
	defer rows.Close()
	var out []model.InspectionRecord
	for rows.Next() {
		var r model.InspectionRecord
		var kind string
		if err := rows.Scan(&r.ID, &r.SystemID, &kind, &r.Result, &r.Detail, &r.InspectedEpoch); err != nil {
			return nil, err
		}
		r.Kind = model.InspectionKind(kind)
		out = append(out, r)
	}
	return out, rows.Err()
}

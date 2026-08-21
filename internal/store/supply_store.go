package store

import (
	"context"
	"database/sql"
	"errors"
	"fmt"

	"task141-firehydraulics/internal/model"
)

// --- Water supply ---

// CreateWaterSupply inserts a water-supply row (without points).
func (s *Store) CreateWaterSupply(ctx context.Context, tx *sql.Tx, ws *model.WaterSupply) error {

	var q DBTX = s.db
	if tx != nil {
		q = tx
	}
	_, err := q.ExecContext(ctx,
		`INSERT INTO water_supplies(id,system_id,kind,static_pressure_mbar) VALUES(?,?,?,?)`,
		ws.ID, ws.SystemID, string(ws.Kind), ws.StaticPressure)
	if err != nil {
		return fmt.Errorf("create water supply: %w", err)
	}
	return nil
}

// AddSupplyPoint inserts one flow→residual point on a supply curve.
func (s *Store) AddSupplyPoint(ctx context.Context, tx *sql.Tx, p *model.SupplyPoint) error {

	var q DBTX = s.db
	if tx != nil {
		q = tx
	}
	_, err := q.ExecContext(ctx,
		`INSERT INTO water_supply_points(id,supply_id,flow_lpm,residual_pressure_mbar,seq) VALUES(?,?,?,?,?)`,
		p.ID, p.SupplyID, p.FlowLPM, p.Pressure, p.Seq)
	if err != nil {
		return fmt.Errorf("add supply point: %w", err)
	}
	return nil
}

// GetWaterSupply returns the supply for a system with all its curve points.
func (s *Store) GetWaterSupply(ctx context.Context, systemID string) (*model.WaterSupply, error) {
	var ws model.WaterSupply
	var kind string
	err := s.db.QueryRowContext(ctx,
		`SELECT id,system_id,kind,static_pressure_mbar FROM water_supplies WHERE system_id=?`, systemID).
		Scan(&ws.ID, &ws.SystemID, &kind, &ws.StaticPressure)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	ws.Kind = model.WaterSupplyKind(kind)
	rows, err := s.db.QueryContext(ctx,
		`SELECT id,supply_id,flow_lpm,residual_pressure_mbar,seq FROM water_supply_points WHERE supply_id=? ORDER BY seq`, ws.ID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()
	for rows.Next() {
		var p model.SupplyPoint
		if err := rows.Scan(&p.ID, &p.SupplyID, &p.FlowLPM, &p.Pressure, &p.Seq); err != nil {
			return nil, err
		}
		ws.Points = append(ws.Points, p)
	}
	return &ws, rows.Err()
}

// --- Fire pump ---

// CreateFirePump inserts a pump row.
func (s *Store) CreateFirePump(ctx context.Context, tx *sql.Tx, p *model.FirePump) error {

	var q DBTX = s.db
	if tx != nil {
		q = tx
	}
	_, err := q.ExecContext(ctx,
		`INSERT INTO fire_pumps(id,system_id,rated_flow_lpm,rated_head_mbar,churn_pressure_mbar,hundred_fifty_flow_lpm,hundred_fifty_head_mbar,driver_type,rated_rpm)
		 VALUES(?,?,?,?,?,?,?,?,?)`,
		p.ID, p.SystemID, p.RatedFlowLPM, p.RatedHeadMbar, p.ChurnPressureMbar,
		p.FiftyExtraFlowLPM, p.FiftyExtraHeadMbar, string(p.DriverType), p.RatedRPM)
	if err != nil {
		return fmt.Errorf("create fire pump: %w", err)
	}
	return nil
}

// GetFirePump returns the pump for a system, if any.
func (s *Store) GetFirePump(ctx context.Context, systemID string) (*model.FirePump, error) {
	var p model.FirePump
	var dt string
	err := s.db.QueryRowContext(ctx,
		`SELECT id,system_id,rated_flow_lpm,rated_head_mbar,churn_pressure_mbar,hundred_fifty_flow_lpm,hundred_fifty_head_mbar,driver_type,rated_rpm
		 FROM fire_pumps WHERE system_id=?`, systemID).
		Scan(&p.ID, &p.SystemID, &p.RatedFlowLPM, &p.RatedHeadMbar, &p.ChurnPressureMbar,
			&p.FiftyExtraFlowLPM, &p.FiftyExtraHeadMbar, &dt, &p.RatedRPM)
	if err != nil {
		if errors.Is(err, sql.ErrNoRows) {
			return nil, ErrNotFound
		}
		return nil, err
	}
	p.DriverType = model.PumpDriverType(dt)
	return &p, nil
}

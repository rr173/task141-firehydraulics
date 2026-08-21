package service

import (
	"context"
	"database/sql"
	"fmt"

	"task141-firehydraulics/internal/idlib"
	"task141-firehydraulics/internal/model"
	"task141-firehydraulics/internal/store"
	"task141-firehydraulics/internal/watersupply"
)

// --- Water supply ---

// CreateWaterSupply creates a supply for a system (replacing any existing one)
// with a set of flow→residual-pressure curve points. The supply row, its curve
// points, and the system linkage are written in one transaction.
func (s *Services) CreateWaterSupply(ctx context.Context, systemID string, kind model.WaterSupplyKind, staticMbar int64, points []model.SupplyPoint) (*model.WaterSupply, error) {
	sys, err := s.st.GetSystem(ctx, systemID)
	if err != nil {
		return nil, err
	}
	if sys.State != model.StateDraft && sys.State != model.StateDesigned {
		return nil, fmt.Errorf("%w: cannot edit supply in state %s", store.ErrStateConflict, sys.State)
	}
	if staticMbar <= 0 {
		return nil, fmt.Errorf("%w: static pressure must be > 0", store.ErrInvariant)
	}
	ws := &model.WaterSupply{
		ID:             idlib.New("sup"),
		SystemID:       systemID,
		Kind:           kind,
		StaticPressure: staticMbar,
	}
	for i := range points {
		points[i].ID = idlib.New("spt")
		points[i].SupplyID = ws.ID
		points[i].Seq = int64(i)
	}
	ws.Points = points
	if err := s.st.InTx(ctx, func(tx *sql.Tx) error {
		// Replace any existing supply for this system: delete old supply + points.
		var oldID string
		_ = tx.QueryRowContext(ctx, `SELECT id FROM water_supplies WHERE system_id=?`, systemID).Scan(&oldID)
		if oldID != "" {
			if _, err := tx.ExecContext(ctx, `DELETE FROM water_supply_points WHERE supply_id=?`, oldID); err != nil {
				return fmt.Errorf("delete old supply points: %w", err)
			}
			if _, err := tx.ExecContext(ctx, `DELETE FROM water_supplies WHERE id=?`, oldID); err != nil {
				return fmt.Errorf("delete old supply: %w", err)
			}
		}
		if err := s.st.CreateWaterSupply(ctx, tx, ws); err != nil {
			return err
		}
		for _, p := range ws.Points {
			if err := s.st.AddSupplyPoint(ctx, tx, &p); err != nil {
				return err
			}
		}
		if err := s.st.SetSystemWaterSupply(ctx, tx, systemID, ws.ID, s.now()); err != nil {
			return err
		}
		if err := s.st.DeleteHydraulicResult(ctx, tx, systemID); err != nil {
			return err
		}
		return nil
	}); err != nil {
		return nil, err
	}
	return ws, nil
}

// GetWaterSupply returns the supply for a system.
func (s *Services) GetWaterSupply(ctx context.Context, systemID string) (*model.WaterSupply, error) {
	return s.st.GetWaterSupply(ctx, systemID)
}

// --- Fire pump ---

// CreateFirePump creates a pump for a system (replacing any existing one).
func (s *Services) CreateFirePump(ctx context.Context, systemID string, ratedFlow, ratedHead, churn, fiftyFlow, fiftyHead int64, driver model.PumpDriverType, rpm int64) (*model.FirePump, error) {
	sys, err := s.st.GetSystem(ctx, systemID)
	if err != nil {
		return nil, err
	}
	if sys.State != model.StateDraft && sys.State != model.StateDesigned {
		return nil, fmt.Errorf("%w: cannot edit pump in state %s", store.ErrStateConflict, sys.State)
	}
	if churn <= 0 || ratedFlow <= 0 || ratedHead <= 0 {
		return nil, fmt.Errorf("%w: pump churn/rated flow/head must be > 0", store.ErrInvariant)
	}
	if churn > 12000 {
		return nil, fmt.Errorf("%w: pump churn %d mbar exceeds 12 bar component rating", store.ErrInvariant, churn)
	}
	p := &model.FirePump{
		ID:                 idlib.New("pmp"),
		SystemID:           systemID,
		RatedFlowLPM:       ratedFlow,
		RatedHeadMbar:      ratedHead,
		ChurnPressureMbar:  churn,
		FiftyExtraFlowLPM:  fiftyFlow,
		FiftyExtraHeadMbar: fiftyHead,
		DriverType:         driver,
		RatedRPM:           rpm,
	}
	if err := s.st.InTx(ctx, func(tx *sql.Tx) error {
		// Replace existing pump.
		if _, err := tx.ExecContext(ctx, `DELETE FROM fire_pumps WHERE system_id=?`, systemID); err != nil {
			return fmt.Errorf("delete old pump: %w", err)
		}
		if err := s.st.CreateFirePump(ctx, tx, p); err != nil {
			return err
		}
		if err := s.st.SetSystemPump(ctx, tx, systemID, p.ID, s.now()); err != nil {
			return err
		}
		return nil
	}); err != nil {
		return nil, err
	}
	_ = watersupply.Compare // keep import used even when no direct call here
	return p, nil
}

// GetFirePump returns the pump for a system, if any.
func (s *Services) GetFirePump(ctx context.Context, systemID string) (*model.FirePump, error) {
	return s.st.GetFirePump(ctx, systemID)
}

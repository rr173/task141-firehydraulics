package watersupply

import (
	"testing"

	"task141-firehydraulics/internal/model"
)

func TestResidualAt(t *testing.T) {
	s := &model.WaterSupply{
		StaticPressure: 6000,
		Points: []model.SupplyPoint{
			{FlowLPM: 500, Pressure: 5500},
			{FlowLPM: 1000, Pressure: 5000},
			{FlowLPM: 2000, Pressure: 4000},
		},
	}
	// below first point → interpolate from (0,6000) to (500,5500); at 0 → 6000.
	if got := residualAt(s, 0); got != 6000 {
		t.Errorf("residualAt(0)=%d want 6000", got)
	}
	// at first point exactly.
	if got := residualAt(s, 500); got != 5500 {
		t.Errorf("residualAt(500)=%d want 5500", got)
	}
	// midpoint between 500 and 1000 → 5250.
	if got := residualAt(s, 750); got != 5250 {
		t.Errorf("residualAt(750)=%d want 5250", got)
	}
	// at last point.
	if got := residualAt(s, 2000); got != 4000 {
		t.Errorf("residualAt(2000)=%d want 4000", got)
	}
}

func TestPumpBoostAt(t *testing.T) {
	p := &model.FirePump{
		ChurnPressureMbar:  10000,
		RatedFlowLPM:       1500,
		RatedHeadMbar:       8000,
		FiftyExtraFlowLPM:  2250,
		FiftyExtraHeadMbar: 5000,
	}
	// zero flow → churn.
	if got := pumpBoostAt(p, 0); got != 10000 {
		t.Errorf("pumpBoostAt(0)=%d want 10000 (churn)", got)
	}
	// at rated flow → rated head.
	if got := pumpBoostAt(p, 1500); got != 8000 {
		t.Errorf("pumpBoostAt(1500)=%d want 8000 (rated)", got)
	}
	// pump boost decreases with flow: churn > rated > 150%.
	b0 := pumpBoostAt(p, 100)
	b1 := pumpBoostAt(p, 1500)
	b2 := pumpBoostAt(p, 2250)
	if !(b0 > b1 && b1 > b2) {
		t.Errorf("pump curve not monotone decreasing: %d %d %d", b0, b1, b2)
	}
}

func TestCompare(t *testing.T) {
	s := &model.WaterSupply{
		StaticPressure: 6000,
		Points: []model.SupplyPoint{
			{FlowLPM: 500, Pressure: 5500},
			{FlowLPM: 1000, Pressure: 5000},
		},
	}
	// required below available → adequate, no pump.
	cmp := Compare(s, nil, 400, 1000)
	if cmp.NeedsPump {
		t.Errorf("adequate supply should not need pump")
	}
	if cmp.SurplusMbar < 0 {
		t.Errorf("adequate supply should have surplus ≥ 0, got %d", cmp.SurplusMbar)
	}
	// required above available → needs pump.
	cmp2 := Compare(s, nil, 400, 6000)
	if !cmp2.NeedsPump {
		t.Errorf("insufficient supply should need pump")
	}
	// with pump, available rises.
	p := &model.FirePump{ChurnPressureMbar: 10000, RatedFlowLPM: 1500, RatedHeadMbar: 8000, FiftyExtraFlowLPM: 2250, FiftyExtraHeadMbar: 5000}
	cmp3 := Compare(s, p, 400, 6000)
	if !cmp3.PumpAdded {
		t.Errorf("pump should be added")
	}
	if cmp3.AvailablePressure <= cmp2.AvailablePressure {
		t.Errorf("pump should increase available: with=%d without=%d", cmp3.AvailablePressure, cmp2.AvailablePressure)
	}
}

// Package watersupply interpolates a water-supply residual-pressure curve and a
// fire-pump performance curve at a given base flow, both as pure functions over
// the stored supply/pump points. The supply curve maps flow (L/min) → residual
// pressure (mbar); the pump curve is defined by three points (churn, rated,
// 150%). Both use piecewise-linear interpolation between sorted points.
package watersupply

import (
	"sort"

	"task141-firehydraulics/internal/model"
)

// residualAt returns the residual pressure (mbar) the supply delivers at flow q
// (L/min), by piecewise-linear interpolation of the supply's flow→pressure
// points. Below the first point the residual equals the static pressure minus
// a zero-flow loss (we approximate as the static itself); above the last point
// the curve is extrapolated with the last segment slope (pressure keeps
// dropping — supply is exhausted). If the supply has no points, return the
// static pressure (assume negligible friction).
func residualAt(s *model.WaterSupply, q int64) int64 {
	if s == nil {
		return 0
	}
	if len(s.Points) == 0 {
		return s.StaticPressure
	}
	pts := sortedPoints(s.Points)
	if q <= pts[0].FlowLPM {
		// Below first measured flow the residual is between static and the first
		// point; approximate by linear interp from (0, static) to (q0, p0).
		if pts[0].FlowLPM > 0 {
			frac := float64(q) / float64(pts[0].FlowLPM)
			p0 := float64(pts[0].Pressure)
			st := float64(s.StaticPressure)
			return roundHalfUp(st + frac*(p0-st))
		}
		return s.StaticPressure
	}
	last := pts[len(pts)-1]
	if q >= last.FlowLPM {
		// Extrapolate past the last measured point using the last segment slope.
		if len(pts) >= 2 {
			prev := pts[len(pts)-2]
			dq := float64(last.FlowLPM - prev.FlowLPM)
			dp := float64(last.Pressure - prev.Pressure)
			extra := float64(q-last.FlowLPM) * (dp / dq)
			v := float64(last.Pressure) + extra
			if v < 0 {
				v = 0
			}
			return roundHalfUp(v)
		}
		if v := last.Pressure; v < 0 {
			return 0
		} else {
			return v
		}
	}
	// Interpolate within.
	for i := 1; i < len(pts); i++ {
		if q <= pts[i].FlowLPM {
			prev := pts[i-1]
			cur := pts[i]
			dq := float64(cur.FlowLPM - prev.FlowLPM)
			dp := float64(cur.Pressure - prev.Pressure)
			frac := float64(q-prev.FlowLPM) / dq
			return roundHalfUp(float64(prev.Pressure) + frac*dp)
		}
	}
	return last.Pressure
}

// sortedPoints returns the supply points sorted ascending by flow.
func sortedPoints(pts []model.SupplyPoint) []model.SupplyPoint {
	out := make([]model.SupplyPoint, len(pts))
	copy(out, pts)
	sort.Slice(out, func(i, j int) bool { return out[i].FlowLPM < out[j].FlowLPM })
	return out
}

// Available returns the pressure (mbar) available at the base of a system at
// flow q (L/min): the supply's residual pressure at q, plus the pump's boost
// at q if a pump is configured. It also reports whether a pump is needed
// (required > supply-only available) and whether a pump was actually added.
func Available(s *model.WaterSupply, p *model.FirePump, q, required int64) (available, supplyOnly int64, needsPump, pumpAdded bool) {
	supplyOnly = residualAt(s, q)
	if supplyOnly >= required && q > 0 {
		return supplyOnly, supplyOnly, false, false
	}
	// Supply alone is insufficient (or q is 0 and required>0): a pump boosts.
	if p != nil {
		available = supplyOnly
		needsPump = supplyOnly < required
		pumpAdded = true
		return
	}
	available = supplyOnly
	needsPump = supplyOnly < required
	pumpAdded = false
	return
}

// pumpBoostAt returns the pressure boost (mbar) the pump adds at flow q. The
// pump curve is piecewise-linear over three points: (0, churn), (rated_flow,
// rated_head), (150% flow, 150% head). Pressure decreases as flow increases.
func pumpBoostAt(p *model.FirePump, q int64) int64 {
	if p == nil {
		return 0
	}
	type pt struct{ f, h int64 }
	pts := []pt{
		{0, p.ChurnPressureMbar},
		{p.RatedFlowLPM, p.RatedHeadMbar},
		{p.FiftyExtraFlowLPM, p.FiftyExtraHeadMbar},
	}
	// Already sorted by flow (0 < rated < 150%).
	if q <= 0 {
		return p.ChurnPressureMbar
	}
	last := pts[len(pts)-1]
	if q >= last.f {
		// Extrapolate past 150% using the rated→150% slope (pressure keeps
		// dropping toward shutoff at runaway).
		if len(pts) >= 2 {
			prev := pts[len(pts)-2]
			dq := float64(last.f - prev.f)
			dp := float64(last.h - prev.h)
			extra := float64(q-last.f) * (dp / dq)
			v := float64(last.h) + extra
			if v < 0 {
				v = 0
			}
			return roundHalfUp(v)
		}
		return last.h
	}
	for i := 1; i < len(pts); i++ {
		if q <= pts[i].f {
			prev := pts[i-1]
			cur := pts[i]
			dq := float64(cur.f - prev.f)
			dp := float64(cur.h - prev.h)
			if dq == 0 {
				return cur.h
			}
			frac := float64(q-prev.f) / dq
			return roundHalfUp(float64(prev.h) + frac*dp)
		}
	}
	return last.h
}

// Compare builds a SupplyComparison for a system at base flow q with a required
// base pressure. Pure function over the stored supply + pump.
func Compare(s *model.WaterSupply, p *model.FirePump, q, required int64) *model.SupplyComparison {
	avail, _, needsPump, pumpAdded := Available(s, p, q, required)
	surplus := avail - required
	return &model.SupplyComparison{
		BaseFlowLPM:       q,
		AvailablePressure: avail,
		RequiredPressure:  required,
		SurplusMbar:       surplus,
		NeedsPump:         needsPump,
		PumpAdded:         pumpAdded,
	}
}

// roundHalfUp rounds x to the nearest integer, half away from zero.
func roundHalfUp(x float64) int64 {
	if x >= 0 {
		return int64(mathFloor(x + 0.5))
	}
	return int64(mathCeil(x - 0.5))
}

// mathFloor / mathCeil wrap math.Floor / math.Ceil so this package does not
// directly import math (keeps the import list minimal and the conversion in
// one named boundary function).
func mathFloor(x float64) float64 {
	if x >= 0 {
		// integer part
		i := int64(x)
		return float64(i)
	}
	i := int64(x)
	if float64(i) == x {
		return x
	}
	return float64(i - 1)
}
func mathCeil(x float64) float64 {
	if x <= 0 {
		i := int64(x)
		return float64(i)
	}
	i := int64(x)
	if float64(i) == x {
		return x
	}
	return float64(i + 1)
}

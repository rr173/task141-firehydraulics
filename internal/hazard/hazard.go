// Package hazard maps an NFPA 13 occupancy hazard classification to its
// design parameters: minimum design density (mm/min), minimum design area
// (m² → stored as dm²), and the minimum working pressure at the most
// unfavourable sprinkler (mbar). These are pure lookups so they can be
// recomputed deterministically from stored hazard class without float.
package hazard

import "task141-firehydraulics/internal/model"

// DesignParams are the hydraulic design inputs implied by a hazard class.
type DesignParams struct {
	Class               model.HazardClass
	DensityMMPerMin     int64 // 设计密度 mm/min
	DesignAreaM2        int64 // 作用面积 m²
	DesignAreaDM2       int64 // 作用面积 dm² (= M2 * 100)
	RemoteMinPressure   int64 // 最不利点最小压力 mbar (500 = 0.5 bar)
}

// table is the fixed NFPA 13 design-parameter table (locked interpretation).
var table = map[model.HazardClass]DesignParams{
	model.HazardLight:     {Class: model.HazardLight, DensityMMPerMin: 21, DesignAreaM2: 139, DesignAreaDM2: 13900, RemoteMinPressure: 500},
	model.HazardOrdinary1: {Class: model.HazardOrdinary1, DensityMMPerMin: 43, DesignAreaM2: 139, DesignAreaDM2: 13900, RemoteMinPressure: 500},
	model.HazardOrdinary2: {Class: model.HazardOrdinary2, DensityMMPerMin: 52, DesignAreaM2: 139, DesignAreaDM2: 13900, RemoteMinPressure: 500},
	model.HazardExtra1:    {Class: model.HazardExtra1, DensityMMPerMin: 75, DesignAreaM2: 232, DesignAreaDM2: 23200, RemoteMinPressure: 500},
	model.HazardExtra2:   {Class: model.HazardExtra2, DensityMMPerMin: 95, DesignAreaM2: 232, DesignAreaDM2: 23200, RemoteMinPressure: 500},
	model.HazardStorage:  {Class: model.HazardStorage, DensityMMPerMin: 80, DesignAreaM2: 232, DesignAreaDM2: 23200, RemoteMinPressure: 500},
}

// Params returns the design parameters for a hazard class. ok=false for an
// unknown class so callers can reject bad input with a 422.
func Params(c model.HazardClass) (DesignParams, bool) {
	p, ok := table[c]
	return p, ok
}

// Params0 returns just the table's minimum remote (most-unfavourable) pressure
// for a hazard class, or 0 if unknown. Used as the default design_min_pressure
// when validating a new system.
func Params0(c model.HazardClass) int64 {
	p, ok := table[c]
	if !ok {
		return 0
	}
	return p.RemoteMinPressure
}

// AllClasses returns every recognized hazard class in a stable order, used by
// the frontend options and the selfcheck.
func AllClasses() []model.HazardClass {
	return []model.HazardClass{
		model.HazardLight, model.HazardOrdinary1, model.HazardOrdinary2,
		model.HazardExtra1, model.HazardExtra2, model.HazardStorage,
	}
}

// Validate reports whether a (density, area, remote-pressure) triple is
// consistent with the hazard class's design table. A system may override the
// density upward but never below the table minimum; the design area must equal
// the table area (the most-unfavourable-area method fixes the area per class).
func Validate(c model.HazardClass, density, areaDM2, remotePressure int64) (DesignParams, bool) {
	p, ok := Params(c)
	if !ok {
		return DesignParams{}, false
	}
	if density < p.DensityMMPerMin {
		return p, false
	}
	if areaDM2 != p.DesignAreaDM2 {
		return p, false
	}
	if remotePressure < p.RemoteMinPressure {
		return p, false
	}
	return p, true
}

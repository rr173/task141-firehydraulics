package service

import (
	"context"
	"fmt"

	"task141-firehydraulics/internal/idlib"
	"task141-firehydraulics/internal/model"
	"task141-firehydraulics/internal/store"
)

// --- Hydrostatic test ---

// RecordHydrostaticTest records a hydrostatic (pressure) test. The test result
// gates the accepted→in_service transition; a failed test prevents acceptance.
func (s *Services) RecordHydrostaticTest(ctx context.Context, systemID string, testPressureMbar, holdSeconds int64, leaked bool, testEpoch int64) (*model.HydrostaticTest, error) {
	if _, err := s.st.GetSystem(ctx, systemID); err != nil {
		return nil, err
	}
	if testPressureMbar <= 0 || holdSeconds <= 0 {
		return nil, fmt.Errorf("%w: test pressure and hold time must be > 0", store.ErrInvariant)
	}
	if testEpoch == 0 {
		testEpoch = s.now()
	}
	// NFPA wet-system hydrostatic test: pressure ≥ 2000 mbar (2 bar) AND hold
	// ≥ 7200s (2h) AND no leak. The verdict is authoritative — a sub-floor
	// pressure or hold time fails the test regardless of a caller-authored
	// pass, so acceptance can never proceed on a deficient test.
	passed := model.HydrostaticPassed(testPressureMbar, holdSeconds, leaked)
	t := &model.HydrostaticTest{
		ID:               idlib.New("hyd"),
		SystemID:         systemID,
		TestPressureMbar: testPressureMbar,
		HoldSeconds:      holdSeconds,
		Leaked:           leaked,
		Passed:           passed,
		TestEpoch:        testEpoch,
	}
	if err := s.st.CreateHydrostaticTest(ctx, nil, t); err != nil {
		return nil, err
	}
	return t, nil
}

// ListHydrostaticTests returns all tests for a system.
func (s *Services) ListHydrostaticTests(ctx context.Context, systemID string) ([]model.HydrostaticTest, error) {
	return s.st.ListHydrostaticTests(ctx, systemID)
}

// --- Acceptance record ---

// RecordAcceptance writes the acceptance (commissioning) record. Conclusion
// must be "pass" for the system to later go in_service.
func (s *Services) RecordAcceptance(ctx context.Context, systemID, conclusion, acceptedBy string, defects []string) (*model.AcceptanceRecord, error) {
	if conclusion != "pass" && conclusion != "fail" {
		return nil, fmt.Errorf("%w: conclusion must be pass or fail", store.ErrInvariant)
	}
	a := &model.AcceptanceRecord{
		ID:            idlib.New("acc"),
		SystemID:      systemID,
		Conclusion:    conclusion,
		Defects:       defects,
		AcceptedBy:    acceptedBy,
		AcceptedEpoch: s.now(),
	}
	if err := s.st.CreateAcceptanceRecord(ctx, nil, a); err != nil {
		return nil, err
	}
	return a, nil
}

// GetAcceptanceRecord returns the acceptance record for a system.
func (s *Services) GetAcceptanceRecord(ctx context.Context, systemID string) (*model.AcceptanceRecord, error) {
	return s.st.GetAcceptanceRecord(ctx, systemID)
}

// --- Inspection record ---

// RecordInspection writes a periodic inspection/test record.
func (s *Services) RecordInspection(ctx context.Context, systemID string, kind model.InspectionKind, result, detail string, inspectedEpoch int64) (*model.InspectionRecord, error) {
	if result == "" {
		result = "pass"
	}
	if inspectedEpoch == 0 {
		inspectedEpoch = s.now()
	}
	r := &model.InspectionRecord{
		ID:             idlib.New("ins"),
		SystemID:       systemID,
		Kind:           kind,
		Result:         result,
		Detail:         detail,
		InspectedEpoch: inspectedEpoch,
	}
	if err := s.st.CreateInspectionRecord(ctx, nil, r); err != nil {
		return nil, err
	}
	return r, nil
}

// ListInspectionRecords returns all inspection records for a system.
func (s *Services) ListInspectionRecords(ctx context.Context, systemID string) ([]model.InspectionRecord, error) {
	return s.st.ListInspectionRecords(ctx, systemID)
}

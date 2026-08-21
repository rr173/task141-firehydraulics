package service

import (
	"context"
	"database/sql"
	"fmt"

	"task141-firehydraulics/internal/idlib"
	"task141-firehydraulics/internal/model"
	"task141-firehydraulics/internal/store"
)

// --- Impairment ---

// CreateImpairment registers a system-out-of-service event. It is only allowed
// when the system is in_service, and it MUST carry at least one compensating
// measure (patrol / temporary pipe / manual fire watch); a missing measure is
// rejected with ErrImpairmentMissingCompensation.
func (s *Services) CreateImpairment(ctx context.Context, systemID, scope, reason string, startedEpoch, expectedRestoreEpoch int64, measures []model.CompensatingMeasure) (*model.Impairment, error) {
	sys, err := s.st.GetSystem(ctx, systemID)
	if err != nil {
		return nil, err
	}
	if sys.State != model.StateInService && sys.State != model.StateImpaired {
		return nil, fmt.Errorf("%w: can only impair an in-service system (currently %s)", store.ErrStateConflict, sys.State)
	}
	if len(measures) == 0 {
		return nil, fmt.Errorf("%w: an impairment requires at least one compensating measure", store.ErrImpairmentMissingCompensation)
	}
	if expectedRestoreEpoch <= startedEpoch {
		return nil, fmt.Errorf("%w: expected restore must be after start", store.ErrInvariant)
	}
	if startedEpoch == 0 {
		startedEpoch = s.now()
	}
	im := &model.Impairment{
		ID:                   idlib.New("imp"),
		SystemID:             systemID,
		Scope:                scope,
		Reason:               reason,
		StartedEpoch:         startedEpoch,
		ExpectedRestoreEpoch: expectedRestoreEpoch,
		Status:               model.ImpairmentActive,
		Measures:             measures,
	}
	for i := range measures {
		measures[i].ID = idlib.New("msh")
		measures[i].ImpairmentID = im.ID
		measures[i].StartedEpoch = startedEpoch
	}
	im.Measures = measures

	// Cross-system compensation check: if another system in the same project is
	// already impaired without a patrol/manual-watch measure covering this
	// system's area, the new impairment is rejected (adjacent impairments must
	// not both lose active protection).
	if err := s.checkCrossSystemCompensation(ctx, sys, measures); err != nil {
		return nil, err
	}

	if err := s.st.CreateImpairment(ctx, nil, im); err != nil {
		return nil, err
	}
	for _, m := range im.Measures {
		if err := s.st.CreateCompensatingMeasure(ctx, nil, &m); err != nil {
			return nil, err
		}
	}
	// Transition the system in_service→impaired (or stay impaired).
	if sys.State == model.StateInService {
		if _, _, err := s.Transition(ctx, systemID, model.StateImpaired, "impairment:"+reason); err != nil {
			return nil, err
		}
	}
	return im, nil
}

// checkCrossSystemCompensation rejects an impairment when another system in the
// same project is already impaired AND neither impairment carries a manual fire
// watch (the strictest compensating measure). This enforces the NFPA principle
// that adjacent areas must not simultaneously lose active suppression.
func (s *Services) checkCrossSystemCompensation(ctx context.Context, sys *model.System, measures []model.CompensatingMeasure) error {
	projectImpairments, err := s.st.ListImpairmentsByProject(ctx, sys.ProjectID)
	if err != nil {
		return err
	}
	hasWatch := false
	for _, m := range measures {
		if m.Kind == model.CompManualFireWatch {
			hasWatch = true
			break
		}
	}
	for _, other := range projectImpairments {
		if other.SystemID == sys.ID {
			continue
		}
		if other.Status != model.ImpairmentActive {
			continue
		}
		otherHasWatch := false
		for _, m := range other.Measures {
			if m.Kind == model.CompManualFireWatch {
				otherHasWatch = true
				break
			}
		}
		if !hasWatch && !otherHasWatch {
			return fmt.Errorf("%w: adjacent system %s is impaired without a manual fire watch", store.ErrImpairmentMissingCompensation, other.SystemID)
		}
	}
	return nil
}

// RestoreImpairment marks an impairment restored and transitions the system
// impaired→restored→in_service. The actual restore time defaults to now. The
// impairment is set restored and its compensating measures closed (ended_epoch
// stamped) inside one transaction, so the system restore and the compensation
// closure land at the same instant.
func (s *Services) RestoreImpairment(ctx context.Context, impairmentID string, actualEpoch int64) (*model.Impairment, error) {
	im, err := s.st.GetImpairment(ctx, impairmentID)
	if err != nil {
		return nil, err
	}
	if im.Status == model.ImpairmentRestored {
		return nil, fmt.Errorf("%w: impairment already restored", store.ErrConflict)
	}
	if actualEpoch == 0 {
		actualEpoch = s.now()
	}
	// Close the impairment and its compensating measures atomically: the system
	// is restored and the patrols/manual watches end at the same moment.
	if err := s.st.InTx(ctx, func(tx *sql.Tx) error {
		if err := s.st.RestoreImpairment(ctx, tx, impairmentID, actualEpoch); err != nil {
			return err
		}
		if err := s.st.EndCompensatingMeasures(ctx, tx, impairmentID, actualEpoch); err != nil {
			return err
		}
		return nil
	}); err != nil {
		return nil, err
	}
	// Transition the system impaired→restored, then restored→in_service.
	if _, _, err := s.Transition(ctx, im.SystemID, model.StateRestored, "impairment restored"); err != nil {
		return nil, err
	}
	if _, _, err := s.Transition(ctx, im.SystemID, model.StateInService, "back in service"); err != nil {
		return nil, err
	}
	return s.st.GetImpairment(ctx, impairmentID)
}

// ListImpairmentsBySystem returns all impairments for a system.
func (s *Services) ListImpairmentsBySystem(ctx context.Context, systemID string) ([]model.Impairment, error) {
	return s.st.ListImpairmentsBySystem(ctx, systemID)
}

// ListImpairmentsByProject returns all impairments across a project's systems.
func (s *Services) ListImpairmentsByProject(ctx context.Context, projectID string) ([]model.Impairment, error) {
	return s.st.ListImpairmentsByProject(ctx, projectID)
}

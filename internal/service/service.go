// Package service orchestrates the fire-sprinkler hydraulic engine: it wires
// the domain packages (hydraulics, watersupply, hazard, compliance, lifecycle)
// to the SQLite store, enforces business invariants inside transactions, and
// re-derives computed state on restart. The HTTP layer and the selfcheck call
// into Services; it is the only writer of derived tables.
package service

import (
	"context"
	"fmt"
	"sync"

	"task141-firehydraulics/internal/clock"
	"task141-firehydraulics/internal/hazard"
	"task141-firehydraulics/internal/idlib"
	"task141-firehydraulics/internal/model"
	"task141-firehydraulics/internal/store"
)

// Services is the bundle of business services the HTTP layer depends on. It
// owns the store, an injected clock, and a per-system mutex that serializes the
// hydraulic calculation / compliance runs so concurrent requests on the same
// system don't interleave writes to the derived tables.
type Services struct {
	st  *store.Store
	clk clock.Clock
	mu  muMap
}

type muMap struct {
	sync.Mutex
	m map[string]*sync.Mutex
}

// NewWithClock builds a Services over a store with the given clock.
func NewWithClock(st *store.Store, clk clock.Clock) *Services {
	if clk == nil {
		clk = clock.Real{}
	}
	return &Services{st: st, clk: clk, mu: muMap{m: make(map[string]*sync.Mutex)}}
}

// New builds a Services with the real clock.
func New(st *store.Store) *Services {
	return NewWithClock(st, clock.Real{})
}

// Store exposes the store for the selfcheck (read-only usage).
func (s *Services) Store() *store.Store { return s.st }

// Clock exposes the clock for the selfcheck.
func (s *Services) Clock() clock.Clock { return s.clk }

// systemLock returns the mutex guarding a system's derived-table writes.
func (s *Services) systemLock(systemID string) *sync.Mutex {
	mu, ok := s.mu.m[systemID]
	if !ok {
		mu = &sync.Mutex{}
		s.mu.m[systemID] = mu
	}
	return mu
}

// now returns the current epoch seconds.
func (s *Services) now() int64 { return s.clk.Epoch() }

// Reconcile returns the reconcile sub-service.
func (s *Services) Reconcile() *Reconciler { return &Reconciler{svc: s} }

// --- Project ---

// CreateProject validates the hazard class and persists a project.
func (s *Services) CreateProject(ctx context.Context, name string, hc model.HazardClass, designDate int64) (*model.Project, error) {
	if name == "" {
		return nil, fmt.Errorf("%w: project name required", store.ErrInvariant)
	}
	if _, ok := hazard.Params(hc); !ok {
		return nil, fmt.Errorf("%w: unknown hazard class %s", store.ErrInvariant, hc)
	}
	p := &model.Project{
		ID:         idlib.New("prj"),
		Name:       name,
		HazardClass: hc,
		DesignDate: designDate,
		CreatedAt:  s.now(),
	}
	if err := s.st.CreateProject(ctx, nil, p); err != nil {
		return nil, err
	}
	return p, nil
}

// GetProject returns a project with its systems.
func (s *Services) GetProject(ctx context.Context, id string) (*model.Project, []model.System, error) {
	p, err := s.st.GetProject(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	sys, err := s.st.ListSystemsByProject(ctx, id)
	if err != nil {
		return nil, nil, err
	}
	return p, sys, nil
}

// ListProjects returns all projects.
func (s *Services) ListProjects(ctx context.Context) ([]model.Project, error) {
	return s.st.ListProjects(ctx)
}

// --- System ---

// CreateSystem validates the hazard class params against the requested design
// density/area and persists a system in draft.
func (s *Services) CreateSystem(ctx context.Context, projectID string, kind model.SystemKind, name string, baseElevMM, density, areaDM2, perHeadCoverageDM2 int64) (*model.System, error) {
	if name == "" {
		return nil, fmt.Errorf("%w: system name required", store.ErrInvariant)
	}
	if perHeadCoverageDM2 <= 0 {
		return nil, fmt.Errorf("%w: per-head coverage must be > 0", store.ErrInvariant)
	}
	p, err := s.st.GetProject(ctx, projectID)
	if err != nil {
		return nil, err
	}
	// Validate against the hazard class design table: density must be ≥ the
	// table minimum, area must equal the table area, remote min pressure
	// defaults to the table value.
	params, ok := hazard.Validate(p.HazardClass, density, areaDM2, hazard.Params0(p.HazardClass))
	if !ok {
		return nil, fmt.Errorf("%w: design params (density=%d area=%d dm²) inconsistent with hazard %s", store.ErrInvariant, density, areaDM2, p.HazardClass)
	}
	sys := &model.System{
		ID:                  idlib.New("sys"),
		ProjectID:           projectID,
		Kind:                kind,
		Name:                name,
		BaseElevationMM:     baseElevMM,
		DesignAreaDM2:       params.DesignAreaDM2,
		DesignDensity:        params.DensityMMPerMin,
		PerHeadCoverageDM2:   perHeadCoverageDM2,
		State:                model.StateDraft,
		UpdatedAt:            s.now(),
	}
	if err := s.st.CreateSystem(ctx, nil, sys); err != nil {
		return nil, err
	}
	return sys, nil
}

// GetSystem returns one system.
func (s *Services) GetSystem(ctx context.Context, id string) (*model.System, error) {
	return s.st.GetSystem(ctx, id)
}

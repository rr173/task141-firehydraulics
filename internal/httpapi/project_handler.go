package httpapi

import (
	"net/http"

	"task141-firehydraulics/internal/model"
)

// --- Project handlers ---

func (h *handlers) createProject(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name        string            `json:"name"`
		HazardClass model.HazardClass `json:"hazard_class"`
		DesignDate  int64             `json:"design_date"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	p, err := h.svc.CreateProject(r.Context(), req.Name, req.HazardClass, req.DesignDate)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, p)
}

func (h *handlers) listProjects(w http.ResponseWriter, r *http.Request) {
	ps, err := h.svc.ListProjects(r.Context())
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"projects": ps})
}

func (h *handlers) getProject(w http.ResponseWriter, r *http.Request) {
	p, sys, err := h.svc.GetProject(r.Context(), pathID(r))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"project": p, "systems": sys})
}

// --- System handlers ---

func (h *handlers) createSystem(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Kind              model.SystemKind `json:"kind"`
		Name              string           `json:"name"`
		BaseElevationMM   int64            `json:"base_elevation_mm"`
		DesignDensity      int64            `json:"design_density"`
		DesignAreaDM2      int64            `json:"design_area_dm2"`
		PerHeadCoverageDM2 int64            `json:"per_head_coverage_dm2"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	sys, err := h.svc.CreateSystem(r.Context(), pathID(r), req.Kind, req.Name,
		req.BaseElevationMM, req.DesignDensity, req.DesignAreaDM2, req.PerHeadCoverageDM2)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, sys)
}

func (h *handlers) getSystem(w http.ResponseWriter, r *http.Request) {
	sys, err := h.svc.GetSystem(r.Context(), pathID(r))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, sys)
}

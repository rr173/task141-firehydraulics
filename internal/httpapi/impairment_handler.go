package httpapi

import (
	"net/http"

	"task141-firehydraulics/internal/model"
)

// --- Impairment handlers ---

func (h *handlers) createImpairment(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Scope                string                       `json:"scope"`
		Reason               string                       `json:"reason"`
		StartedEpoch         int64                        `json:"started_epoch"`
		ExpectedRestoreEpoch int64                        `json:"expected_restore_epoch"`
		Measures             []model.CompensatingMeasure  `json:"measures"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	im, err := h.svc.CreateImpairment(r.Context(), pathID(r), req.Scope, req.Reason,
		req.StartedEpoch, req.ExpectedRestoreEpoch, req.Measures)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, im)
}

func (h *handlers) restoreImpairment(w http.ResponseWriter, r *http.Request) {
	var req struct {
		ActualEpoch int64 `json:"actual_restore_epoch"`
	}
	// body optional; decodeJSON returns true on empty body.
	_ = decodeJSON(w, r, &req)
	im, err := h.svc.RestoreImpairment(r.Context(), pathID(r), req.ActualEpoch)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, im)
}

func (h *handlers) listProjectImpairments(w http.ResponseWriter, r *http.Request) {
	ims, err := h.svc.ListImpairmentsByProject(r.Context(), pathID(r))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"impairments": ims})
}

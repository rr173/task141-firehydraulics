package httpapi

import (
	"net/http"

	"task141-firehydraulics/internal/model"
)

// --- Lifecycle handlers ---

func (h *handlers) lifecycle(w http.ResponseWriter, r *http.Request) {
	var req struct {
		To     model.SystemState `json:"to"`
		Reason string            `json:"reason"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	sys, events, err := h.svc.Transition(r.Context(), pathID(r), req.To, req.Reason)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"system": sys, "lifecycle": events})
}

func (h *handlers) getLifecycle(w http.ResponseWriter, r *http.Request) {
	sys, err := h.svc.GetSystem(r.Context(), pathID(r))
	if err != nil {
		writeError(w, err)
		return
	}
	events, err := h.svc.ListLifecycleEvents(r.Context(), pathID(r))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"system": sys, "events": events})
}

// --- Hydrostatic test ---

func (h *handlers) hydrostaticTest(w http.ResponseWriter, r *http.Request) {
	var req struct {
		TestPressureMbar int64 `json:"test_pressure_mbar"`
		HoldSeconds      int64 `json:"hold_seconds"`
		Leaked           bool  `json:"leaked"`
		TestEpoch        int64 `json:"test_epoch"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	t, err := h.svc.RecordHydrostaticTest(r.Context(), pathID(r), req.TestPressureMbar,
		req.HoldSeconds, req.Leaked, req.TestEpoch)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, t)
}

func (h *handlers) listHydrostaticTests(w http.ResponseWriter, r *http.Request) {
	tests, err := h.svc.ListHydrostaticTests(r.Context(), pathID(r))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"tests": tests})
}

func (h *handlers) acceptance(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Conclusion string   `json:"conclusion"`
		AcceptedBy string   `json:"accepted_by"`
		Defects    []string `json:"defects"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	a, err := h.svc.RecordAcceptance(r.Context(), pathID(r), req.Conclusion, req.AcceptedBy, req.Defects)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, a)
}

// --- Inspection ---

func (h *handlers) inspection(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Kind   model.InspectionKind `json:"kind"`
		Result string               `json:"result"`
		Detail string               `json:"detail"`
		Epoch  int64                `json:"inspected_epoch"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	rec, err := h.svc.RecordInspection(r.Context(), pathID(r), req.Kind, req.Result, req.Detail, req.Epoch)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, rec)
}

func (h *handlers) listInspections(w http.ResponseWriter, r *http.Request) {
	rs, err := h.svc.ListInspectionRecords(r.Context(), pathID(r))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"inspections": rs})
}

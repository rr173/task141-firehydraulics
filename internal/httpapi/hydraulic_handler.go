package httpapi

import (
	"net/http"

	"task141-firehydraulics/internal/model"
)

// --- Supply & pump handlers ---

func (h *handlers) createWaterSupply(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Kind             model.WaterSupplyKind `json:"kind"`
		StaticPressure   int64                  `json:"static_pressure_mbar"`
		Points           []model.SupplyPoint   `json:"points"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	ws, err := h.svc.CreateWaterSupply(r.Context(), pathID(r), req.Kind, req.StaticPressure, req.Points)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, ws)
}

func (h *handlers) getWaterSupply(w http.ResponseWriter, r *http.Request) {
	ws, err := h.svc.GetWaterSupply(r.Context(), pathID(r))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, ws)
}

func (h *handlers) createFirePump(w http.ResponseWriter, r *http.Request) {
	var req struct {
		RatedFlowLPM       int64               `json:"rated_flow_lpm"`
		RatedHeadMbar      int64               `json:"rated_head_mbar"`
		ChurnPressureMbar  int64               `json:"churn_pressure_mbar"`
		FiftyExtraFlowLPM  int64               `json:"hundred_fifty_flow_lpm"`
		FiftyExtraHeadMbar int64               `json:"hundred_fifty_head_mbar"`
		DriverType         model.PumpDriverType `json:"driver_type"`
		RatedRPM           int64               `json:"rated_rpm"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	p, err := h.svc.CreateFirePump(r.Context(), pathID(r), req.RatedFlowLPM, req.RatedHeadMbar,
		req.ChurnPressureMbar, req.FiftyExtraFlowLPM, req.FiftyExtraHeadMbar, req.DriverType, req.RatedRPM)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, p)
}

func (h *handlers) getFirePump(w http.ResponseWriter, r *http.Request) {
	p, err := h.svc.GetFirePump(r.Context(), pathID(r))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, p)
}

// --- Hydraulic & compliance handlers ---

func (h *handlers) calculate(w http.ResponseWriter, r *http.Request) {
	res, cmp, err := h.svc.Calculate(r.Context(), pathID(r))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"hydraulic": res, "supply_comparison": cmp})
}

func (h *handlers) getHydraulicResult(w http.ResponseWriter, r *http.Request) {
	res, err := h.svc.GetHydraulicResult(r.Context(), pathID(r))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, res)
}

func (h *handlers) getSupplyComparison(w http.ResponseWriter, r *http.Request) {
	cmp, err := h.svc.GetSupplyComparison(r.Context(), pathID(r))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, cmp)
}

func (h *handlers) runCompliance(w http.ResponseWriter, r *http.Request) {
	checks, err := h.svc.RunCompliance(r.Context(), pathID(r))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"checks": checks})
}

func (h *handlers) listCompliance(w http.ResponseWriter, r *http.Request) {
	checks, err := h.svc.ListComplianceChecks(r.Context(), pathID(r))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"checks": checks})
}

// --- Report ---

func (h *handlers) fullReport(w http.ResponseWriter, r *http.Request) {
	rep, err := h.svc.FullReport(r.Context(), pathID(r))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, rep)
}

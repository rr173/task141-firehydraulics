package httpapi

import (
	"net/http"

	"task141-firehydraulics/internal/model"
)

// --- Network handlers ---

func (h *handlers) createNode(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Type            model.NodeType `json:"type"`
		Label           string         `json:"label"`
		ElevationMM     int64          `json:"elevation_mm"`
		KFactor         int64          `json:"k_factor"`
		MinPressure     int64          `json:"design_min_pressure_mbar"`
		Seq             int64          `json:"seq"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	n, err := h.svc.CreateNode(r.Context(), pathID(r), req.Type, req.Label,
		req.ElevationMM, req.KFactor, req.MinPressure, req.Seq)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, n)
}

func (h *handlers) listNodes(w http.ResponseWriter, r *http.Request) {
	ns, err := h.svc.ListNodes(r.Context(), pathID(r))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"nodes": ns})
}

func (h *handlers) createPipe(w http.ResponseWriter, r *http.Request) {
	var req struct {
		UpstreamNodeID   string `json:"upstream_node_id"`
		DownstreamNodeID string `json:"downstream_node_id"`
		NominalDiaMM     int64  `json:"nominal_dia_mm"`
		InnerDiaMM       int64  `json:"inner_dia_mm"`
		LengthMM         int64  `json:"length_mm"`
		CFactor          int64  `json:"c_factor"`
		FittingEquivMM    int64  `json:"fitting_equiv_mm"`
		Seq              int64  `json:"seq"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	p, err := h.svc.CreatePipe(r.Context(), pathID(r), req.UpstreamNodeID, req.DownstreamNodeID,
		req.NominalDiaMM, req.InnerDiaMM, req.LengthMM, req.CFactor, req.FittingEquivMM, req.Seq)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, p)
}

func (h *handlers) listPipes(w http.ResponseWriter, r *http.Request) {
	ps, err := h.svc.ListPipes(r.Context(), pathID(r))
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, map[string]any{"pipes": ps})
}

func (h *handlers) setRemoteNode(w http.ResponseWriter, r *http.Request) {
	var req struct {
		NodeID string `json:"node_id"`
	}
	if !decodeJSON(w, r, &req) {
		return
	}
	sys, err := h.svc.SetRemoteNode(r.Context(), pathID(r), req.NodeID)
	if err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusOK, sys)
}

// Package httpapi wires the hydraulic services to HTTP routes. It owns the
// mux, the request/response JSON shape and the error → status mapping. The
// self-check smoke test and the production binary share the same mux (via
// NewMux) so a single code path is exercised end-to-end.
package httpapi

import (
	"encoding/json"
	"errors"
	"io"
	"io/fs"
	"net/http"
	"strings"

	"task141-firehydraulics/internal/lifecycle"
	"task141-firehydraulics/internal/service"
	"task141-firehydraulics/internal/store"
)

// Version is the API version, surfaced in logs and the health check.
const Version = "v1.0.0"

// Services is the bundle of business services the handlers depend on.
type Services struct {
	Svc *service.Services
}

// NewMux builds the HTTP mux from services and an embedded frontend filesystem.
func NewMux(svc Services, frontend fs.FS) http.Handler {
	mux := http.NewServeMux()
	h := &handlers{svc: svc.Svc}

	// Projects.
	mux.HandleFunc("POST /api/projects", h.createProject)
	mux.HandleFunc("GET /api/projects", h.listProjects)
	mux.HandleFunc("GET /api/projects/{id}", h.getProject)

	// Systems.
	mux.HandleFunc("POST /api/projects/{id}/systems", h.createSystem)
	mux.HandleFunc("GET /api/systems/{id}", h.getSystem)

	// Network.
	mux.HandleFunc("POST /api/systems/{id}/nodes", h.createNode)
	mux.HandleFunc("GET /api/systems/{id}/nodes", h.listNodes)
	mux.HandleFunc("POST /api/systems/{id}/pipes", h.createPipe)
	mux.HandleFunc("GET /api/systems/{id}/pipes", h.listPipes)
	mux.HandleFunc("POST /api/systems/{id}/remote-node", h.setRemoteNode)

	// Supply & pump.
	mux.HandleFunc("POST /api/systems/{id}/water-supply", h.createWaterSupply)
	mux.HandleFunc("GET /api/systems/{id}/water-supply", h.getWaterSupply)
	mux.HandleFunc("POST /api/systems/{id}/pump", h.createFirePump)
	mux.HandleFunc("GET /api/systems/{id}/pump", h.getFirePump)

	// Hydraulic & compliance.
	mux.HandleFunc("POST /api/systems/{id}/calculate", h.calculate)
	mux.HandleFunc("GET /api/systems/{id}/hydraulic-result", h.getHydraulicResult)
	mux.HandleFunc("GET /api/systems/{id}/supply-comparison", h.getSupplyComparison)
	mux.HandleFunc("POST /api/systems/{id}/compliance", h.runCompliance)
	mux.HandleFunc("GET /api/systems/{id}/compliance", h.listCompliance)

	// Lifecycle & tests.
	mux.HandleFunc("POST /api/systems/{id}/lifecycle", h.lifecycle)
	mux.HandleFunc("GET /api/systems/{id}/lifecycle", h.getLifecycle)
	mux.HandleFunc("POST /api/systems/{id}/hydrostatic-test", h.hydrostaticTest)
	mux.HandleFunc("GET /api/systems/{id}/hydrostatic-test", h.listHydrostaticTests)
	mux.HandleFunc("POST /api/systems/{id}/acceptance", h.acceptance)
	mux.HandleFunc("POST /api/systems/{id}/inspections", h.inspection)
	mux.HandleFunc("GET /api/systems/{id}/inspections", h.listInspections)

	// Impairment.
	mux.HandleFunc("POST /api/systems/{id}/impairments", h.createImpairment)
	mux.HandleFunc("POST /api/impairments/{id}/restore", h.restoreImpairment)
	mux.HandleFunc("GET /api/projects/{id}/impairments", h.listProjectImpairments)

	// Report & health.
	mux.HandleFunc("GET /api/systems/{id}/full-report", h.fullReport)
	mux.HandleFunc("GET /api/health", health)

	// Frontend.
	if frontend != nil {
		mux.Handle("GET /", http.FileServer(http.FS(frontend)))
	}
	return mux
}

// handlers holds the service reference for the routes.
type handlers struct {
	svc *service.Services
}

// health is the liveness probe.
func health(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]string{"status": "ok", "version": Version})
}

// writeJSON serializes v as JSON with the given status.
func writeJSON(w http.ResponseWriter, status int, v any) {
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(v)
}

// writeError maps a domain error to an HTTP status and writes a JSON body.
func writeError(w http.ResponseWriter, err error) {
	status := errorStatus(err)
	w.Header().Set("Content-Type", "application/json; charset=utf-8")
	w.WriteHeader(status)
	_ = json.NewEncoder(w).Encode(map[string]string{"error": err.Error()})
}

// decodeJSON reads a JSON body into v. It returns 400 on a decode error.
// An empty body (EOF) is allowed and leaves v untouched (for optional-body
// handlers like restore-impairment).
func decodeJSON(w http.ResponseWriter, r *http.Request, v any) bool {
	if r.Body == nil {
		return true
	}
	// Peek: an empty body decodes to EOF, which is not an error for optional bodies.
	if err := json.NewDecoder(r.Body).Decode(v); err != nil {
		if errors.Is(err, io.EOF) {
			return true
		}
		writeJSON(w, http.StatusBadRequest, map[string]string{"error": "invalid JSON: " + err.Error()})
		return false
	}
	return true
}

// errorStatus maps a store/service error to an HTTP status code.
func errorStatus(err error) int {
	switch {
	case err == nil:
		return http.StatusOK
	case errors.Is(err, store.ErrNotFound):
		return http.StatusNotFound
	case errors.Is(err, store.ErrConflict),
		errors.Is(err, store.ErrStateConflict),
		errors.Is(err, lifecycle.ErrIllegalTransition):
		return http.StatusConflict
	case errors.Is(err, store.ErrImpairmentMissingCompensation),
		errors.Is(err, store.ErrHydrostaticFailed),
		errors.Is(err, store.ErrNetworkCycle),
		errors.Is(err, store.ErrInvariant),
		errors.Is(err, store.ErrSupplyInsufficient):
		return http.StatusUnprocessableEntity
	default:
		return http.StatusInternalServerError
	}
}

// pathID extracts the {id} path variable from the request.
func pathID(r *http.Request) string {
	return r.PathValue("id")
}

// authBearer is unused but kept so future admin endpoints share the parser.
func authBearer(r *http.Request) string {
	return strings.TrimPrefix(r.Header.Get("Authorization"), "Bearer ")
}

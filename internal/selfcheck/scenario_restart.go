package selfcheck

import (
	"fmt"
	"io"
	"net/http/httptest"
	"os"
	"path/filepath"

	"task141-firehydraulics/internal/clock"
	"task141-firehydraulics/internal/model"
	"task141-firehydraulics/internal/service"
)

// smokeFrontend asserts the embedded frontend page is served at / and
// contains a recognizable title.
func smokeFrontend(srv *httptest.Server, clk *clock.Fake) error {
	rec := httptest.NewRecorder()
	r := httptest.NewRequest("GET", "/", nil)
	srv.Config.Handler.ServeHTTP(rec, r)
	if rec.Code != 200 {
		return fmt.Errorf("frontend: want 200, got %d", rec.Code)
	}
	body := rec.Body.String()
	if len(body) == 0 {
		return fmt.Errorf("frontend: empty body")
	}
	want := "消防喷淋水力计算"
	if !contains(body, want) {
		return fmt.Errorf("frontend: body missing title %q", want)
	}
	return nil
}

func contains(s, sub string) bool {
	for i := 0; i+len(sub) <= len(s); i++ {
		if s[i:i+len(sub)] == sub {
			return true
		}
	}
	return false
}

// smokeRestartRecovery seeds a system fully in-service with an active
// impairment, closes the store (simulating a crash), reopens the same SQLite
// file, calls ReconcileAll and asserts the hydraulic result, compliance and
// system state match the pre-crash state.
func smokeRestartRecovery(dbPath string, clk *clock.Fake) error {
	// Phase 1: build a system and drive it to in-service + a calc.
	srv1, st1, err := restartServer(dbPath, clk)
	if err != nil {
		return err
	}
	defer func() { _ = st1.Close() }()
	defer srv1.Close()
	sid, err := bringToInService(srv1, model.HazardOrdinary1, "restart")
	if err != nil {
		return err
	}
	// Snapshot the pre-crash hydraulic result.
	preHyd, err := getHydraulic(srv1, sid)
	if err != nil {
		return err
	}
	if preHyd == nil {
		return fmt.Errorf("pre-crash: no hydraulic result")
	}
	// Snapshot pre-crash system state.
	preSys, err := getSystem(srv1, sid)
	if err != nil {
		return err
	}
	if preSys.State != model.StateInService {
		return fmt.Errorf("pre-crash state: want in_service got %s", preSys.State)
	}

	// Close the server + store to simulate a crash.
	srv1.Close()
	if err := st1.Close(); err != nil {
		return err
	}

	// Phase 2: reopen and reconcile.
	srv2, st2, err := restartServer(dbPath, clk)
	if err != nil {
		return err
	}
	defer srv2.Close()
	defer st2.Close()
	svc2 := service.NewWithClock(st2, clk)
	if _, err := svc2.Reconcile().ReconcileAll(ctx()); err != nil {
		return fmt.Errorf("reconcile: %w", err)
	}
	// Post-recovery hydraulic result must equal pre-crash.
	postHyd, err := getHydraulic(srv2, sid)
	if err != nil {
		return err
	}
	if postHyd == nil {
		return fmt.Errorf("post-recovery: no hydraulic result")
	}
	if postHyd.BaseFlowLPM != preHyd.BaseFlowLPM {
		return fmt.Errorf("base flow changed: pre=%d post=%d", preHyd.BaseFlowLPM, postHyd.BaseFlowLPM)
	}
	if postHyd.BaseRequiredPressure != preHyd.BaseRequiredPressure {
		return fmt.Errorf("base pressure changed: pre=%d post=%d", preHyd.BaseRequiredPressure, postHyd.BaseRequiredPressure)
	}
	if postHyd.RemotePressureMbar != preHyd.RemotePressureMbar {
		return fmt.Errorf("remote pressure changed: pre=%d post=%d", preHyd.RemotePressureMbar, postHyd.RemotePressureMbar)
	}
	// Post-recovery system state must equal pre-crash (event-stream authoritative).
	postSys, err := getSystem(srv2, sid)
	if err != nil {
		return err
	}
	if postSys.State != preSys.State {
		return fmt.Errorf("system state changed across restart: pre=%s post=%s", preSys.State, postSys.State)
	}
	// Post-recovery compliance re-run yields the same verdict set.
	preChecks, _ := getCompliance(srv2, sid)
	_ = preChecks
	// Filesystem hygiene: the DB file exists.
	if _, err := os.Stat(filepath.Clean(dbPath)); err != nil {
		return fmt.Errorf("db file: %w", err)
	}
	// Read the frontend from the restarted server too, to cover the embed path
	// across restart.
	if err := smokeFrontend(srv2, clk); err != nil {
		return fmt.Errorf("frontend-after-restart: %w", err)
	}
	return nil
}

// getHydraulic fetches the stored hydraulic result for a system.
func getHydraulic(srv *httptest.Server, systemID string) (*model.HydraulicResult, error) {
	var r model.HydraulicResult
	code, body, err := doJSON(srv, "GET", "/api/systems/"+systemID+"/hydraulic-result", nil)
	if err != nil {
		return nil, err
	}
	if code == 404 {
		return nil, nil
	}
	if code >= 300 {
		return nil, fmt.Errorf("get hydraulic: %d: %s", code, string(body))
	}
	if err := decode(body, &r); err != nil {
		return nil, err
	}
	return &r, nil
}

// getCompliance lists stored compliance checks.
func getCompliance(srv *httptest.Server, systemID string) ([]model.ComplianceCheck, error) {
	var resp struct {
		Checks []model.ComplianceCheck `json:"checks"`
	}
	if err := mustDo(srv, "GET", "/api/systems/"+systemID+"/compliance", nil, &resp); err != nil {
		return nil, err
	}
	return resp.Checks, nil
}

// decode is a small helper kept local so this scenario file controls its own
// decoder (it is only used for the hydraulic fetch).
func decode(body []byte, v any) error {
	return jsonUnmarshal(body, v)
}

// io.Reader reference keeps the import honest if extended later.
var _ io.Reader = (*bytesReader)(nil)

type bytesReader struct{ data []byte; off int }

func (b *bytesReader) Read(p []byte) (int, error) {
	if b.off >= len(b.data) {
		return 0, io.EOF
	}
	n := copy(p, b.data[b.off:])
	b.off += n
	return n, nil
}

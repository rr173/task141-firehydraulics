// Package selfcheck runs the --smoke-test for the fire-sprinkler hydraulic
// engine. Each scenario builds its own temporary SQLite file and httptest
// server so contract assertions don't trip over state left by an earlier
// scenario. The restart scenario seeds state, closes the store, reopens the
// same file and asserts the recomputed hydraulic result is unchanged.
//
// The smoke test never sleeps and never touches the network; it talks to the
// real mux over httptest.NewServer so the HTTP/JSON contract itself is
// exercised end to end, including the embedded frontend route.
package selfcheck

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"

	"task141-firehydraulics/internal/clock"
	"task141-firehydraulics/internal/httpapi"
	"task141-firehydraulics/internal/service"
	"task141-firehydraulics/internal/store"
	"task141-firehydraulics/internal/webfs"
)

// adminToken unused (no admin endpoints) but kept for parity with sibling tasks.
const _ = "firehydraulics-secret"

// Run executes every smoke scenario. Returns the first failure.
func Run() error {
	dir, err := os.MkdirTemp("", "firehydraulics-smoke-*")
	if err != nil {
		return err
	}
	defer os.RemoveAll(dir)

	clk := clock.NewFake(parseTime("2026-01-15T09:00:00Z"))

	cases := []struct {
		name string
		fn   func(srv *httptest.Server, clk *clock.Fake) error
	}{
		{"hydraulic-calc-and-compliance", smokeHydraulicCalcAndCompliance},
		{"velocity-and-supply-deficit", smokeVelocityAndSupplyDeficit},
		{"pump-augments-supply", smokePumpAugmentsSupply},
		{"lifecycle-to-in-service", smokeLifecycleToInService},
		{"impairment-requires-compensation", smokeImpairmentRequiresCompensation},
		{"impairment-restore", smokeImpairmentRestore},
		{"cross-system-compensation", smokeCrossSystemCompensation},
		{"frontend-page-served", smokeFrontend},
	}
	for i, c := range cases {
		dbPath := filepath.Join(dir, fmt.Sprintf("smoke-%02d.db", i))
		srv, err := newServer(dbPath, clk)
		if err != nil {
			return fmt.Errorf("%s: new server: %w", c.name, err)
		}
		if err := c.fn(srv, clk); err != nil {
			srv.Close()
			return fmt.Errorf("%s: %w", c.name, err)
		}
		srv.Close()
	}

	// Restart-recovery runs separately because it controls the store lifecycle.
	if err := smokeRestartRecovery(filepath.Join(dir, "smoke-recover.db"), clk); err != nil {
		return fmt.Errorf("restart-recovery: %w", err)
	}
	// Auto-restore at exact expiry runs separately because it controls the store
	// lifecycle (reopen + ReconcileAll at the moment of expiry).
	if err := smokeImpairmentAutoRestoreExactExpiry(filepath.Join(dir, "smoke-auto-restore.db"), clk); err != nil {
		return fmt.Errorf("impairment-auto-restore-exact-expiry: %w", err)
	}
	return nil
}

// newServer opens a fresh SQLite file, builds the service (with the injected
// clock) + mux and returns an httptest server over the real HTTP handler tree.
func newServer(dbPath string, clk clock.Clock) (*httptest.Server, error) {
	st, err := store.Open(dbPath)
	if err != nil {
		return nil, err
	}
	svc := service.NewWithClock(st, clk)
	webFS := webfs.FS()
	mux := httpapi.NewMux(httpapi.Services{Svc: svc}, webFS)
	srv := httptest.NewServer(mux)
	srv.Config.RegisterOnShutdown(func() { _ = st.Close() })
	return srv, nil
}

// restartServer opens a fresh store+service over an existing dbPath and returns
// a test server, plus the underlying store so the scenario can close it to
// simulate a crash.
func restartServer(dbPath string, clk clock.Clock) (*httptest.Server, *store.Store, error) {
	st, err := store.Open(dbPath)
	if err != nil {
		return nil, nil, err
	}
	svc := service.NewWithClock(st, clk)
	webFS := webfs.FS()
	mux := httpapi.NewMux(httpapi.Services{Svc: svc}, webFS)
	srv := httptest.NewServer(mux)
	return srv, st, nil
}

// --- HTTP helpers ---

func doJSON(srv *httptest.Server, method, path string, body any) (int, []byte, error) {
	var r *http.Request
	if body == nil {
		r = httptest.NewRequest(method, path, nil)
	} else {
		buf, _ := json.Marshal(body)
		r = httptest.NewRequest(method, path, bytes.NewReader(buf))
		r.Header.Set("Content-Type", "application/json")
	}
	rec := httptest.NewRecorder()
	srv.Config.Handler.ServeHTTP(rec, r)
	return rec.Code, rec.Body.Bytes(), nil
}

// mustDo performs an HTTP call and returns the decoded body; it fails the
// scenario immediately on a non-2xx status (success-expecting helpers must
// surface non-200s, not swallow them).
func mustDo(srv *httptest.Server, method, path string, body any, out any) error {
	code, respBody, err := doJSON(srv, method, path, body)
	if err != nil {
		return err
	}
	if code < 200 || code >= 300 {
		return fmt.Errorf("%s %s: status %d: %s", method, path, code, string(respBody))
	}
	if out != nil && len(respBody) > 0 {
		if e := json.Unmarshal(respBody, out); e != nil {
			return fmt.Errorf("%s %s: decode: %w", method, path, e)
		}
	}
	return nil
}

// expectCode asserts the HTTP status without decoding the body.
func expectCode(srv *httptest.Server, method, path string, body any, wantCode int) error {
	code, respBody, err := doJSON(srv, method, path, body)
	if err != nil {
		return err
	}
	if code != wantCode {
		return fmt.Errorf("%s %s: want status %d, got %d: %s", method, path, wantCode, code, string(respBody))
	}
	return nil
}

// parseTime parses an RFC3339 timestamp; it panics on error because a malformed
// fixed base time is a programmer mistake, not a runtime condition.
func parseTime(s string) (t timeT) {
	return timeParse(s)
}

// ctx returns a background context.
func ctx() context.Context { return context.Background() }

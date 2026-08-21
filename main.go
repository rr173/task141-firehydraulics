// Command task141-firehydraulics serves the fire-sprinkler hydraulic HTTP API
// backed by SQLite, and provides a --smoke-test that exercises the full
// contract (project + system + network + water supply + pump, tree hydraulic
// calculation, supply-vs-demand comparison, NFPA 13 compliance, lifecycle,
// hydrostatic test, acceptance, impairment with compensating measures,
// restart recovery, frontend page) without real-time sleeps.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"log"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"task141-firehydraulics/internal/clock"
	"task141-firehydraulics/internal/httpapi"
	"task141-firehydraulics/internal/selfcheck"
	"task141-firehydraulics/internal/service"
	"task141-firehydraulics/internal/store"
	"task141-firehydraulics/internal/webfs"
)

// webFS is the embedded static frontend (native HTML/CSS/JS, no build step).
// The embed lives in package webfs so the selfcheck can serve the same page.
var webFS = webfs.FS()

func main() {
	smoke := flag.Bool("smoke-test", false, "run self-check and exit")
	migrateOnly := flag.Bool("migrate-only", false, "apply schema and exit")
	dbPath := flag.String("db", "firehydraulics.db", "SQLite database file path")
	addr := flag.String("addr", ":8080", "listen address")
	flag.Parse()

	if *smoke {
		if err := selfcheck.Run(); err != nil {
			fmt.Println("smoke-test: FAIL:", err)
			osExit(1)
		}
		fmt.Println("smoke-test: ok")
		osExit(0)
	}

	st, err := store.Open(*dbPath)
	if err != nil {
		log.Fatalf("open store: %v", err)
	}
	defer st.Close()

	svc := service.NewWithClock(st, clock.Real{})
	// Restart-recovery: recompute every derived figure from the persisted
	// authoritative inputs so a process that died between writes converges to
	// the same state.
	if n, err := svc.Reconcile().ReconcileAll(context.Background()); err != nil {
		log.Printf("reconcile on startup: %v", err)
	} else if n > 0 {
		log.Printf("reconciled %d systems on startup", n)
	}

	if *migrateOnly {
		fmt.Println("migrate-only: schema applied")
		return
	}

	srv := &http.Server{
		Addr:              *addr,
		Handler:           httpapi.NewMux(httpapi.Services{Svc: svc}, webFS),
		ReadHeaderTimeout: 5 * time.Second,
	}
	go func() {
		log.Printf("firehydraulics %s listening on %s (db=%s)", httpapi.Version, *addr, *dbPath)
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			log.Fatalf("listen: %v", err)
		}
	}()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt, syscall.SIGTERM)
	<-stop
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := srv.Shutdown(ctx); err != nil {
		log.Printf("shutdown: %v", err)
	}
}

// osExit is indirected so tests can substitute it; in production it is os.Exit.
var osExit = os.Exit

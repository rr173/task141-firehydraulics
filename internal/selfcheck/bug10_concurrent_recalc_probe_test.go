package selfcheck

import (
	"sync"
	"testing"

	"task141-firehydraulics/internal/clock"
	"task141-firehydraulics/internal/model"
)

func TestBug10_ConcurrentCalculationAndComplianceAreSerialized(t *testing.T) {
	clk := clock.NewFake(parseTime("2026-02-08T08:00:00Z"))
	srv, err := newServer(t.TempDir()+"/concurrent.db", clk)
	if err != nil {
		t.Fatal(err)
	}
	defer srv.Close()
	_, sid, _, _, err := buildSimpleTree(srv, model.HazardExtra2, 95, 23200, 50)
	if err != nil {
		t.Fatal(err)
	}
	if err := addAdequateSupply(srv, sid); err != nil {
		t.Fatal(err)
	}
	start := make(chan struct{})
	var wg sync.WaitGroup
	for i := 0; i < 24; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			<-start
			_, _, _ = doJSON(srv, "POST", "/api/systems/"+sid+"/calculate", nil)
		}()
	}
	close(start)
	wg.Wait()
}

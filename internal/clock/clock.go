// Package clock abstracts wall-clock time so the hydraulic engine can be driven
// deterministically in tests and in the smoke-test. Impairment auto-restore and
// inspection due-date computations are advanced by an injected clock instead
// of real time, so a self-check can complete a restore-without-sleep.
package clock

import (
	"context"
	"sync"
	"time"
)

// Clock is the minimal interface the services depend on. Production uses Real;
// tests/self-check use Fake and advance it explicitly.
type Clock interface {
	// Now returns the current instant.
	Now() time.Time
	// Epoch returns the current unix seconds.
	Epoch() int64
}

// Real is the production clock backed by the wall clock.
type Real struct{}

// Now returns the wall-clock time.
func (Real) Now() time.Time { return time.Now() }

// Epoch returns the wall-clock unix seconds.
func (Real) Epoch() int64 { return time.Now().Unix() }

// Fake is a controllable clock. It is safe for concurrent use.
type Fake struct {
	mu  sync.Mutex
	now time.Time
}

// NewFake creates a Fake clock anchored at t. Callers should pass a fixed base
// time (the self-check passes a constant) so timestamps are deterministic.
func NewFake(t time.Time) *Fake {
	return &Fake{now: t}
}

// Now returns the fake current time.
func (f *Fake) Now() time.Time {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.now
}

// Epoch returns the fake current time's unix seconds.
func (f *Fake) Epoch() int64 {
	f.mu.Lock()
	defer f.mu.Unlock()
	return f.now.Unix()
}

// Advance moves the fake clock forward by d.
func (f *Fake) Advance(d time.Duration) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.now = f.now.Add(d)
}

// SetEpoch sets the fake clock to the given unix seconds.
func (f *Fake) SetEpoch(sec int64) {
	f.mu.Lock()
	defer f.mu.Unlock()
	f.now = time.Unix(sec, 0).UTC()
}

// ctxKey is an unexported type for context-stored clocks so callers cannot
// collide on the key.
type ctxKey struct{}

// WithClock returns ctx with c attached. Service methods retrieve it via
// FromContext, falling back to Real when absent.
func WithClock(ctx context.Context, c Clock) context.Context {
	if c == nil {
		c = Real{}
	}
	return context.WithValue(ctx, ctxKey{}, c)
}

// FromContext returns the clock stored in ctx, or Real{} if none is present.
func FromContext(ctx context.Context) Clock {
	if c, ok := ctx.Value(ctxKey{}).(Clock); ok {
		return c
	}
	return Real{}
}

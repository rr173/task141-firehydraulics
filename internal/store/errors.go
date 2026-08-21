package store

import "errors"

// Domain errors mapped to HTTP statuses by the httpapi layer. Each is a
// sentinel so the service can wrap it with %w and the handler can errors.Is
// it.
var (
	ErrNotFound                   = errors.New("store: not found")
	ErrConflict                   = errors.New("store: conflict")
	ErrStateConflict              = errors.New("store: illegal state transition")
	ErrImpairmentMissingCompensation = errors.New("store: impairment requires compensating measure")
	ErrHydrostaticFailed          = errors.New("store: hydrostatic test failed")
	ErrNetworkCycle               = errors.New("store: network cycle or disconnect")
	ErrInvariant                  = errors.New("store: invariant violation")
	ErrSupplyInsufficient         = errors.New("store: supply insufficient")
)

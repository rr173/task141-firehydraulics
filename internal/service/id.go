package service

import "task141-firehydraulics/internal/idlib"

// newID generates a new opaque id (shim over idlib.New so the reconcile file
// keeps the recovery path self-contained).
func newID() string { return idlib.New("r") }

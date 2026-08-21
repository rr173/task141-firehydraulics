// Package lifecycle encodes the fire-protection system state machine and the
// legal transitions between states. It is a pure function so the service layer
// can validate a transition before persisting it, and the restart replay can
// re-derive the current state from the event stream.
package lifecycle

import (
	"errors"

	"task141-firehydraulics/internal/model"
)

// ErrIllegalTransition is returned when a requested transition is not legal.
var ErrIllegalTransition = errors.New("lifecycle: illegal state transition")

// legalTransitions is the adjacency matrix of the state machine.
var legalTransitions = map[model.SystemState][]model.SystemState{
	model.StateDraft:       {model.StateDesigned},
	model.StateDesigned:    {model.StateSubmitted, model.StateDraft},
	model.StateSubmitted:   {model.StateApproved, model.StateDesigned},
	model.StateApproved:    {model.StateInstalled, model.StateSubmitted},
	model.StateInstalled:   {model.StateHydrostatic, model.StateApproved},
	model.StateHydrostatic: {model.StateAccepted, model.StateInstalled},
	model.StateAccepted:   {model.StateInService, model.StateHydrostatic},
	model.StateInService:   {model.StateImpaired},
	model.StateImpaired:    {model.StateRestored, model.StateInService},
	model.StateRestored:    {model.StateInService},
}

// CanTransition reports whether from→to is a legal transition.
func CanTransition(from, to model.SystemState) bool {
	for _, t := range legalTransitions[from] {
		if t == to {
			return true
		}
	}
	return false
}

// Reason validates that a transition is legal given a reason code (the reason
// is informational; it does not change legality but is recorded on the event).
type Transition struct {
	From   model.SystemState
	To     model.SystemState
	Reason string
}

// Validate returns nil if t is legal, ErrIllegalTransition otherwise.
func (tr Transition) Validate() error {
	if CanTransition(tr.From, tr.To) {
		return nil
	}
	return ErrIllegalTransition
}

// Precondition checks: some transitions require a precondition to be met
// (e.g. must have a hydrostatic test before accepted→in_service). These are
// enforced by the service layer; this package only owns the graph.

// IsTerminal reports whether a state cannot leave the main flow (none here —
// all states can flow onward). Provided for the selfcheck.
func IsTerminal(s model.SystemState) bool { return false }

// AllStates returns the states in lifecycle order.
func AllStates() []model.SystemState {
	return []model.SystemState{
		model.StateDraft, model.StateDesigned, model.StateSubmitted,
		model.StateApproved, model.StateInstalled, model.StateHydrostatic,
		model.StateAccepted, model.StateInService, model.StateImpaired,
		model.StateRestored,
	}
}

package lifecycle

import (
	"testing"

	"task141-firehydraulics/internal/model"
)

func TestCanTransition(t *testing.T) {
	cases := []struct {
		from, to model.SystemState
		want      bool
	}{
		{model.StateDraft, model.StateDesigned, true},
		{model.StateDesigned, model.StateSubmitted, true},
		{model.StateSubmitted, model.StateApproved, true},
		{model.StateApproved, model.StateInstalled, true},
		{model.StateInstalled, model.StateHydrostatic, true},
		{model.StateHydrostatic, model.StateAccepted, true},
		{model.StateAccepted, model.StateInService, true},
		{model.StateInService, model.StateImpaired, true},
		{model.StateImpaired, model.StateRestored, true},
		{model.StateRestored, model.StateInService, true},
		// illegal jumps
		{model.StateDraft, model.StateApproved, false},
		{model.StateApproved, model.StateInService, false},
		{model.StateInService, model.StateApproved, false},
		{model.StateRestored, model.StateImpaired, false},
	}
	for _, c := range cases {
		got := CanTransition(c.from, c.to)
		if got != c.want {
			t.Errorf("CanTransition(%s→%s)=%v want %v", c.from, c.to, got, c.want)
		}
	}
}

func TestTransitionValidate(t *testing.T) {
	tr := Transition{From: model.StateDraft, To: model.StateDesigned, Reason: "ok"}
	if err := tr.Validate(); err != nil {
		t.Errorf("valid transition errored: %v", err)
	}
	tr2 := Transition{From: model.StateDraft, To: model.StateApproved, Reason: "skip"}
	if err := tr2.Validate(); err == nil {
		t.Errorf("illegal transition should error")
	}
}

package domain

import (
	"errors"
	"testing"
)

func TestTransition_Invalid(t *testing.T) {
	err := Transition(StateRequested, StateCompleted)
	if err == nil {
		t.Fatal("Transition(Requested, Completed) expected error")
	}
	var domainErr *DomainError
	if !errors.As(err, &domainErr) || domainErr.ErrCode != CodeInvalidTransition {
		t.Errorf("expected STATE.INVALID_TRANSITION, got %v", err)
	}
}

func TestTransition_Valid(t *testing.T) {
	valid := []struct{ from, to State }{
		{StateRequested, StateValidating},
		{StateValidating, StateAnalyzing},
		{StateValidating, StateRejected},
		{StateAnalyzing, StateApproved},
		{StateAnalyzing, StateRejected},
		{StateApproved, StateFundsPosted},
		{StateFundsPosted, StateCompleted},
		{StateFundsPosted, StateReturned},
		{StateCompleted, StateReturned},
	}
	for _, tt := range valid {
		if err := Transition(tt.from, tt.to); err != nil {
			t.Errorf("Transition(%s, %s): %v", tt.from, tt.to, err)
		}
	}
}

func TestValidTransitions_Copy(t *testing.T) {
	a := ValidTransitions(StateValidating)
	b := ValidTransitions(StateValidating)
	if len(a) != len(b) || len(a) < 1 {
		t.Fatalf("unexpected ValidTransitions length")
	}
	a[0] = StateCompleted
	if b[0] == StateCompleted {
		t.Error("ValidTransitions should return a copy (mutation affected other caller)")
	}
}

func TestTransition_InvalidPairs(t *testing.T) {
	invalid := []struct{ from, to State }{
		{StateRequested, StateCompleted},
		{StateRequested, StateRejected},
		{StateApproved, StateRequested},
		{StateRejected, StateApproved},
		{StateReturned, StateFundsPosted},
	}
	for _, tt := range invalid {
		if err := Transition(tt.from, tt.to); err == nil {
			t.Errorf("Transition(%s, %s) expected error", tt.from, tt.to)
		}
	}
}

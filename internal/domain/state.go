package domain

// EventTypeStateTransition is the deposit_events event_type when recording a state change (PRD §7.2).
// Persistence is the caller's responsibility: Transition() stays pure and returns only validation errors.
const EventTypeStateTransition = "STATE_TRANSITION"

// State is the transfer lifecycle state (PRD §7.8).
type State string

const (
	StateRequested   State = "Requested"
	StateValidating  State = "Validating"
	StateAnalyzing   State = "Analyzing"
	StateApproved    State = "Approved"
	StateFundsPosted State = "FundsPosted"
	StateCompleted   State = "Completed"
	StateRejected    State = "Rejected"
	StateReturned    State = "Returned"
)

// validTransitions defines allowed next states per PRD §7.8. Terminal states have no entries.
var validTransitions = map[State][]State{
	StateRequested:   {StateValidating},
	StateValidating:  {StateAnalyzing, StateRejected},
	StateAnalyzing:   {StateApproved, StateRejected},
	StateApproved:    {StateFundsPosted},
	StateFundsPosted: {StateCompleted, StateReturned},
	StateCompleted:   {StateReturned},
	StateRejected:    nil, // terminal
	StateReturned:    nil, // terminal
}

// Transition validates that moving from `from` to `to` is allowed. Returns ErrInvalidTransition if not.
// It does not write to deposit_events; the pipeline/handler must insert a STATE_TRANSITION event after a valid transition (TICKET-007).
func Transition(from, to State) error {
	allowed := validTransitions[from]
	for _, s := range allowed {
		if s == to {
			return nil
		}
	}
	return ErrInvalidTransition
}

// ValidTransitions returns a copy of the list of states allowed from the given state (caller cannot mutate internal map slices).
func ValidTransitions(from State) []State {
	src := validTransitions[from]
	if len(src) == 0 {
		return nil
	}
	out := make([]State, len(src))
	copy(out, src)
	return out
}

// StateTransitionPayload holds the payload JSON fields for deposit_events when event_type is STATE_TRANSITION.
type StateTransitionPayload struct {
	FromState string `json:"from_state"`
	ToState   string `json:"to_state"`
	Actor     string `json:"actor"`
	Reason    string `json:"reason,omitempty"`
}

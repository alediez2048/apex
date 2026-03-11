package pipeline

// Action is what a pipeline step tells the runner to do next.
type Action int

const (
	Continue     Action = iota // run next step
	HaltRejected              // terminal failure — deposit rejected
	HaltFlagged               // needs operator review (stays in Analyzing)
	HaltApproved              // reached FundsPosted (final good state for pipeline)
)

func (a Action) String() string {
	switch a {
	case Continue:
		return "Continue"
	case HaltRejected:
		return "HaltRejected"
	case HaltFlagged:
		return "HaltFlagged"
	case HaltApproved:
		return "HaltApproved"
	default:
		return "Unknown"
	}
}

// Result is what a pipeline step returns to the runner.
type Result struct {
	Action Action
	Status string // final transfer status after this step
	Reason string // human-readable reason (for events)
}

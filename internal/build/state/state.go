package state

type State string

const (
	Scheduled State = "scheduled"
	Running   State = "running"
	Blocked   State = "blocked"
	Canceling State = "canceling"
	Failing   State = "failing"
	Passed    State = "passed"
	Failed    State = "failed"
	Canceled  State = "canceled"
	Skipped   State = "skipped"
	NotRun    State = "not_run"
)

func IsTerminal(state State) bool {
	switch state {
	case Passed, Failed, Canceled, Skipped, NotRun:
		return true
	default:
		return false
	}
}

func IsIncomplete(state State) bool {
	switch state {
	case Scheduled, Running, Blocked, Canceling:
		return true
	default:
		return false
	}
}

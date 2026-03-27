package state

type State string

const (
	Scheduled State = "scheduled"
	Running   State = "running"
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

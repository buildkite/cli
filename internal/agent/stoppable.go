package agent

import (
	"fmt"

	tea "github.com/charmbracelet/bubbletea"
)

type Status int

const (
	Waiting Status = iota
	Stopping
	Succeeded
	Failed
)

// StatusUpdate is used to update the internal state of a StoppableAgent
type StatusUpdate struct {
	Cmd    tea.Cmd
	Err    error
	ID     string
	Status Status
}

// StopFn represents a function that returns a StatusUpdate
// Use a function of this type to update the state of a StoppableAgent
type StopFn func() StatusUpdate

type StoppableAgent struct {
	err    error
	id     string
	status Status
	stopFn StopFn
}

func NewStoppableAgent(id string, stopFn StopFn) StoppableAgent {
	return StoppableAgent{
		id:     id,
		status: Waiting,
		stopFn: stopFn,
	}
}

// Init implements tea.Model
func (agent StoppableAgent) Init() tea.Cmd {
	return func() tea.Msg {
		if agent.stopFn != nil {
			return agent.stopFn()
		}
		return nil
	}
}

// Update implements tea.Model.
func (agent StoppableAgent) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case StatusUpdate:
		// if the msg ID doesn't match this agent, do nothing as it doesnt apply to this instance
		if msg.ID != agent.id {
			return agent, nil
		}
		// if the status update contains an error, deal with that first
		if msg.Err != nil {
			agent.err = msg.Err
			agent.status = Failed
			return agent, msg.Cmd
		}
		agent.status = msg.Status
		return agent, msg.Cmd
	default:
		return agent, nil
	}
}

// View implements tea.Model.
func (agent StoppableAgent) View() string {
	var out string

	switch agent.status {
	case Waiting:
		out = fmt.Sprintf("... Waiting to stop agent %s\n", agent.id)
	case Stopping:
		out = fmt.Sprintf("... Stopping agent %s\n", agent.id)
	case Succeeded:
		out = fmt.Sprintf("✓   Stopped agent %s\n", agent.id)
	case Failed:
		out = fmt.Sprintf("✗   Failed to stop agent %s (error: %s)\n", agent.id, agent.err.Error())
	}

	return out
}

func (agent StoppableAgent) Errored() bool {
	return agent.err != nil
}

package agent

import (
	"fmt"

	"github.com/charmbracelet/bubbles/spinner"
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
	err     error
	id      string
	spinner spinner.Model
	status  Status
	stopFn  StopFn
}

func NewStoppableAgent(id string, stopFn StopFn) StoppableAgent {
	return StoppableAgent{
		id:      id,
		spinner: spinner.New(spinner.WithSpinner(spinner.Points)),
		status:  Waiting,
		stopFn:  stopFn,
	}
}

// Init implements tea.Model
func (agent StoppableAgent) Init() tea.Cmd {
	return tea.Batch(
		agent.spinner.Tick,
		func() tea.Msg {
			if agent.stopFn != nil {
				return agent.stopFn()
			}
			return nil
		},
	)
}

// Update implements tea.Model.
func (agent StoppableAgent) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	var cmd tea.Cmd
	switch msg := msg.(type) {
	case spinner.TickMsg:
		agent.spinner, cmd = agent.spinner.Update(msg)
		return agent, cmd
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
		// only update the spinner if status is Stopping
		var spinCmd tea.Cmd
		if msg.Status == Stopping {
			agent.spinner, spinCmd = agent.spinner.Update(msg)
		}
		agent.status = msg.Status
		return agent, tea.Batch(msg.Cmd, spinCmd)
	default:
		return agent, nil
	}
}

// View implements tea.Model.
func (agent StoppableAgent) View() string {
	var out string

	switch agent.status {
	case Waiting:
		out = fmt.Sprintf("%s Waiting to stop agent %s\n", agent.spinner.View(), agent.id)
	case Stopping:
		out = fmt.Sprintf("%s Stopping agent %s\n", agent.spinner.View(), agent.id)
	case Succeeded:
		out = fmt.Sprintf("✓   Stopped agent %s\n", agent.id)
	case Failed:
		out = fmt.Sprintf("✗   Failed to stop agent %s (error: %s)\n", agent.id, agent.err.Error())
	}

	return out
}

package io

import (
	"fmt"

	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
)

type PendingOutput string

// Pending is used to show a loading spinner while a long running function runs to perform some action and output
// information
type Pending struct {
	Err      error
	fn       tea.Cmd
	output   string
	quitting bool
	spinner  spinner.Model
}

// Init implements tea.Model.
func (p Pending) Init() tea.Cmd {
	return tea.Batch(
		p.spinner.Tick,
		p.fn,
	)
}

// Update implements tea.Model.
func (p Pending) Update(msg tea.Msg) (tea.Model, tea.Cmd) {
	switch msg := msg.(type) {
	case tea.KeyMsg:
		// if a key is pressed, ignore everything except for common quitting
		switch msg.String() {
		case "q", "esc", "ctrl+c":
			p.quitting = true
			p.output = "Cancelled"
			return p, tea.Quit
		default:
			return p, nil
		}
	case PendingOutput: // this signals that p.fn has finished executing and ready to send final output
		p.quitting = true
		p.output = string(msg)
		return p, tea.Quit
	case error: // an error occurred somewhere. show the error and exit
		p.quitting = true
		p.output = "Error: " + msg.Error()
		p.Err = msg
		return p, tea.Quit
	default: // start/update the spinner
		var cmd tea.Cmd
		p.spinner, cmd = p.spinner.Update(msg)
		return p, cmd
	}
}

// View implements tea.Model.
func (p Pending) View() string {
	if !p.quitting {
		return fmt.Sprintf("%s %s", p.spinner.View(), p.output)
	}

	// add a newline to the output if not present, otherwise the last output gets swallowed
	if last := p.output[len(p.output)-1]; last != '\n' {
		p.output += "\n"
	}
	return p.output
}

// NewPendingCommand is used to show a loading spinner while a long running function runs to perform some action and
// output information.
// fn is a function run to perform the action. It should return a PendingOutput if to update the output after the action
// is complete. It can also return an error instance.
// loadingText is the text shown while the function is running
func NewPendingCommand(fn tea.Cmd, loadingText string) Pending {
	return Pending{
		spinner: spinner.New(spinner.WithSpinner(spinner.Points)),
		fn:      fn,
		output:  loadingText,
	}
}

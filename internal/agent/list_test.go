package agent

import (
	"io"
	"sync"
	"testing"
	"time"

	"github.com/buildkite/go-buildkite/v3/buildkite"
	"github.com/charmbracelet/bubbles/spinner"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/exp/teatest"
	"github.com/muesli/termenv"
)

func init() {
	lipgloss.SetColorProfile(termenv.Ascii)
}

func simpleAgentLoader(wg *sync.WaitGroup) func(int) tea.Cmd {
	return func(int) tea.Cmd {
		return func() tea.Msg {
			wg.Done()
			return NewAgentItemsMsg(
				[]AgentListItem{
					{Agent: &buildkite.Agent{Name: buildkite.String("test agent"), ConnectedState: buildkite.String("connected"), Version: buildkite.String("0.0.0")}},
					{Agent: &buildkite.Agent{Name: buildkite.String("test agent 2"), ConnectedState: buildkite.String("connected"), Version: buildkite.String("0.0.0")}},
				},
				1,
			)
		}
	}
}

func TestAgentListModel(t *testing.T) {
	t.Parallel()

	t.Run("agents are added and rendered", func(t *testing.T) {
		var wg sync.WaitGroup
		wg.Add(1)
		model := NewAgentList(simpleAgentLoader(&wg), 1, 1, func(s string, b bool) any { return nil })

		// disable spinner for testing (it causes flaky time-sensitive results)
		model.agentList.SetSpinner(spinner.Spinner{Frames: []string{}})
		testModel := teatest.NewTestModel(t, model, teatest.WithInitialTermSize(100, 100))

		// wait on the wait group to add an agent and then quit
		wg.Wait()
		testModel.Type("q")
		finalModel := testModel.FinalModel(t, teatest.WithFinalTimeout(time.Second))
		finalOutput, err := io.ReadAll(testModel.FinalOutput(t))
		if err != nil {
			t.Error(err)
		}

		if len(finalModel.(AgentListModel).agentList.Items()) != 2 {
			t.Error("model does not have an agent")
		}
		teatest.RequireEqualOutput(t, finalOutput)
	})
}

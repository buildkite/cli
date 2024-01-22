package agent

import (
	"io"
	"testing"
	"time"

	"github.com/buildkite/go-buildkite/v3/buildkite"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/lipgloss"
	"github.com/charmbracelet/x/exp/teatest"
	"github.com/muesli/termenv"
)

func init() {
	lipgloss.SetColorProfile(termenv.Ascii)
}

func TestAgentListModel(t *testing.T) {
	t.Parallel()

	t.Run("agents are added and rendered", func(t *testing.T) {
		model := NewAgentList(func() tea.Msg {
			return NewAgentItemsMsg{
				AgentListItem{Agent: &buildkite.Agent{Name: buildkite.String("test agent"), ConnectedState: buildkite.String("connected"), Version: buildkite.String("0.0.0")}},
			}
		}, func(s string, b bool) error { return nil })

		testModel := teatest.NewTestModel(t, model)
		timer := time.NewTimer(time.Millisecond * 100)
		go (func() {
			<-timer.C
			testModel.Send(tea.Quit())
		})()

		finalModel := testModel.FinalModel(t, teatest.WithFinalTimeout(time.Second))
		finalOutput, err := io.ReadAll(testModel.FinalOutput(t))
		if err != nil {
			t.Error(err)
		}

		if len(finalModel.(AgentListModel).agentList.Items()) != 1 {
			t.Error("model does not have an agent")
		}
		teatest.RequireEqualOutput(t, finalOutput)
	})
}

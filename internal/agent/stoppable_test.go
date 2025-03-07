package agent

import (
	"errors"
	"testing"
	"time"

	"github.com/buildkite/cli/v3/internal/testutil"
	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/exp/teatest"
)

func TestStoppableAgentOutput(t *testing.T) {
	t.Parallel()

	t.Run("starts in waiting state", func(t *testing.T) {
		t.Parallel()

		// use a StopFn that quits straight away
		model := NewStoppableAgent("123", func() StatusUpdate { return StatusUpdate{tea.Quit, nil, "123", Waiting} })
		testutil.AssertTeaOutput(t, model, "Waiting to stop agent 123")
	})

	t.Run("stopping state", func(t *testing.T) {
		t.Parallel()

		// use a StopFn that quits straight away
		model := NewStoppableAgent("123", func() StatusUpdate { return StatusUpdate{tea.Quit, nil, "123", Stopping} })
		testutil.AssertTeaOutput(t, model, "Stopping agent 123")
	})

	t.Run("success state", func(t *testing.T) {
		t.Parallel()

		// use a StopFn that quits straight away
		model := NewStoppableAgent("123", func() StatusUpdate { return StatusUpdate{tea.Quit, nil, "123", Succeeded} })
		testutil.AssertTeaOutput(t, model, "Stopped agent 123")
	})

	t.Run("failed state", func(t *testing.T) {
		t.Parallel()

		// use a StopFn that quits straight away
		model := NewStoppableAgent("123", func() StatusUpdate { return StatusUpdate{tea.Quit, errors.New("error"), "123", Failed} })
		testutil.AssertTeaOutput(t, model, "Failed to stop agent 123 (error: error)")
	})

	t.Run("transitions through waiting-stopping-succeeded", func(t *testing.T) {
		t.Parallel()

		// use a StopFn that quits straight away
		model := NewStoppableAgent("123", func() StatusUpdate { return StatusUpdate{nil, nil, "123", Waiting} })
		testModel := teatest.NewTestModel(t, model)

		teatest.WaitFor(t, testModel.Output(), func(bts []byte) bool {
			return testutil.Contains(bts, "Waiting to stop agent 123")
		})

		testModel.Send(StatusUpdate{
			ID:     "123",
			Status: Stopping,
		})

		teatest.WaitFor(t, testModel.Output(), func(bts []byte) bool {
			return testutil.Contains(bts, "Stopping agent 123")
		})

		testModel.Send(StatusUpdate{
			ID:     "123",
			Status: Succeeded,
			Cmd:    tea.Quit,
		})

		teatest.WaitFor(t, testModel.Output(), func(bts []byte) bool {
			return testutil.Contains(bts, "Stopped agent 123")
		})

		testModel.WaitFinished(t, teatest.WithFinalTimeout(time.Second))
	})

	t.Run("shows error state", func(t *testing.T) {
		t.Parallel()

		// use a StopFn that quits straight away
		model := NewStoppableAgent("123", func() StatusUpdate { return StatusUpdate{nil, nil, "123", Waiting} })
		testModel := teatest.NewTestModel(t, model)

		teatest.WaitFor(t, testModel.Output(), func(bts []byte) bool {
			return testutil.Contains(bts, "Waiting to stop agent 123")
		})

		testModel.Send(StatusUpdate{
			ID:  "123",
			Err: errors.New("Could not stop"),
		})

		teatest.WaitFor(t, testModel.Output(), func(bts []byte) bool {
			return testutil.Contains(bts, "Failed to stop agent 123 (error: Could not stop)")
		})

		testModel.Send(tea.Quit())
		testModel.WaitFinished(t, teatest.WithFinalTimeout(time.Second))
	})
}

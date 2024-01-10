package agent

import (
	"bytes"
	"errors"
	"io"
	"testing"
	"time"

	tea "github.com/charmbracelet/bubbletea"
	"github.com/charmbracelet/x/exp/teatest"
)

func TestStoppableAgentOutput(t *testing.T) {
	t.Parallel()

	t.Run("starts in waiting state", func(t *testing.T) {
		// use a StopFn that quits straight away
		model := NewStoppableAgent("123", func() StatusUpdate { return StatusUpdate{tea.Quit, nil, Waiting} })
		testModel := teatest.NewTestModel(t, model)
		out, err := io.ReadAll(testModel.FinalOutput(t))
		if err != nil {
			t.Error(err)
		}
		if !bytes.Contains(out, []byte("Waiting to stop agent 123")) {
			t.Error("Output did not match")
		}
	})

	t.Run("stopping state", func(t *testing.T) {
		// use a StopFn that quits straight away
		model := NewStoppableAgent("123", func() StatusUpdate { return StatusUpdate{tea.Quit, nil, Stopping} })
		testModel := teatest.NewTestModel(t, model)
		out, err := io.ReadAll(testModel.FinalOutput(t))
		if err != nil {
			t.Error(err)
		}
		if !bytes.Contains(out, []byte("Stopping agent 123")) {
			t.Error("Output did not match")
		}
	})

	t.Run("success state", func(t *testing.T) {
		// use a StopFn that quits straight away
		model := NewStoppableAgent("123", func() StatusUpdate { return StatusUpdate{tea.Quit, nil, Succeeded} })
		testModel := teatest.NewTestModel(t, model)
		out, err := io.ReadAll(testModel.FinalOutput(t))
		if err != nil {
			t.Error(err)
		}
		if !bytes.Contains(out, []byte("Stopped agent 123")) {
			t.Error("Output did not match")
		}
	})

	t.Run("failed state", func(t *testing.T) {
		// use a StopFn that quits straight away
		model := NewStoppableAgent("123", func() StatusUpdate { return StatusUpdate{tea.Quit, errors.New("error"), Failed} })
		testModel := teatest.NewTestModel(t, model)
		out, err := io.ReadAll(testModel.FinalOutput(t))
		if err != nil {
			t.Error(err)
		}
		if !bytes.Contains(out, []byte("Failed to stop agent 123 (error: error)")) {
			t.Error("Output did not match")
		}
	})

	t.Run("transitions through waiting-stopping-succeeded", func(t *testing.T) {
		// use a StopFn that quits straight away
		model := NewStoppableAgent("123", func() StatusUpdate { return StatusUpdate{nil, nil, Waiting} })
		testModel := teatest.NewTestModel(t, model)

		teatest.WaitFor(t, testModel.Output(), func(bts []byte) bool {
			return bytes.Contains(bts, []byte("Waiting to stop agent 123"))
		})
		testModel.Send(StatusUpdate{
			status: Stopping,
		})
		teatest.WaitFor(t, testModel.Output(), func(bts []byte) bool {
			return bytes.Contains(bts, []byte("Stopping agent 123"))
		})
		testModel.Send(StatusUpdate{
			status: Succeeded,
			cmd:    tea.Quit,
		})
		teatest.WaitFor(t, testModel.Output(), func(bts []byte) bool {
			return bytes.Contains(bts, []byte("Stopped agent 123"))
		}, teatest.WithCheckInterval(time.Millisecond*100))

		testModel.WaitFinished(t, teatest.WithFinalTimeout(time.Second*5))
	})

	t.Run("shows error state", func(t *testing.T) {
		// use a StopFn that quits straight away
		model := NewStoppableAgent("123", func() StatusUpdate { return StatusUpdate{nil, nil, Waiting} })
		testModel := teatest.NewTestModel(t, model)

		teatest.WaitFor(t, testModel.Output(), func(bts []byte) bool {
			return bytes.Contains(bts, []byte("Waiting to stop agent 123"))
		})
		testModel.Send(StatusUpdate{
			err: errors.New("Could not stop"),
		})
		teatest.WaitFor(t, testModel.Output(), func(bts []byte) bool {
			return bytes.Contains(bts, []byte("Failed to stop agent 123 (error: Could not stop)"))
		})
		testModel.Send(tea.Quit)

		testModel.WaitFinished(t, teatest.WithFinalTimeout(time.Second))
	})
}

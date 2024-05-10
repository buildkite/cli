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
		t.Parallel()

		// use a StopFn that quits straight away
		model := NewStoppableAgent("123", func() StatusUpdate { return StatusUpdate{tea.Quit, nil, "123", Waiting} })
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
		t.Parallel()

		// use a StopFn that quits straight away
		model := NewStoppableAgent("123", func() StatusUpdate { return StatusUpdate{tea.Quit, nil, "123", Stopping} })
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
		t.Parallel()

		// use a StopFn that quits straight away
		model := NewStoppableAgent("123", func() StatusUpdate { return StatusUpdate{tea.Quit, nil, "123", Succeeded} })
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
		t.Parallel()

		// use a StopFn that quits straight away
		model := NewStoppableAgent("123", func() StatusUpdate { return StatusUpdate{tea.Quit, errors.New("error"), "123", Failed} })
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
		t.Parallel()

		// use a StopFn that quits straight away
		model := NewStoppableAgent("123", func() StatusUpdate { return StatusUpdate{nil, nil, "123", Waiting} })
		testModel := teatest.NewTestModel(t, model)

		teatest.WaitFor(t, testModel.Output(), func(bts []byte) bool {
			return bytes.Contains(bts, []byte("Waiting to stop agent 123"))
		})
		testModel.Send(StatusUpdate{
			ID:     "123",
			Status: Stopping,
		})
		teatest.WaitFor(t, testModel.Output(), func(bts []byte) bool {
			return bytes.Contains(bts, []byte("Stopping agent 123"))
		})
		testModel.Send(StatusUpdate{
			ID:     "123",
			Status: Succeeded,
			Cmd:    tea.Quit,
		})
		teatest.WaitFor(t, testModel.Output(), func(bts []byte) bool {
			return bytes.Contains(bts, []byte("Stopped agent 123"))
		})

		testModel.WaitFinished(t, teatest.WithFinalTimeout(time.Second))
	})

	t.Run("shows error state", func(t *testing.T) {
		t.Parallel()

		// use a StopFn that quits straight away
		model := NewStoppableAgent("123", func() StatusUpdate { return StatusUpdate{nil, nil, "123", Waiting} })
		testModel := teatest.NewTestModel(t, model)

		teatest.WaitFor(t, testModel.Output(), func(bts []byte) bool {
			return bytes.Contains(bts, []byte("Waiting to stop agent 123"))
		})
		testModel.Send(StatusUpdate{
			ID:  "123",
			Err: errors.New("Could not stop"),
		})
		teatest.WaitFor(t, testModel.Output(), func(bts []byte) bool {
			return bytes.Contains(bts, []byte("Failed to stop agent 123 (error: Could not stop)"))
		})
		testModel.Send(tea.Quit())

		testModel.WaitFinished(t, teatest.WithFinalTimeout(time.Second))
	})
}

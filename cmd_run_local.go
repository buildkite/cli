package cli

import (
	"context"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"strings"

	"github.com/buildkite/cli/local"
)

type RunLocalCommandContext struct {
	TerminalContext
	KeyringContext

	Debug     bool
	DebugHTTP bool

	Command         string
	StepFilterRegex string
	Prompt          bool
}

func RunLocalCommand(ctx RunLocalCommandContext) error {
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt)

	if ctx.Debug {
		local.Debug = true
	}

	cancelCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		<-quit
		log.Printf("Shutting down")
		cancel()
	}()

	wd, err := os.Getwd()
	if err != nil {
		return NewExitError(err, 1)
	}

	commit, err := gitCommit()
	if err != nil {
		log.Printf("Error getting git commit: %v", err)
	}

	branch, err := gitBranch()
	if err != nil {
		log.Printf("Error getting git branch: %v", err)
	}

	if err := local.Run(cancelCtx, local.RunParams{
		Command: ctx.Command,
		Dir:     wd,
		Prompt:  ctx.Prompt,
		Filter: func(j local.Job) bool {
			return true
		},
		JobTemplate: local.Job{
			Commit:           commit,
			Branch:           branch,
			Command:          "buildkite-agent pipeline upload",
			Label:            ":pipeline:",
			Repository:       wd,
			OrganizationSlug: "local",
			PipelineSlug:     filepath.Base(wd),
		},
	}); err != nil {
		return NewExitError(err, 1)
	}

	return nil
}

func gitCommit() (string, error) {
	out, err := exec.Command(`git`, `rev-parse`, `--abbrev-ref`, `HEAD`).Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}
func gitBranch() (string, error) {
	out, err := exec.Command(`git`, `rev-parse`, `HEAD`).Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

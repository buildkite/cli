package cli

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/exec"
	"os/signal"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/buildkite/cli/local"
)

type LocalRunCommandContext struct {
	TerminalContext
	KeyringContext

	Debug     bool
	DebugHTTP bool

	File            *os.File
	Env             []string
	Command         string
	StepFilterRegex *regexp.Regexp
	Prompt          bool
	DryRun          bool
}

func LocalRunCommand(ctx LocalRunCommandContext) error {
	quit := make(chan os.Signal, 1)
	signal.Notify(quit, os.Interrupt)

	if ctx.Debug {
		local.Debug = true
	}

	cancelCtx, cancel := context.WithCancel(context.Background())
	defer cancel()

	go func() {
		<-quit
		fmt.Printf("\n>>> Gracefully shutting down...\n")
		cancel()
	}()

	wd, err := os.Getwd()
	if err != nil {
		return NewExitError(err, 1)
	}

	commit, err := gitCommit()
	if err != nil {
		log.Printf("Error getting git commit: %v", err)
		commit = "no_commit_found"
	}

	branch, err := gitBranch()
	if err != nil {
		log.Printf("Error getting git branch: %v", err)
		branch = "master"
	}

	command := ctx.Command
	if ctx.File != nil {
		command = fmt.Sprintf("buildkite-agent pipeline upload %q", ctx.File.Name())
	}

	if err := local.Run(cancelCtx, local.RunParams{
		Env:        ctx.Env,
		DryRun:     ctx.DryRun,
		Command:    command,
		Dir:        wd,
		Prompt:     ctx.Prompt,
		StepFilter: ctx.StepFilterRegex,
		JobTemplate: local.Job{
			Commit:           commit,
			Branch:           branch,
			Repository:       wd,
			OrganizationSlug: "local",
			PipelineSlug:     filepath.Base(wd),
		},
	}); err != nil {
		return NewExitError(err, 1)
	}

	return nil
}

func gitBranch() (string, error) {
	out, err := exec.Command(`git`, `rev-parse`, `--abbrev-ref`, `HEAD`).Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

func gitCommit() (string, error) {
	out, err := exec.Command(`git`, `rev-parse`, `HEAD`).Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

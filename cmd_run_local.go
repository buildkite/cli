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

	File *os.File
	Step string
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

	commit, err := gitCommit()
	if err != nil {
		return err
	}

	branch, err := gitBranch()
	if err != nil {
		return err
	}

	wd, err := os.Getwd()
	if err != nil {
		return err
	}

	if err := local.Run(cancelCtx, local.RunParams{
		Commit:           commit,
		Branch:           branch,
		Command:          "buildkite-agent pipeline upload",
		Label:            ":pipeline:",
		Repository:       wd,
		OrganizationSlug: "local",
		PipelineSlug:     filepath.Base(wd),
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

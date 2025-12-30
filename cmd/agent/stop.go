package agent

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"strings"
	"sync"

	"github.com/alecthomas/kong"
	"github.com/buildkite/cli/v3/internal/cli"
	bkIO "github.com/buildkite/cli/v3/internal/io"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/cli/v3/pkg/cmd/validation"
	"github.com/mattn/go-isatty"
	"golang.org/x/sync/semaphore"
)

type StopCmd struct {
	Agents []string `arg:"" optional:"" help:"Agent IDs to stop"`
	Force  bool     `help:"Force stop the agent. Terminating any jobs in progress"`
	Limit  int64    `help:"Limit parallel API requests" short:"l" default:"5"`
}

func (c *StopCmd) Help() string {
	return `Instruct one or more agents to stop accepting new build jobs and shut itself down.
Agents can be supplied as positional arguments or from STDIN, one per line.

If the "ORGANIZATION_SLUG/" portion of the "ORGANIZATION_SLUG/UUID" agent argument
is omitted, it uses the currently selected organization.

The --force flag applies to all agents that are stopped.

Examples:
  # Stop a single agent
  $ bk agent stop 0198d108-a532-4a62-9bd7-b2e744bf5c45

  # Stop multiple agents
  $ bk agent stop agent-1 agent-2 agent-3

  # Force stop an agent
  $ bk agent stop 0198d108-a532-4a62-9bd7-b2e744bf5c45 --force

  # Stop agents from STDIN
  $ cat agent-ids.txt | bk agent stop`
}

func (c *StopCmd) Run(kongCtx *kong.Context, globals cli.GlobalFlags) error {
	f, err := factory.New()
	if err != nil {
		return err
	}

	f.SkipConfirm = globals.SkipConfirmation()
	f.NoInput = globals.DisableInput()
	f.Quiet = globals.IsQuiet()

	if err := validation.ValidateConfiguration(f.Config, kongCtx.Command()); err != nil {
		return err
	}

	ctx := context.Background()

	// this semaphore is used to limit how many concurrent API requests can be sent
	limit := c.Limit
	if limit < 1 {
		limit = 1
	}
	sem := semaphore.NewWeighted(limit)

	var agentIDs []string
	// this command accepts either input from stdin or positional arguments (not both) in that order
	// so we need to check if stdin has data for us to read and read that, otherwise use positional args and if
	// there are none, then we need to error
	// if stdin has data available, use that
	if bkIO.HasDataAvailable(os.Stdin) {
		scanner := bufio.NewScanner(os.Stdin)
		scanner.Split(bufio.ScanLines)
		for scanner.Scan() {
			id := scanner.Text()
			if strings.TrimSpace(id) != "" {
				agentIDs = append(agentIDs, id)
			}
		}

		if scanner.Err() != nil {
			return scanner.Err()
		}
	} else if len(c.Agents) > 0 {
		for _, id := range c.Agents {
			if strings.TrimSpace(id) != "" {
				agentIDs = append(agentIDs, id)
			}
		}
	} else {
		return errors.New("must supply agents to stop")
	}

	if len(agentIDs) == 0 {
		return errors.New("must supply agents to stop")
	}

	writer := os.Stdout
	isTTY := isatty.IsTerminal(writer.Fd())

	total := len(agentIDs)
	updates := make(chan stopResult, total)

	var wg sync.WaitGroup
	for _, id := range agentIDs {
		wg.Add(1)
		go func(agentID string) {
			defer wg.Done()
			updates <- stopAgent(ctx, agentID, f, c.Force, sem)
		}(id)
	}

	go func() {
		wg.Wait()
		close(updates)
	}()

	succeeded := 0
	failed := 0
	completed := 0
	var errorDetails []string

	if !f.Quiet {
		line := bkIO.ProgressLine("Stopping agents", completed, total, succeeded, failed, 24)
		if isTTY {
			fmt.Fprint(writer, line)
		} else {
			fmt.Fprintln(writer, line)
		}
	}

	for update := range updates {
		completed++
		if update.err != nil {
			failed++
			errorDetails = append(errorDetails, fmt.Sprintf("FAILED %s: %v", update.id, update.err))
		} else {
			succeeded++
		}

		if !f.Quiet && isTTY {
			line := bkIO.ProgressLine("Stopping agents", completed, total, succeeded, failed, 24)
			fmt.Fprintf(writer, "\r%s", line)
		}
	}

	if !f.Quiet {
		line := bkIO.ProgressLine("Stopping agents", completed, total, succeeded, failed, 24)
		if isTTY {
			fmt.Fprintln(writer)
		} else {
			fmt.Fprintln(writer, line)
		}
	}

	if len(errorDetails) > 0 {
		for _, detail := range errorDetails {
			fmt.Fprintln(os.Stderr, detail)
		}
	}

	if failed > 0 {
		return fmt.Errorf("failed to stop %d of %d agents (see above for details)", failed, total)
	}

	return nil
}

type stopResult struct {
	id  string
	err error
}

func stopAgent(ctx context.Context, id string, f *factory.Factory, force bool, sem *semaphore.Weighted) stopResult {
	org, agentID := parseAgentArg(id, f.Config)

	if err := sem.Acquire(ctx, 1); err != nil {
		return stopResult{id: id, err: err}
	}
	defer sem.Release(1)

	_, err := f.RestAPIClient.Agents.Stop(ctx, org, agentID, force)
	return stopResult{id: id, err: err}
}

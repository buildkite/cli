package agent

import (
	"bufio"
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"sync"
	"syscall"

	"github.com/alecthomas/kong"
	"github.com/buildkite/cli/v3/internal/cli"
	bkIO "github.com/buildkite/cli/v3/internal/io"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/cli/v3/pkg/cmd/validation"
	"github.com/mattn/go-isatty"
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

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	sigCh := make(chan os.Signal, 1)
	signal.Notify(sigCh, os.Interrupt, syscall.SIGTERM)
	go func() {
		<-sigCh
		cancel()
	}()

	limit := max(c.Limit, 1)

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
	label := "Stopping agents"
	if total == 1 {
		label = "Stopping agent"
	}

	workerCount := int(min(limit, int64(total)))

	work := make(chan string, workerCount)
	updates := make(chan stopResult, workerCount)

	var wg sync.WaitGroup
	for i := 0; i < workerCount; i++ {
		wg.Add(1)
		go func() {
			defer wg.Done()
			for agentID := range work {
				if ctx.Err() != nil {
					updates <- stopResult{id: agentID, err: ctx.Err()}
					continue
				}
				updates <- stopAgent(ctx, agentID, f, c.Force)
			}
		}()
	}

	go func() {
		for _, id := range agentIDs {
			select {
			case <-ctx.Done():
				close(work)
				return
			case work <- id:
			}
		}
		close(work)
	}()

	go func() {
		wg.Wait()
		close(updates)
	}()

	succeeded := 0
	failed := 0
	completed := 0
	var errorDetails []string

	if !f.Quiet {
		line := bkIO.ProgressLine(label, completed, total, succeeded, failed, 24)
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

		if !f.Quiet {
			line := bkIO.ProgressLine(label, completed, total, succeeded, failed, 24)
			if isTTY {
				fmt.Fprintf(writer, "\r%s", line)
			} else {
				fmt.Fprintln(writer, line)
			}
		}
	}

	if !f.Quiet && isTTY {
		fmt.Fprintln(writer)
	}

	summaryWriter := writer
	if failed > 0 {
		summaryWriter = os.Stderr
	}

	if len(errorDetails) > 0 {
		fmt.Fprintln(summaryWriter)
		for _, detail := range errorDetails {
			fmt.Fprintln(summaryWriter, detail)
		}
	}

	if !f.Quiet {
		agentLabel := pluralize("agent", total)
		failedLabel := pluralize("agent", failed)
		if failed > 0 {
			fmt.Fprintf(summaryWriter, "\nStopped %d of %d %s (%d %s failed)\n", succeeded, total, agentLabel, failed, failedLabel)
		} else {
			fmt.Fprintf(summaryWriter, "\nSuccessfully stopped %d of %d %s\n", succeeded, total, agentLabel)
		}
	}

	if failed > 0 {
		return fmt.Errorf("failed to stop %d of %d %s (see above for details)", failed, total, pluralize("agent", total))
	}

	return nil
}

type stopResult struct {
	id  string
	err error
}

func pluralize(word string, count int) string {
	if count == 1 {
		return word
	}
	return word + "s"
}

func stopAgent(ctx context.Context, id string, f *factory.Factory, force bool) stopResult {
	org, agentID := parseAgentArg(id, f.Config)
	_, err := f.RestAPIClient.Agents.Stop(ctx, org, agentID, force)
	return stopResult{id: id, err: err}
}

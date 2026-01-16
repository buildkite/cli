package agent

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/alecthomas/kong"
	"github.com/buildkite/cli/v3/internal/cli"
	bkIO "github.com/buildkite/cli/v3/internal/io"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/cli/v3/pkg/cmd/validation"
	"github.com/buildkite/cli/v3/pkg/output"
	buildkite "github.com/buildkite/go-buildkite/v4"
	"github.com/pkg/browser"
)

type ViewCmd struct {
	Agent  string `arg:"" help:"Agent ID to view"`
	Web    bool   `help:"Open agent in a browser" short:"w"`
	Output string `help:"Output format. One of: json, yaml, text" short:"o" default:"${output_default_format}" enum:",json,yaml,text"`
}

func (c *ViewCmd) Help() string {
	return `If the "ORGANIZATION_SLUG/" portion of the "ORGANIZATION_SLUG/UUID" agent argument
is omitted, it uses the currently selected organization.

Examples:
  # View an agent
  $ bk agent view 0198d108-a532-4a62-9bd7-b2e744bf5c45

  # View an agent with organization slug
  $ bk agent view my-org/0198d108-a532-4a62-9bd7-b2e744bf5c45

  # Open agent in browser
  $ bk agent view 0198d108-a532-4a62-9bd7-b2e744bf5c45 --web

  # View agent as JSON
  $ bk agent view 0198d108-a532-4a62-9bd7-b2e744bf5c45 --output json`
}

func (c *ViewCmd) Run(kongCtx *kong.Context, globals cli.GlobalFlags) error {
	f, err := factory.New(factory.WithDebug(globals.EnableDebug()))
	if err != nil {
		return err
	}

	f.SkipConfirm = globals.SkipConfirmation()
	f.NoInput = globals.DisableInput()
	f.Quiet = globals.IsQuiet()
	f.NoPager = f.NoPager || globals.DisablePager()

	if err := validation.ValidateConfiguration(f.Config, kongCtx.Command()); err != nil {
		return err
	}

	ctx := context.Background()

	format := output.ResolveFormat(c.Output, f.Config.OutputFormat())

	org, id := parseAgentArg(c.Agent, f.Config)

	if c.Web {
		url := fmt.Sprintf("https://buildkite.com/organizations/%s/agents/%s", org, id)
		fmt.Printf("Opening %s in your browser\n", url)
		return browser.OpenURL(url)
	}

	var agentData buildkite.Agent
	spinErr := bkIO.SpinWhile(f, "Loading agent", func() {
		agentData, _, err = f.RestAPIClient.Agents.Get(ctx, org, id)
	})
	if spinErr != nil {
		return spinErr
	}
	if err != nil {
		return err
	}

	if format != output.FormatText {
		return output.Write(os.Stdout, agentData, format)
	}

	metadata, queue := parseMetadata(agentData.Metadata)
	if metadata == "" {
		metadata = "~"
	}
	connected := "-"
	if agentData.CreatedAt != nil {
		connected = agentData.CreatedAt.Format(time.RFC3339)
	}

	headers := []string{"Property", "Value"}
	rows := [][]string{
		{"ID", agentData.ID},
		{"Name", agentData.Name},
		{"State", agentData.ConnectedState},
		{"Queue", queue},
		{"Version", agentData.Version},
		{"Hostname", agentData.Hostname},
		{"User Agent", agentData.UserAgent},
		{"IP Address", agentData.IPAddress},
		{"Connected", connected},
		{"Metadata", metadata},
	}

	table := output.Table(headers, rows, map[string]string{
		"property": "bold",
		"value":    "dim",
	})

	writer, cleanup := bkIO.Pager(f.NoPager, f.Config.Pager())
	defer func() { _ = cleanup() }()

	fmt.Fprintf(writer, "Agent %s (%s)\n\n", agentData.Name, agentData.ID)
	fmt.Fprint(writer, table)

	return nil
}

func parseMetadata(metadataList []string) (string, string) {
	var metadataTags []string
	var queue string

	if len(metadataList) == 1 {
		if queueValue := parseQueue(metadataList[0]); queueValue != "" {
			return "~", queueValue
		}
		return metadataList[0], "default"
	}

	for _, v := range metadataList {
		if queueValue := parseQueue(v); queueValue != "" {
			queue = queueValue
		} else {
			metadataTags = append(metadataTags, v)
		}
	}

	if queue == "" {
		queue = "default"
	}

	metadata := strings.Join(metadataTags, ", ")
	return metadata, queue
}

func parseQueue(metadata string) string {
	parts := strings.Split(metadata, "=")
	if len(parts) > 1 && parts[0] == "queue" {
		return parts[1]
	}
	return ""
}

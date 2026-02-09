package secret

import (
	"context"
	"fmt"
	"os"
	"os/signal"
	"strings"
	"syscall"
	"time"

	"github.com/alecthomas/kong"
	"github.com/buildkite/cli/v3/internal/cli"
	bkIO "github.com/buildkite/cli/v3/internal/io"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/cli/v3/pkg/cmd/validation"
	"github.com/buildkite/cli/v3/pkg/output"
	buildkite "github.com/buildkite/go-buildkite/v4"
)

type GetCmd struct {
	ClusterID string `help:"The ID of the cluster" required:"" name:"cluster-id"`
	SecretID  string `help:"The UUID of the secret to view" required:"" name:"secret-id"`
	Output    string `help:"Output format. One of: json, yaml, text" short:"o" default:"${output_default_format}" enum:",json,yaml,text"`
}

func (c *GetCmd) Help() string {
	return `
View details of a cluster secret.

Examples:
  # View a secret
  $ bk secret get --cluster-id my-cluster-id --secret-id my-secret-id

  # View a secret in JSON format
  $ bk secret get --cluster-id my-cluster-id --secret-id my-secret-id -o json
`
}

func (c *GetCmd) Run(kongCtx *kong.Context, globals cli.GlobalFlags) error {
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

	format := output.ResolveFormat(c.Output, f.Config.OutputFormat())

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	var secret buildkite.ClusterSecret
	spinErr := bkIO.SpinWhile(f, "Loading secret", func() {
		secret, _, err = f.RestAPIClient.ClusterSecrets.Get(ctx, f.Config.OrganizationSlug(), c.ClusterID, c.SecretID)
	})
	if spinErr != nil {
		return spinErr
	}
	if err != nil {
		return fmt.Errorf("error fetching secret: %v", err)
	}

	secretView := output.Viewable[buildkite.ClusterSecret]{
		Data:   secret,
		Render: renderSecretText,
	}

	if format != output.FormatText {
		return output.Write(os.Stdout, secretView, format)
	}

	writer, cleanup := bkIO.Pager(f.NoPager, f.Config.Pager())
	defer func() { _ = cleanup() }()

	return output.Write(writer, secretView, format)
}

func renderSecretText(s buildkite.ClusterSecret) string {
	rows := [][]string{
		{"Key", output.ValueOrDash(s.Key)},
		{"ID", output.ValueOrDash(s.ID)},
		{"Description", output.ValueOrDash(s.Description)},
		{"Policy", output.ValueOrDash(s.Policy)},
	}

	if s.CreatedBy.ID != "" {
		rows = append(rows,
			[]string{"Created By", output.ValueOrDash(s.CreatedBy.Name)},
		)
	}

	if s.CreatedAt != nil {
		rows = append(rows, []string{"Created At", s.CreatedAt.Format(time.RFC3339)})
	}

	if s.UpdatedBy != nil && s.UpdatedBy.ID != "" {
		rows = append(rows,
			[]string{"Updated By", output.ValueOrDash(s.UpdatedBy.Name)},
		)
	}

	if s.UpdatedAt != nil {
		rows = append(rows, []string{"Updated At", s.UpdatedAt.Format(time.RFC3339)})
	}

	if s.LastReadAt != nil {
		rows = append(rows, []string{"Last Read At", s.LastReadAt.Format(time.RFC3339)})
	}

	var sb strings.Builder
	fmt.Fprintf(&sb, "Viewing secret %s\n\n", output.ValueOrDash(s.Key))

	table := output.Table(
		[]string{"Field", "Value"},
		rows,
		map[string]string{"field": "dim", "value": "italic"},
	)

	sb.WriteString(table)
	return sb.String()
}

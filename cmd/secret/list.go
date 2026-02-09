package secret

import (
	"context"
	"errors"
	"fmt"
	"os"
	"os/signal"
	"syscall"

	"github.com/alecthomas/kong"
	"github.com/buildkite/cli/v3/internal/cli"
	bkIO "github.com/buildkite/cli/v3/internal/io"
	internalSecret "github.com/buildkite/cli/v3/internal/secret"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/cli/v3/pkg/cmd/validation"
	"github.com/buildkite/cli/v3/pkg/output"
	buildkite "github.com/buildkite/go-buildkite/v4"
)

type ListCmd struct {
	ClusterID string `help:"The ID of the cluster to list secrets for" required:"" name:"cluster-id"`
	Output    string `help:"Output format. One of: json, yaml, text" short:"o" default:"${output_default_format}" enum:",json,yaml,text"`
}

func (c *ListCmd) Help() string {
	return `
List secrets for a cluster.

Examples:
  # List all secrets in a cluster
  $ bk secret list --cluster-id my-cluster-id

  # List secrets in JSON format
  $ bk secret list --cluster-id my-cluster-id -o json
`
}

func (c *ListCmd) Run(kongCtx *kong.Context, globals cli.GlobalFlags) error {
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

	var secrets []buildkite.ClusterSecret
	spinErr := bkIO.SpinWhile(f, "Loading secrets", func() {
		secrets, _, err = f.RestAPIClient.ClusterSecrets.List(ctx, f.Config.OrganizationSlug(), c.ClusterID, nil)
	})
	if spinErr != nil {
		return spinErr
	}
	if err != nil {
		return fmt.Errorf("error fetching secrets: %v", err)
	}

	if len(secrets) == 0 {
		return errors.New("no secrets found for cluster")
	}

	if format != output.FormatText {
		return output.Write(os.Stdout, secrets, format)
	}

	summary := internalSecret.SecretViewTable(secrets...)

	writer, cleanup := bkIO.Pager(f.NoPager, f.Config.Pager())
	defer func() { _ = cleanup() }()

	fmt.Fprintf(writer, "%v\n", summary)

	return nil
}

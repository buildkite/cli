package secret

import (
	"context"
	"fmt"
	"io"
	"os"
	"os/signal"
	"syscall"
	"unicode/utf8"

	"github.com/alecthomas/kong"
	"github.com/buildkite/cli/v3/internal/cli"
	bkIO "github.com/buildkite/cli/v3/internal/io"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/cli/v3/pkg/cmd/validation"
	"github.com/buildkite/cli/v3/pkg/output"
	buildkite "github.com/buildkite/go-buildkite/v5"
)

type CreateCmd struct {
	ClusterUUID string `help:"The UUID of the cluster" required:"" name:"cluster-uuid"`
	Key         string `help:"The key name for the secret (e.g. MY_SECRET)" required:""`
	Value       string `help:"The secret value. If neither value source is provided, you will be prompted to enter it." optional:"" xor:"value-source"`
	ValueFile   string `help:"Read the secret value from a file, or from stdin with -. Content is preserved exactly." optional:"" name:"value-file" xor:"value-source"`
	Description string `help:"A description of the secret" optional:""`
	Policy      string `help:"The access policy for the secret (YAML format)" optional:""`
	output.OutputFlags
}

func (c *CreateCmd) Help() string {
	return `
Create a new secret in a cluster.

Use --value-file to read the value exactly as stored, including multiline content
and any trailing LF or CRLF. Use --value-file - to read the value from stdin.
File and stdin values must be non-empty valid UTF-8.

If neither --value nor --value-file is provided, you will be prompted to enter
the secret value interactively (input will be masked).

Examples:
  # Create a secret with interactive value input
  $ bk secret create --cluster-uuid my-cluster-uuid --key MY_SECRET

  # Create a secret with the value provided inline
  $ bk secret create --cluster-uuid my-cluster-uuid --key MY_SECRET --value "s3cr3t"

  # Create a secret from a file, preserving its content exactly
  $ bk secret create --cluster-uuid my-cluster-uuid --key MY_SECRET --value-file ./secret.txt

  # Create a secret from stdin (including the trailing newline from printf)
  $ printf 'line one\nline two\n' | bk secret create --no-input --cluster-uuid my-cluster-uuid --key MY_SECRET --value-file -

  # Create a secret with a description
  $ bk secret create --cluster-uuid my-cluster-uuid --key MY_SECRET --description "My secret description"
`
}

func (c *CreateCmd) Run(kongCtx *kong.Context, globals cli.GlobalFlags) error {
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

	value, err := resolveSecretValue(c.Value, c.ValueFile, os.Stdin)
	if err != nil {
		return err
	}
	if value == "" {
		if f.NoInput {
			return fmt.Errorf("--value or --value-file is required when --no-input is set")
		}
		fmt.Fprint(os.Stderr, "Enter secret value: ")
		value, err = bkIO.ReadPassword()
		fmt.Fprintln(os.Stderr)
		if err != nil {
			return fmt.Errorf("error reading secret value: %v", err)
		}
		if value == "" {
			return fmt.Errorf("secret value cannot be empty")
		}
	}

	format := output.ResolveFormat(c.Output, f.Config.OutputFormat())

	ctx, stop := signal.NotifyContext(context.Background(), os.Interrupt, syscall.SIGTERM)
	defer stop()

	input := buildkite.ClusterSecretCreate{
		Key:         c.Key,
		Value:       value,
		Description: c.Description,
		Policy:      c.Policy,
	}

	var secret buildkite.ClusterSecret
	if err = bkIO.SpinWhile(f, "Creating secret", func() error {
		var apiErr error
		secret, _, apiErr = f.RestAPIClient.ClusterSecrets.Create(ctx, f.Config.OrganizationSlug(), c.ClusterUUID, input)
		return apiErr
	}); err != nil {
		return fmt.Errorf("error creating secret: %v", err)
	}

	secretView := output.Viewable[buildkite.ClusterSecret]{
		Data:   secret,
		Render: renderSecretText,
	}

	if format != output.FormatText {
		return output.Write(os.Stdout, secretView, format)
	}

	fmt.Fprintf(os.Stdout, "Secret %s created successfully\n\n", secret.Key)
	return output.Write(os.Stdout, secretView, format)
}

func resolveSecretValue(value, valueFile string, stdin io.Reader) (string, error) {
	if valueFile == "" {
		return value, nil
	}

	var (
		contents []byte
		err      error
		source   string
	)
	if valueFile == "-" {
		contents, err = io.ReadAll(stdin)
		source = "stdin"
	} else {
		contents, err = os.ReadFile(valueFile)
		source = fmt.Sprintf("file %q", valueFile)
	}
	if err != nil {
		return "", fmt.Errorf("read secret value from %s: %w", source, err)
	}
	if len(contents) == 0 {
		return "", fmt.Errorf("secret value from %s cannot be empty", source)
	}
	if !utf8.Valid(contents) {
		return "", fmt.Errorf("secret value from %s is not valid UTF-8", source)
	}

	return string(contents), nil
}

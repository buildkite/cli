package pkg

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/alecthomas/kong"
	"github.com/buildkite/cli/v3/internal/cli"
	bkIO "github.com/buildkite/cli/v3/internal/io"
	"github.com/buildkite/cli/v3/internal/util"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/go-buildkite/v4"
)

var (
	ErrInvalidConfig = errors.New("invalid config")
	ErrAPIError      = errors.New("API error")

	// To be overridden in testing
	// Actually diddling an io.Reader so that it looks like a readable stdin is tricky
	// so we'll just stub this out
	isStdInReadableFunc = isStdinReadable
)

type PushCmd struct {
	RegistrySlug  string `arg:"" required:"" help:"The slug of the registry to push the package to" `
	FilePath      string `xor:"input" help:"Path to the package file to push"`
	StdinFileName string `xor:"input" help:"The filename to use when reading the package from stdin"`
	StdInArg      string `arg:"" optional:"" hidden:"" help:"Use '-' as value to pass package via stdin. Required if --stdin-file-name is used."`
	Web           bool   `short:"w" help:"Open the pipeline in a web browser." `
}

func (c *PushCmd) Help() string {
	return `Push a new package to a Buildkite registry. The package can be passed as a path to a file with the --file-path flag,
or via stdin. If passed via stdin, the filename must be provided with the --stdin-file-name flag, as a Buildkite
registry requires a filename for the package.

Examples:
	Push a package to a Buildkite registry
	The web URL of the uploaded package will be printed to stdout.

	# Push package from file
	$ bk package push my-registry --file-path my-package.tar.gz

	# Push package via stdin
	$ cat my-package.tar.gz | bk package push my-registry --stdin-file-name my-package.tar.gz - # Pass package via stdin, note hyphen as the argument

	# add -w to open the build in your web browser
	$ bk package push my-registry --file-path my-package.tar.gz -w
`
}

func (c *PushCmd) Run(kongCtx *kong.Context, globals cli.GlobalFlags) error {
	f, err := factory.New(factory.WithDebug(globals.EnableDebug()))
	if err != nil {
		return err
	}

	f.SkipConfirm = globals.SkipConfirmation()
	f.NoInput = globals.DisableInput()
	f.Quiet = globals.IsQuiet()

	err = c.Validate()
	if err != nil {
		return fmt.Errorf("failed to validate flags and args: %w", err)
	}

	var (
		from        io.Reader
		packageName string
	)

	switch {
	case c.FilePath != "":
		packageName = c.FilePath
		file, err := os.Open(c.FilePath)
		if err != nil {
			return fmt.Errorf("couldn't open file %s: %w", c.FilePath, err)
		}
		defer file.Close()

		from = file
	case c.StdinFileName != "":
		packageName = c.StdinFileName
		from = os.Stdin

	default:
		panic("Neither file path nor stdin file name are available, there has been an error in the config validation. Report this to support@buildkite.com")
	}

	ctx := context.Background()
	var pkg buildkite.Package
	spinErr := bkIO.SpinWhile(f, "Pushing file", func() {
		pkg, _, err = f.RestAPIClient.PackagesService.Create(ctx, f.Config.OrganizationSlug(), c.RegistrySlug, buildkite.CreatePackageInput{
			Filename: packageName,
			Package:  from,
		})
	})
	if spinErr != nil {
		return spinErr
	}
	if err != nil {
		return fmt.Errorf("%w: request to create package failed: %w", ErrAPIError, err)
	}

	return util.OpenInWebBrowser(c.Web, pkg.WebURL)
}

func isStdinReadable() (bool, error) {
	stat, err := os.Stdin.Stat()
	if err != nil {
		return false, fmt.Errorf("failed to stat stdin: %w", err)
	}

	readable := (stat.Mode() & os.ModeCharDevice) == 0
	return readable, nil
}

func (c *PushCmd) Validate() error {
	// Validate the args such that either a file path is provided or stdin is being used

	// check if c.FilePath and c.Stdin cannot be both set or both empty
	if c.FilePath == "" && c.StdinFileName == "" {
		return fmt.Errorf("%w: either a file path argument or --stdin-file-name must be provided", ErrInvalidConfig)
	}

	if c.FilePath != "" && c.StdinFileName != "" {
		return fmt.Errorf("%w: cannot provide both a file path argument and --stdin-file-name", ErrInvalidConfig)
	}

	if c.StdinFileName != "" {
		if c.StdInArg != "-" {
			return fmt.Errorf("%w:  when passing a package file via stdin, the final argument must be '-'", ErrInvalidConfig)
		}

		stdInReadable, err := isStdInReadableFunc()
		if err != nil {
			return fmt.Errorf("failed to check if stdin is readable: %w", err)
		}

		if !stdInReadable {
			return fmt.Errorf("%w: stdin is not readable", ErrInvalidConfig)
		}

		return nil
	} else {
		// Validate if an std-in arg is provided without stdin-file-name
		if c.StdInArg == "-" {
			return fmt.Errorf("%w: when passing a package file via stdin, --stdin-file-name must be provided", ErrInvalidConfig)
		}
		// We have a file path, check it exists and is a regular file
		fi, err := os.Stat(c.FilePath)
		if err != nil {
			return fmt.Errorf("%w: %w", ErrInvalidConfig, err)
		}

		if !fi.Mode().IsRegular() {
			mode := "directory"
			if !fi.Mode().IsDir() {
				mode = fi.Mode().String()
			}
			return fmt.Errorf("%w: file at %s is not a regular file, mode was: %s", ErrInvalidConfig, c.FilePath, mode)
		}

		return nil
	}
}

package pkg

import (
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/MakeNowJust/heredoc"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/go-buildkite/v3/buildkite"
	"github.com/kr/pretty"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

type pushPackageConfig struct {
	RegistrySlug  string
	FilePath      string
	StdinFileName string
}

const stdinFileNameFlag = "stdin-file-name"

func NewCmdPackagePush(f *factory.Factory) *cobra.Command {
	cmd := cobra.Command{
		Use:   "push registry-name {path/to/file | --stdin-file-name filename -}",
		Short: "Push a new package to Buildkite Packages",
		Long: heredoc.Doc(`
			Push a new package to Buildkite Packages. The package can be passed as a path to a file in the second positional argument,
			or via stdin. If passed via stdin, the filename must be provided with the --stdin-file-name flag, as Buildkite
			Packages requires a filename for the package.`),
		Example: heredoc.Doc(`
			$ bk package push my-registry my-package.tar.gz
			$ cat my-package.tar.gz | bk package push my-registry --stdin-file-name my-package.tar.gz - # Pass package via stdin, note hyphen as the argument
		`),
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := loadAndValidateConfig(cmd.Flags(), args)
			if err != nil {
				return fmt.Errorf("failed to validate flags and args: %w", err)
			}

			packageName := ""
			var from io.Reader
			switch {
			case cfg.FilePath != "":
				packageName = cfg.FilePath

				file, err := os.Open(cfg.FilePath)
				if err != nil {
					return fmt.Errorf("couldn't open file %s: %w", cfg.FilePath, err)
				}

				defer file.Close()

				from = file
			case cfg.StdinFileName != "":
				packageName = cfg.StdinFileName
				from = cmd.InOrStdin()
			}

			r, w := io.Pipe()
			go func() {
				if _, err := io.Copy(w, from); err != nil {
					w.CloseWithError(fmt.Errorf("failed to read package from input: %w", err))
				}
				w.Close()
			}()

			pkg, _, err := f.RestAPIClient.PackagesService.Create(f.Config.OrganizationSlug(), cfg.RegistrySlug, buildkite.CreatePackageInput{
				Filename: packageName,
				Package:  r,
			})
			if err != nil {
				return fmt.Errorf("%w: request to create package failed: %w", ErrAPIError, err)
			}

			fmt.Fprintln(cmd.OutOrStdout(), "Created package:")
			pretty.Fprintf(cmd.OutOrStdout(), "%# v\n", pkg)
			return nil
		},
	}

	cmd.Flags().StringP(stdinFileNameFlag, "n", "", "The filename to use for the package, if it's passed via stdin. Invalid otherwise.")

	return &cmd
}

var (
	ErrInvalidConfig = errors.New("invalid config")
	ErrAPIError      = errors.New("API error")

	// To be overridden in testing
	// Actually diddling an io.Reader so that it looks like a readable stdin is tricky
	// so we'll just stub this out
	isStdInReadableFunc = isStdinReadable
)

func isStdinReadable() (bool, error) {
	stat, err := os.Stdin.Stat()
	if err != nil {
		return false, fmt.Errorf("failed to stat stdin: %w", err)
	}

	readable := (stat.Mode() & os.ModeCharDevice) == 0
	return readable, nil
}

func loadAndValidateConfig(flags *pflag.FlagSet, args []string) (*pushPackageConfig, error) {
	if len(args) != 2 {
		return nil, fmt.Errorf("%w: Exactly 2 arguments are required, got: %d", ErrInvalidConfig, len(args))
	}

	if stdinFileName := flags.Lookup(stdinFileNameFlag); stdinFileName != nil && stdinFileName.Value.String() != "" {
		if args[1] != "-" {
			return nil, fmt.Errorf("%w: When passing a package via stdin, the final argument must be '-'", ErrInvalidConfig)
		}

		stdInReadable, err := isStdInReadableFunc()
		if err != nil {
			return nil, fmt.Errorf("failed to check if stdin is readable: %w", err)
		}

		if !stdInReadable {
			return nil, fmt.Errorf("%w: stdin is not readable", ErrInvalidConfig)
		}

		return &pushPackageConfig{
			RegistrySlug:  args[0],
			StdinFileName: stdinFileName.Value.String(),
		}, nil
	} else {
		// No stdin file name, so we expect a file path as the second argument
		filePath := args[1]
		fi, err := os.Stat(filePath)
		if err != nil {
			return nil, fmt.Errorf("%w: %w", ErrInvalidConfig, err)
		}

		if !fi.Mode().IsRegular() {
			mode := "directory"
			if !fi.Mode().IsDir() {
				mode = fi.Mode().String()
			}
			return nil, fmt.Errorf("%w: file at %s is not a regular file, mode was: %s", ErrInvalidConfig, filePath, mode)
		}

		return &pushPackageConfig{
			RegistrySlug: args[0],
			FilePath:     filePath,
		}, nil
	}
}

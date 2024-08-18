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
	"github.com/oleiade/reflections"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

type newPackageConfig struct {
	RegistrySlug string `flag:"registry"`
	FilePath     string `flag:"file"`
	FileName     string `flag:"file-name"`
}

func NewCmdPackagePush(f *factory.Factory) *cobra.Command {
	cmd := cobra.Command{
		Use:   "push --registry <registry> {--file <file> | --file-name <filename> -}",
		Short: "Push a new package to Buildkite Packages",
		Example: heredoc.Doc(`
			$ bk package push --registry my-registry --file my-package.tar.gz
			$ cat my-package.tar.gz | bk package push --registry my-registry --file-name my-package.tar.gz - # Pass package via stdin, note hyphen as the argument
		`),
		Args:      cobra.MaximumNArgs(1),
		ValidArgs: []string{"-"},
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := populateFlags[newPackageConfig](cmd.Flags())
			if err != nil {
				return fmt.Errorf("failed to populate flags: %w", err)
			}

			if err := validateConfig(cfg, args); err != nil {
				return fmt.Errorf("failed to validate flags and args: %w", err)
			}

			var from io.Reader
			switch {
			case cfg.FilePath != "":
				file, err := os.Open(cfg.FilePath)
				if err != nil {
					return fmt.Errorf("couldn't open file %s: %w", cfg.FilePath, err)
				}

				defer file.Close()

				from = file
			case len(args) != 0 && args[0] == "-":
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
				Filename: cfg.FileName,
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

	cmd.Flags().StringP("registry", "r", "", "The slug for the registry to create the package in")
	cmd.Flags().StringP("file", "f", "", "The path to the package file to upload. Cannot be used when package is passed via stdin")
	cmd.Flags().StringP("file-name", "n", "", "The filename to use for the package, if it's passed via stdin. Invalid otherwise.")

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

func validateConfig(config *newPackageConfig, args []string) error {
	if len(config.RegistrySlug) == 0 {
		return fmt.Errorf("%w, --registry is required", ErrInvalidConfig)
	}

	stdInReadable, err := isStdInReadableFunc()
	if err != nil {
		return fmt.Errorf("failed to check if stdin is readable: %w", err)
	}

	if stdInReadable {
		switch {
		case len(args) == 0:
			return fmt.Errorf("%w: the final argument must be '-' when passing package via stdin", ErrInvalidConfig)
		case len(args) != 0 && args[0] == "-":
			// We're reading the package file in from stdin
			if len(config.FilePath) != 0 {
				return fmt.Errorf("%w: cannot use --file when package is passed via stdin", ErrInvalidConfig)
			}

			if len(config.FileName) == 0 {
				return fmt.Errorf("%w: --file-name is required when package is passed via stdin", ErrInvalidConfig)
			}

			return nil
		}
	}

	if config.FilePath != "" {
		fi, err := os.Stat(config.FilePath)
		if err != nil {
			return fmt.Errorf("%w: file %s did not exist: %w", ErrInvalidConfig, config.FilePath, err)
		}

		if !fi.Mode().IsRegular() {
			mode := "directory"
			if !fi.Mode().IsDir() {
				mode = fi.Mode().String()
			}
			return fmt.Errorf("%w: package file at %s is not a regular file, was: %s", ErrInvalidConfig, config.FilePath, mode)
		}
	}

	return nil
}

func populateFlags[T any](flagSet *pflag.FlagSet) (*T, error) {
	c := new(T)

	fields, err := reflections.Fields(c)
	if err != nil {
		return new(T), fmt.Errorf("getting fields for newPackageConfig: %w", err)
	}

	fieldsForFlagName := map[string]string{}
	for _, field := range fields {
		tag, err := reflections.GetFieldTag(c, field, "flag")
		if err != nil {
			return new(T), fmt.Errorf("getting flag tag for field %s: %w", field, err)
		}
		fieldsForFlagName[tag] = field
	}

	var multiErr error
	flagSet.VisitAll(func(f *pflag.Flag) {
		if field, ok := fieldsForFlagName[f.Name]; ok {
			if err := reflections.SetField(c, field, f.Value.String()); err != nil {
				multiErr = errors.Join(multiErr, fmt.Errorf("setting field %s: %w", field, err))
			}
		}
	})
	if multiErr != nil {
		return new(T), fmt.Errorf("errors populating flags: %w", multiErr)
	}

	return c, nil
}

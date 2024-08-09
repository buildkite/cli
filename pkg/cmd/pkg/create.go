package pkg

import (
	"bytes"
	"errors"
	"fmt"
	"io"
	"os"

	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/go-buildkite/v3/buildkite"
	"github.com/kr/pretty"
	"github.com/oleiade/reflections"
	"github.com/spf13/cobra"
	"github.com/spf13/pflag"
)

var (
// packageCreateOrganization = pflag.StringP("organization", "o", "", "The organization to create the package in")
// packageCreateRegistrySlug = pflag.StringP("registry", "r", "", "The slug for the registry to create the package in")
// packageCreateFilePath     = pflag.StringP("file", "f", "", "The path to the package file to upload. Cannot be used when package is passed via stdin")
// packageCreateFilename     = pflag.StringP("file-name", "n", "", "The filename to use for the package, if it's passed via stdin. Invalid otherwise.")
)

type newPackageConfig struct {
	Organization string `flag:"organization"`
	RegistrySlug string `flag:"registry"`
	FilePath     string `flag:"file"`
	FileName     string `flag:"file-name"`
}

func NewCmdPackageCreate(f *factory.Factory) *cobra.Command {
	cmd := cobra.Command{
		Use:       "create <org>/<name>",
		Short:     "Create a new package",
		Args:      cobra.MaximumNArgs(1),
		ValidArgs: []string{"-"},
		RunE: func(cmd *cobra.Command, args []string) error {
			cfg, err := populateFlags(cmd.PersistentFlags())
			if err != nil {
				return fmt.Errorf("failed to populate flags: %w", err)
			}

			if err := validateInputs(*cfg, args); err != nil {
				return fmt.Errorf("failed to validate flags and args: %w", err)
			}

			var cpi buildkite.CreatePackageInput
			switch {
			case cfg.FilePath != "":
				file, err := os.Open(cfg.FilePath)
				if err != nil {
					return fmt.Errorf("couldn't open file %s: %w", cfg.FilePath, err)
				}

				cpi = buildkite.CreatePackageInput{Package: file}
			case cfg.FileName != "":
				// Read the file from stdin
				var b *bytes.Buffer
				_, err := io.Copy(b, os.Stdin)
				if err != nil {
					return fmt.Errorf("reading stdin: %w", err)
				}

				cpi = buildkite.CreatePackageInput{
					Package:  b,
					Filename: cfg.FileName,
				}
			}

			pkg, _, err := f.RestAPIClient.PackagesService.Create(cfg.Organization, cfg.RegistrySlug, cpi)
			if err != nil {
				return fmt.Errorf("making create package API request: %w", err)
			}

			fmt.Println("Created package:")
			pretty.Println(pkg)
			return nil
		},
	}

	cmd.PersistentFlags().StringP("organization", "o", "", "The organization to create the package in")
	cmd.PersistentFlags().StringP("registry", "r", "", "The slug for the registry to create the package in")
	cmd.PersistentFlags().StringP("file", "f", "", "The path to the package file to upload. Cannot be used when package is passed via stdin")
	cmd.PersistentFlags().StringP("file-name", "n", "", "The filename to use for the package, if it's passed via stdin. Invalid otherwise.")

	return &cmd
}

func validateInputs(config newPackageConfig, args []string) error {
	if len(config.Organization) == 0 {
		return fmt.Errorf("organization is required")
	}

	if len(config.RegistrySlug) == 0 {
		return fmt.Errorf("registry is required")
	}

	stdInReadable, err := isStdinReadable()
	if err != nil {
		return fmt.Errorf("failed to check if stdin is readable: %w", err)
	}

	if stdInReadable && (len(args) == 0 || args[0] == "-") {
		// We're reading the package file in from stdin
		if len(config.FilePath) != 0 {
			return fmt.Errorf("cannot use --file when package is passed via stdin")
		}

		if len(config.FileName) == 0 {
			return fmt.Errorf("filename is required when package is passed via stdin")
		}
	}

	if config.FilePath != "" {
		fi, err := os.Stat(config.FilePath)
		if err != nil {
			return fmt.Errorf("failed to stat package file: %w", err)
		}

		if !fi.Mode().IsRegular() {
			return fmt.Errorf("package file at %s is not a regular file, was: %s", config.FilePath, fi.Mode())
		}
	}

	return nil
}

func isStdinReadable() (bool, error) {
	stat, err := os.Stdin.Stat()
	if err != nil {
		return false, fmt.Errorf("failed to stat stdin: %w", err)
	}

	readable := (stat.Mode() & os.ModeCharDevice) == 0
	return readable, nil
}

func populateFlags(flagSet *pflag.FlagSet) (*newPackageConfig, error) {
	c := &newPackageConfig{}

	fields, err := reflections.Fields(c)
	if err != nil {
		return nil, fmt.Errorf("getting fields for newPackageConfig: %w", err)
	}

	fieldsForFlagName := map[string]string{}
	for _, field := range fields {
		tag, err := reflections.GetFieldTag(c, field, "flag")
		if err != nil {
			return nil, fmt.Errorf("getting flag tag for field %s: %w", field, err)
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
		return nil, fmt.Errorf("errors populating flags: %w", multiErr)
	}

	return c, nil
}

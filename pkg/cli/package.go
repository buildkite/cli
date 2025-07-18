package cli

import (
	"context"
	"fmt"
	"io"
	"os"

	bk_io "github.com/buildkite/cli/v3/internal/io"
	"github.com/buildkite/cli/v3/internal/util"
	"github.com/buildkite/cli/v3/pkg/factory"
	buildkite "github.com/buildkite/go-buildkite/v4"
)

// Package command
type PackageCmd struct {
	Push PackagePushCmd `cmd:"" help:"Push package to registry"`
}

type PackagePushCmd struct {
	Registry      string `arg:"" help:"Registry slug"`
	File          string `arg:"" help:"Package file to push (use '-' for stdin)" predictor:"file"`
	StdinFileName string `help:"Filename for stdin data"`
	Web           bool   `help:"Open results in web browser"`
}

func (p *PackagePushCmd) Help() string {
	return `Upload packages to Buildkite package registries.

EXAMPLES:
  # Push a compiled binary
  bk pkg push binaries myapp-v1.2.0-linux-amd64.tar.gz

  # Push a Docker image archive
  bk pkg push docker-images myapp-latest.tar

  # Push from stdin with custom filename
  tar -czf - ./dist | bk pkg push releases - --stdin-file-name myapp-v1.2.0.tar.gz

  # Push and open in web browser
  bk pkg push artifacts build-123.zip --web`
}

// Package command implementations
func (p *PackagePushCmd) Run(ctx context.Context, f *factory.Factory) error {
	// Validate arguments first (before config/API validation)
	if p.Registry == "" {
		return fmt.Errorf("registry slug is required")
	}

	if p.File == "-" && p.StdinFileName == "" {
		return fmt.Errorf("when using stdin (file = '-'), --stdin-file-name is required")
	}

	if p.File == "" {
		return fmt.Errorf("file path is required (or use '-' with --stdin-file-name for stdin)")
	}

	if err := validateConfig(f.Config); err != nil {
		return err
	}

	var (
		from        io.Reader
		packageName string
	)

	// Handle file input vs stdin
	if p.File == "-" {
		// Stdin input
		packageName = p.StdinFileName
		from = os.Stdin
	} else {
		// File input
		packageName = p.File
		file, err := os.Open(p.File)
		if err != nil {
			return fmt.Errorf("couldn't open file %s: %w", p.File, err)
		}
		defer file.Close()
		from = file
	}

	// Push package to Buildkite
	var pkg buildkite.Package
	var err error
	spinErr := bk_io.SpinWhile("Pushing package", func() {
		pkg, _, err = f.RestAPIClient.PackagesService.Create(ctx, f.Config.OrganizationSlug(), p.Registry, buildkite.CreatePackageInput{
			Filename: packageName,
			Package:  from,
		})
	})
	if spinErr != nil {
		return spinErr
	}
	if err != nil {
		return fmt.Errorf("failed to create package: %w", err)
	}

	fmt.Printf("Created package file: %s\n", pkg.Name)
	fmt.Printf("View this file on the web at: %s\n", pkg.WebURL)

	// Open in web browser if requested
	if p.Web {
		return util.OpenInWebBrowser(true, pkg.WebURL)
	}

	return nil
}

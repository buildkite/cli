package pkg

import (
	"context"
	"fmt"
	"net/url"
	"strings"

	"github.com/alecthomas/kong"
	"github.com/buildkite/cli/v3/internal/cli"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/cli/v3/pkg/cmd/validation"
	"github.com/buildkite/cli/v3/pkg/output"
	"github.com/buildkite/go-buildkite/v5"
)

type RegistryUpdateCmd struct {
	RegistrySlug string `arg:"" help:"Slug of the Packages registry to update"`
	Visibility   string `required:"" enum:"public,private" help:"Set the Packages registry visibility: public or private"`
	output.OutputFlags
}

func (c *RegistryUpdateCmd) Help() string {
	return `
Requires an API token with the write_registries scope and permission to edit
the registry. Making a registry public can also make its organization public.

Examples:
  $ bk package registry update my-registry --org my-org --visibility public
  $ bk package registry update my-registry --org my-org --visibility private
`
}

func (c *RegistryUpdateCmd) Run(kongCtx *kong.Context, globals cli.GlobalFlags) error {
	f, err := factory.New(factory.WithDebug(globals.EnableDebug()))
	if err != nil {
		return err
	}
	org := f.Config.OrganizationSlug()
	if org == "" {
		return fmt.Errorf("no organization selected; specify --org or run bk auth switch")
	}
	if err := validation.ValidateConfiguration(f.Config, kongCtx.Command()); err != nil {
		return err
	}
	for _, slug := range []string{org, c.RegistrySlug} {
		if slug == "" || slug == "." || slug == ".." || strings.ContainsAny(slug, "/\\") {
			return fmt.Errorf("expected an organization and registry slug, got invalid slug %q", slug)
		}
	}

	registry, _, err := f.RestAPIClient.PackageRegistriesService.Update(context.Background(), url.PathEscape(org), url.PathEscape(c.RegistrySlug),
		buildkite.UpdatePackageRegistryInput{Visibility: buildkite.Some(c.Visibility)})
	if err != nil {
		return fmt.Errorf("updating Packages registry visibility: %w", err)
	}
	visibility := "private"
	if registry.Public {
		visibility = "public"
	}
	if visibility != c.Visibility {
		return fmt.Errorf("requested %s visibility, but the API reports %s for Packages registry %s/%s; the API may not support visibility updates", c.Visibility, visibility, org, c.RegistrySlug)
	}

	format := output.ResolveFormat(c.Output, f.Config.OutputFormat())
	return output.WriteTextOrStructured(kongCtx.Stdout, format, registry, fmt.Sprintf("Packages registry %s/%s is %s", org, c.RegistrySlug, visibility))
}

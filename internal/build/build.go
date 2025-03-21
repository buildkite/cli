package build

import (
	"github.com/buildkite/cli/v3/internal/models"
)

// Build represents a Buildkite build
// For backward compatibility, we're keeping the same field names as before
// but the underlying type is models.Build
type Build struct {
	Organization string
	Pipeline     string
	BuildNumber  int
}

// ToModel converts the Build to a models.Build
func (b *Build) ToModel() *models.Build {
	modelBuild := &models.Build{
		Organization: b.Organization,
		BuildNumber:  b.BuildNumber,
	}
	
	// Pipeline is now a struct, not a string
	if b.Pipeline != "" {
		modelBuild.Pipeline = &models.Pipeline{
			Slug: b.Pipeline,
		}
	}
	
	return modelBuild
}

// FromModel creates a Build from a models.Build
func FromModel(mb *models.Build) *Build {
	build := &Build{
		Organization: mb.Organization,
		BuildNumber:  mb.BuildNumber,
	}
	
	// Pipeline is now a struct, not a string
	if mb.Pipeline != nil {
		build.Pipeline = mb.Pipeline.Slug
	}
	
	return build
}
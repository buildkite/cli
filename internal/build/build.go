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
	return &models.Build{
		Organization: b.Organization,
		Pipeline:     b.Pipeline,
		BuildNumber:  b.BuildNumber,
	}
}

// FromModel creates a Build from a models.Build
func FromModel(mb *models.Build) *Build {
	return &Build{
		Organization: mb.Organization,
		Pipeline:     mb.Pipeline,
		BuildNumber:  mb.BuildNumber,
	}
}
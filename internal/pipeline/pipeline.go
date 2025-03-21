package pipeline

import (
	"github.com/buildkite/cli/v3/internal/models"
)

// Pipeline is a struct containing information about a pipeline for a resolver to return
// For backward compatibility, we're using the same field names as before
// but the underlying type is models.Pipeline
type Pipeline struct {
	Name string
	Org  string
}

// ToModel converts the Pipeline to a models.Pipeline
func (p *Pipeline) ToModel() *models.Pipeline {
	return &models.Pipeline{
		Name: p.Name,
		Org:  p.Org,
	}
}

// FromModel creates a Pipeline from a models.Pipeline
func FromModel(mp *models.Pipeline) *Pipeline {
	return &Pipeline{
		Name: mp.Name,
		Org:  mp.Org,
	}
}
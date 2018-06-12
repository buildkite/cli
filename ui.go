package cli

import (
	"github.com/manifoldco/promptui"
)

type pipelineSelect struct {
	Pipelines []pipeline
	Filter    func(pipeline) bool
}

func (ps *pipelineSelect) Run() (pipeline, error) {
	var filtered []pipeline

	if ps.Filter != nil {
		for _, p := range ps.Pipelines {
			if ps.Filter(p) {
				// debugf("Filter matched %#v", p)
				filtered = append(filtered, p)
			}
		}
	} else {
		filtered = ps.Pipelines
	}

	// debugf("Filtered %d pipelines down to %d", len(ps.Pipelines), len(filtered))

	if len(filtered) == 1 {
		return filtered[0], nil
	} else if len(filtered) == 0 {
		return pipeline{}, errPipelineDoesntExist
	}

	var labels []string
	for _, p := range filtered {
		labels = append(labels, p.URL)
	}

	prompt := promptui.Select{
		Label: "Select pipeline",
		Items: labels,
	}

	offset, _, err := prompt.Run()
	if err != nil {
		return pipeline{}, err
	}

	// debugf("Selected %d %#v", offset, filtered[offset])
	return filtered[offset], nil
}

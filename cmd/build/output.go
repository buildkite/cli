package build

import buildkite "github.com/buildkite/go-buildkite/v5"

type buildOutput struct {
	buildkite.Build
	// YAML renders the jobs nested under Build.
	Jobs        []buildJob             `json:"jobs,omitempty" yaml:"-"`
	Artifacts   []buildkite.Artifact   `json:"artifacts,omitempty"`
	Annotations []buildkite.Annotation `json:"annotations,omitempty"`
}

func (build buildDetails) output(artifacts []buildkite.Artifact, annotations []buildkite.Annotation) buildOutput {
	build.Env = nil
	if build.Pipeline != nil {
		build.Pipeline.Env = nil
		build.Pipeline.Provider.WebhookURL = ""
		for i := range build.Pipeline.Steps {
			build.Pipeline.Steps[i].Env = nil
		}
	}
	build.Build.Jobs = make([]buildkite.Job, len(build.Jobs))
	for i := range build.Jobs {
		for job := &build.Jobs[i].Job; job != nil; job = job.Agent.Job {
			job.Agent.AgentToken = ""
		}
		build.Build.Jobs[i] = build.Jobs[i].Job
	}
	return buildOutput{
		Build:       build.Build,
		Jobs:        build.Jobs,
		Artifacts:   artifacts,
		Annotations: annotations,
	}
}

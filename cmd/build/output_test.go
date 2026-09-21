package build

import (
	"bytes"
	"encoding/json"
	"strings"
	"testing"

	"github.com/buildkite/cli/v3/pkg/output"
	buildkite "github.com/buildkite/go-buildkite/v5"
	"gopkg.in/yaml.v3"
)

func TestBuildOutputBrokenReasons(t *testing.T) {
	const response = `{
		"number": 42,
		"jobs": [
			{"id":"conditional","state":"broken","broken_reason":"conditional_failed"},
			{"id":"changed","state":"broken","broken_reason":"if_changed matched no unexcluded changed files in this build"},
			{"id":"parallel","state":"broken","broken_reason":"parallelism_zero"},
			{"id":"passed","state":"passed","broken_reason":null},
			{"id":"scheduled","state":"scheduled"}
		]
	}`
	var build buildDetails
	if err := json.Unmarshal([]byte(response), &build); err != nil {
		t.Fatal(err)
	}
	result := build.output([]buildkite.Artifact{{ID: "artifact"}}, []buildkite.Annotation{{ID: "annotation"}})
	var buf bytes.Buffer
	if err := output.Write(&buf, result, output.FormatJSON); err != nil {
		t.Fatal(err)
	}
	var got struct {
		Number      int
		Jobs        []map[string]any
		Artifacts   []buildkite.Artifact
		Annotations []buildkite.Annotation
	}
	if err := json.Unmarshal(buf.Bytes(), &got); err != nil {
		t.Fatal(err)
	}
	if got.Number != 42 || len(got.Jobs) != 5 || len(result.Build.Jobs) != 5 {
		t.Fatalf("build number or jobs were lost: number=%d, JSON jobs=%d, text jobs=%d", got.Number, len(got.Jobs), len(result.Build.Jobs))
	}
	for i, reason := range []string{
		"conditional_failed",
		"if_changed matched no unexcluded changed files in this build",
		"parallelism_zero",
	} {
		if got.Jobs[i]["broken_reason"] != reason || got.Jobs[i]["state"] != "broken" {
			t.Errorf("job %d = %v, want broken_reason %q and state broken", i, got.Jobs[i], reason)
		}
	}
	for _, job := range got.Jobs[3:] {
		if _, ok := job["broken_reason"]; ok {
			t.Errorf("job %v has an absent broken reason", job)
		}
	}
	if len(got.Artifacts) != 1 || got.Artifacts[0].ID != "artifact" || len(got.Annotations) != 1 || got.Annotations[0].ID != "annotation" {
		t.Fatal("artifacts or annotations were lost")
	}
}

func TestBuildOutputOmitsEnvironmentAndCredentials(t *testing.T) {
	const response = `{
		"env":{"KEY":"sensitive-build-env"},
		"pipeline":{
			"slug":"pipeline",
			"provider":{"id":"custom","webhook_url":"sensitive-webhook-secret"},
			"env":{"KEY":"sensitive-pipeline-env"},
			"steps":[{"label":"step","env":{"KEY":"sensitive-step-env"}}]
		},
		"jobs":[{"id":"job","agent":{
			"name":"agent","access_token":"sensitive-agent-token",
			"job":{"id":"nested","agent":{"access_token":"sensitive-nested-token"}}
		}}],
		"unknown_authentication":"sensitive-unknown"
	}`
	for _, format := range []output.Format{output.FormatJSON, output.FormatYAML} {
		t.Run(string(format), func(t *testing.T) {
			var build buildDetails
			if err := json.Unmarshal([]byte(response), &build); err != nil {
				t.Fatal(err)
			}
			var buf bytes.Buffer
			if err := output.Write(&buf, build.output(nil, nil), format); err != nil {
				t.Fatal(err)
			}
			if strings.Contains(buf.String(), "sensitive-") {
				t.Fatal("output contains environment or authentication values")
			}
			if format == output.FormatYAML {
				var got struct {
					Build struct {
						Jobs []struct{ ID string }
					}
				}
				if err := yaml.Unmarshal(buf.Bytes(), &got); err != nil {
					t.Fatal(err)
				}
				if len(got.Build.Jobs) != 1 || got.Build.Jobs[0].ID != "job" {
					t.Fatal("YAML jobs are missing from build.jobs")
				}
			}
		})
	}
}

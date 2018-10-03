package local

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestPipelineParsing(t *testing.T) {
	for _, tc := range []struct {
		JSON     string
		Expected Pipeline
	}{
		{`{"steps":[{"command": "echo hello world"}]}`, Pipeline{
			Steps: []Step{
				{
					Command: &CommandStep{
						Commands: []string{"echo hello world"},
					},
				},
			},
		}},
		{`{"steps":[{"commands": ["echo hello world","pwd"]}]}`, Pipeline{
			Steps: []Step{
				{
					Command: &CommandStep{
						Commands: []string{"echo hello world", "pwd"},
					},
				},
			},
		}},
		{`{"steps":[{"wait":""},{"label": "llamas"}]}`, Pipeline{
			Steps: []Step{
				{
					Wait: &WaitStep{},
				},
				{
					Command: &CommandStep{
						Label: "llamas",
					},
				},
			},
		}},
	} {
		t.Run("", func(t *testing.T) {
			var pipeline Pipeline

			if err := json.Unmarshal([]byte(tc.JSON), &pipeline); err != nil {
				t.Fatal(err)
			}

			if !reflect.DeepEqual(tc.Expected, pipeline) {
				t.Fatalf("Expected %+v, got %+v", tc.Expected, pipeline)
			}
		})
	}
}

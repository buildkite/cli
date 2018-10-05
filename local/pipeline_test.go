package local

import (
	"encoding/json"
	"reflect"
	"testing"
)

func TestPipelineParsing(t *testing.T) {
	for _, tc := range []struct {
		JSON     string
		Expected pipeline
	}{
		{`{"steps":[{"command": "echo hello world"}]}`, pipeline{
			Steps: []step{
				{
					Command: &commandStep{
						Commands: []string{"echo hello world"},
					},
				},
			},
		}},
		{`{"steps":[{"commands": ["echo hello world","pwd"]}]}`, pipeline{
			Steps: []step{
				{
					Command: &commandStep{
						Commands: []string{"echo hello world", "pwd"},
					},
				},
			},
		}},
		{`{"steps":[{"wait":""},{"label": "llamas"}]}`, pipeline{
			Steps: []step{
				{
					Wait: &waitStep{},
				},
				{
					Command: &commandStep{
						Label: "llamas",
					},
				},
			},
		}},
	} {
		t.Run("", func(t *testing.T) {
			var pipeline pipeline

			if err := json.Unmarshal([]byte(tc.JSON), &pipeline); err != nil {
				t.Fatal(err)
			}

			if !reflect.DeepEqual(tc.Expected, pipeline) {
				t.Fatalf("Expected %+v, got %+v", tc.Expected, pipeline)
			}
		})
	}
}

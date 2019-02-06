package local

import (
	"encoding/json"
	"testing"

	"github.com/go-test/deep"
)

func TestParsingStringable(t *testing.T) {
	for _, tc := range []struct {
		JSON     string
		Expected string
	}{
		{`true`, `true`},
		{`false`, `false`},
		{`"true"`, `true`},
		{`"true"`, `true`},
		{`1`, `1`},
		{`"1"`, `1`},
		{`1.23`, `1.23`},
	} {
		t.Run("", func(t *testing.T) {
			var target stringable

			if err := json.Unmarshal([]byte(tc.JSON), &target); err != nil {
				t.Fatalf("failed to parse json: %v", err)
			}

			if diff := deep.Equal(tc.Expected, string(target)); diff != nil {
				t.Error(diff)
			}
		})
	}
}

func TestParsingEnvMapOrSlice(t *testing.T) {
	for _, tc := range []struct {
		JSON     string
		Expected []string
	}{
		{
			`{ "FOO": false, "BAR": "foo", "BAZ": 12 }`,
			[]string{"BAR=foo", "BAZ=12", "FOO=false"},
		},
		{
			`[ "BAR=foo", "BAZ=12", "FOO=false" ]`,
			[]string{"BAR=foo", "BAZ=12", "FOO=false"},
		},
	} {
		t.Run("", func(t *testing.T) {
			var target envMapOrSlice

			if err := json.Unmarshal([]byte(tc.JSON), &target); err != nil {
				t.Fatalf("failed to parse json: %v", err)
			}

			if diff := deep.Equal(tc.Expected, []string(target)); diff != nil {
				t.Error(diff)
			}
		})
	}
}

func TestStepParsingCommandSteps(t *testing.T) {
	for _, tc := range []struct {
		JSON     string
		Expected step
	}{
		{
			`{
				"label": "testing with env as a map",
				"command": "echo hi $$FOO",
				"env": {
					"FOO": false,
					"BAR": "foo",
					"BAZ": 12
				}
			}`,
			step{
				Command: &commandStep{
					Label:    `testing with env as a map`,
					Commands: []string{`echo hi $$FOO`},
					Env: envMapOrSlice{
						"BAR=foo",
						"BAZ=12",
						"FOO=false",
					},
				},
			},
		},
		{
			`{
				"label": "testing with boolean commands",
				"command": [ "echo llamas", false ]
			}`,
			step{
				Command: &commandStep{
					Label:    `testing with boolean commands`,
					Commands: []string{`echo llamas`, `false`},
				},
			},
		},
	} {
		t.Run("", func(t *testing.T) {
			var step step

			if err := json.Unmarshal([]byte(tc.JSON), &step); err != nil {
				t.Fatalf("failed to parse json: %v", err)
			}

			if diff := deep.Equal(tc.Expected, step); diff != nil {
				t.Error(diff)
			}
		})
	}
}

func TestPipelineParsing(t *testing.T) {
	for _, tc := range []struct {
		JSON     string
		Expected pipeline
	}{
		{
			`{
				"steps": [
					{
						"command": "echo hello world"
					}
				]
			}`,
			pipeline{
				Steps: []step{
					{
						Command: &commandStep{
							Commands: []string{"echo hello world"},
						},
					},
				},
			},
		},
		{
			`{
				"steps":[
					{
						"commands": [ "echo hello world", "pwd" ]
					}
				]
			}`,
			pipeline{
				Steps: []step{
					{
						Command: &commandStep{
							Commands: []string{"echo hello world", "pwd"},
						},
					},
				},
			},
		},
		{
			`{
				"steps": [
					{"wait": ""},
					{"label": "llamas"}
				]
			}`,
			pipeline{
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
			},
		},
		{
			`{
				"steps": [
					{
						"commands": [
							"echo hello world",
							"pwd"
						],
						"plugins": {
							"blah-blah/blah#v0.0.1": null,
							"blah-blah/another#v0.0.1": {
								"my_config":"totes"
							}
						}
					}
				]
			}`,
			pipeline{
				Steps: []step{
					{
						Command: &commandStep{
							Commands: []string{"echo hello world", "pwd"},
							Plugins: []Plugin{
								{Name: "blah-blah/blah#v0.0.1"},
								{Name: "blah-blah/another#v0.0.1", Params: map[string]interface{}{
									"my_config": "totes",
								}},
							},
						},
					},
				},
			},
		},
		{
			`{
				"steps": [
					{
						"plugins": [
							"blah-blah/blah#v0.0.1"
						]
					}
				]
			}`,
			pipeline{
				Steps: []step{
					{
						Command: &commandStep{
							Plugins: []Plugin{
								{Name: "blah-blah/blah#v0.0.1"},
							},
						},
					},
				},
			},
		},
		{
			`{
				"steps": [
					{"wait": ""},
					{"label": "llamas"}
				]
			}`,
			pipeline{
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
			},
		},
		{
			`{
				"env": {
					"FOO": false,
					"BAR": "foo",
					"BAZ": 12
				},
				"steps": []
			}`,
			pipeline{
				Env: envMapOrSlice{
					"BAR=foo",
					"BAZ=12",
					"FOO=false",
				},
				Steps: []step{},
			},
		},
	} {
		t.Run("", func(t *testing.T) {
			var pipeline pipeline

			if err := json.Unmarshal([]byte(tc.JSON), &pipeline); err != nil {
				t.Fatal(err)
			}

			if diff := deep.Equal(tc.Expected, pipeline); diff != nil {
				t.Error(diff)
			}
		})
	}
}

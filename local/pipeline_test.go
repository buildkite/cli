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
		{
			`{
				"command": "echo foo",
				"branches": "!foo"
			}`,
			step{
				Command: &commandStep{
					Commands: []string{`echo foo`},
				},
				Branches: []string{`!foo`},
			},
		},
		{
			`{
				"command": "echo foo",
				"branches": "master stable/*"
			}`,
			step{
				Command: &commandStep{
					Commands: []string{`echo foo`},
				},
				Branches: []string{`master`, `stable/*`},
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

func TestMatchingBranches(t *testing.T) {
	for _, tc := range []struct {
		Pattern string
		Branch  string
	}{
		//  empty branch patterns match all
		{"", "foo"},
		{"", ""},
		{"", "       "},

		//  single name branch matches
		{"foo bar", "foo"},
		{"foo             bar", "bar"},

		//  with whitespace
		{"    master   ", "master"},
		{"master   ", "master"},
		{"   master", "master"},

		//  ones with a slash
		{"feature/authentication", "feature/authentication"},

		//  not-checking
		{"!foo", "master"},
		{"!release/production !release/test", "master"},

		//  prefix wildcards
		{"*-do-the-thing", "can-you-do-the-thing"},
		{"!*-do-the-thing", "can-you-do-the-thing-please"},

		//  wildcards
		{"release/*", "release/foo"},
		{"release/*", "release/bar/bing"},
		{"release-*", "release-thingy"},
		{"release-* release/*", "release-thingy"},
		{"release-* release/*", "release/thingy"},
		{"this-*-thing-is-the-*", "this-ruby-thing-is-the-worst"},
		{"this-*-thing-is-the-*", "this-regex-thing-is-the-best"},
		{"this-*-thing-is-the-*", "this-*-thing-is-the-*"},
		{"this-*-thing-is-the-*", "this--thing-is-the-best-"},
	} {
		t.Run("", func(t *testing.T) {
			branches, err := ParseBranchPattern(tc.Pattern)
			if err != nil {
				t.Fatal(err)
			}

			s := step{
				Branches: branches,
			}

			if !s.MatchBranch(tc.Branch) {
				t.Errorf("Expected pattern %q to match branch %q", tc.Pattern, tc.Branch)
			}
		})
	}
}

func TestNotMatchingBranches(t *testing.T) {
	for _, tc := range []struct {
		Pattern string
		Branch  string
	}{
		// branch names
		{"foo         bar", "bang"},

		// not-matchers
		{"!foo bar", "foo"},
		{"!release/*", "release/foo"},
		{"!release/*", "release/bar"},
		{"!refs/tags/*", "refs/tags/production"},
		{"!release/production !release/test", "release/production"},
		{"!release/production !release/test", "release/test"},

		// ones with a slash
		{"feature/authentication", "update/deployment"},

		// wildcards
		{"release-*", "release/thingy"},
		{"release-*", "master"},
		{"*-do-the-thing", "this-is-not-the-thing"},
	} {
		t.Run("", func(t *testing.T) {
			branches, err := ParseBranchPattern(tc.Pattern)
			if err != nil {
				t.Fatal(err)
			}

			s := step{
				Branches: branches,
			}

			if s.MatchBranch(tc.Branch) {
				t.Errorf("Expected pattern %q to NOT match branch %q", tc.Pattern, tc.Branch)
			}
		})
	}
}

package browse

import "testing"

func TestPipelineURL(t *testing.T) {
	got := pipelineURL("my-org", "my-pipeline")
	want := "https://buildkite.com/my-org/my-pipeline"
	if got != want {
		t.Errorf("pipelineURL = %q, want %q", got, want)
	}
}

func TestBuildURL(t *testing.T) {
	got := buildURL("my-org", "my-pipeline", 420)
	want := "https://buildkite.com/my-org/my-pipeline/builds/420"
	if got != want {
		t.Errorf("buildURL = %q, want %q", got, want)
	}
}

func TestSettingsURL(t *testing.T) {
	got := settingsURL("my-org", "my-pipeline")
	want := "https://buildkite.com/my-org/my-pipeline/settings"
	if got != want {
		t.Errorf("settingsURL = %q, want %q", got, want)
	}
}

func TestPipelineBranchURL(t *testing.T) {
	cases := []struct {
		name   string
		branch string
		want   string
	}{
		{
			name:   "simple branch",
			branch: "main",
			want:   "https://buildkite.com/my-org/my-pipeline/builds?branch=main",
		},
		{
			name:   "branch with query delimiters",
			branch: "this&that&query=hello+world",
			want:   "https://buildkite.com/my-org/my-pipeline/builds?branch=this%26that%26query%3Dhello%2Bworld",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := pipelineBranchURL("my-org", "my-pipeline", tc.branch)
			if got != tc.want {
				t.Errorf("pipelineBranchURL = %q, want %q", got, tc.want)
			}
		})
	}
}

// TestBrowseCmd_Conflicts exercises the mutually-exclusive flag combinations.
// Run returns an error before touching the factory, so no config/auth is
// required for these cases.
func TestBrowseCmd_Conflicts(t *testing.T) {
	cases := []struct {
		name string
		cmd  BrowseCmd
	}{
		{"settings with build", BrowseCmd{Settings: true, Build: "420"}},
		{"branch with build", BrowseCmd{Branch: "main", Build: "420"}},
		{"all-branches with build", BrowseCmd{AllBranches: true, Build: "420"}},
		{"branch with all-branches", BrowseCmd{Branch: "main", AllBranches: true}},
		{"branch with settings", BrowseCmd{Branch: "main", Settings: true}},
		{"all-branches with settings", BrowseCmd{AllBranches: true, Settings: true}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			if err := tc.cmd.Run(nil, nil); err == nil {
				t.Fatal("expected an error for conflicting flags, got nil")
			}
		})
	}
}

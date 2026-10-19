package build

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"reflect"
	"slices"
	"strconv"
	"strings"
	"testing"
	"time"

	"github.com/alecthomas/kong"
	"github.com/buildkite/cli/v3/internal/cli"
	"github.com/buildkite/cli/v3/pkg/keyring"
	buildkite "github.com/buildkite/go-buildkite/v5"
	"gopkg.in/yaml.v3"
)

func comparisonTime(seconds int) *buildkite.Timestamp {
	return &buildkite.Timestamp{Time: time.Date(2026, 10, 1, 0, 0, seconds, 0, time.UTC)}
}

type comparisonFixture struct {
	targetJobs, baselineJobs []buildkite.Job
	candidates               []buildkite.Build
	noBaseline, missingTime  bool
	loopJobs, loopBuilds     bool
	failAt                   string
	pipelines                []string
	storedToken              bool
	requests                 []string
	jobPages                 int
	settings                 bool
	globals                  *cli.Globals
	delay                    time.Duration
}

func runComparison(t *testing.T, fixture *comparisonFixture, args ...string) (buildComparison, string, error) {
	t.Helper()
	const prefix = "/v2/organizations/acme/pipelines/widgets/builds"
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		fixture.requests = append(fixture.requests, r.URL.RequestURI())
		if r.Method != http.MethodGet || r.Header.Get("Authorization") != "Bearer test-token" {
			t.Errorf("unexpected method/auth for %s", r.URL)
		}
		if fixture.failAt != "" && strings.Contains(r.URL.RequestURI(), fixture.failAt) {
			w.WriteHeader(http.StatusForbidden)
			_, _ = w.Write([]byte(`{"message":"Access denied"}`))
			return
		}
		switch r.URL.Path {
		case prefix + "/42", prefix + "/39":
			time.Sleep(fixture.delay)
			if r.URL.Query().Get("exclude_jobs") != "true" || r.URL.Query().Get("exclude_pipeline") != "true" {
				t.Errorf("heavyweight metadata lookup: %s", r.URL)
			}
			b := buildkite.Build{Number: 42, Branch: "feature", State: "failed", CreatedAt: comparisonTime(40)}
			if strings.HasSuffix(r.URL.Path, "/39") {
				// Explicit baselines are permitted on other branches and in failed state.
				b.Number, b.Branch, b.CreatedAt = 39, "main", comparisonTime(10)
			}
			if fixture.missingTime {
				b.CreatedAt = nil
			}
			b.ID, b.WebURL, b.Commit = fmt.Sprint(b.Number), fmt.Sprintf("https://buildkite.com/acme/widgets/builds/%d", b.Number), "commit"
			_ = json.NewEncoder(w).Encode(b)
		case prefix:
			q := r.URL.Query()
			if q.Get("branch[]") != "feature" || q.Get("state[]") != "passed" || q.Get("created_to") == "" || q.Get("per_page") != "100" {
				t.Errorf("incorrect baseline filters: %s", r.URL)
			}
			if fixture.loopBuilds || q.Get("page") == "1" {
				w.Header().Set("Link", fmt.Sprintf(`<https://api.buildkite.com%s?page=7>; rel="next"`, prefix))
				_ = json.NewEncoder(w).Encode(fixture.candidates)
			} else if fixture.noBaseline {
				_, _ = w.Write([]byte(`[]`))
			} else {
				_ = json.NewEncoder(w).Encode([]buildkite.Build{{Number: 39, Branch: "feature", State: "passed", CreatedAt: comparisonTime(10)}})
			}
		case prefix + "/42/jobs", prefix + "/39/jobs":
			q := r.URL.Query()
			if q.Get("include_retried_jobs") != "false" || q.Get("per_page") != "100" || q.Has("state[]") || q.Has("step_key") {
				t.Errorf("incorrect inventory options: %s", r.URL)
			}
			jobs := fixture.baselineJobs
			if strings.HasSuffix(r.URL.Path, "/42/jobs") {
				jobs = fixture.targetJobs
			}
			// A short page with a cursor must not terminate inventory loading.
			page, _ := strconv.Atoi(q.Get("after"))
			page++
			pages := max(2, fixture.jobPages)
			if page < pages || fixture.loopJobs {
				_ = json.NewEncoder(w).Encode(buildkite.JobsList{
					Items: []buildkite.Job{{ID: "superseded", StepKey: "test", State: "failed", Retried: true, RetriesCount: 9}},
					Links: buildkite.JobsListLinks{Next: buildkite.JobsListLink(fmt.Sprintf("https://api.buildkite.com%s?after=%d", r.URL.Path, page))},
				})
			} else {
				if jobs == nil {
					jobs = []buildkite.Job{}
				}
				_ = json.NewEncoder(w).Encode(buildkite.JobsList{Items: jobs})
			}
		default:
			t.Errorf("unexpected URL: %s", r.URL)
			http.NotFound(w, r)
		}
	}))
	defer server.Close()
	t.Chdir(t.TempDir())
	t.Setenv("HOME", t.TempDir())
	t.Setenv("XDG_CONFIG_HOME", "")
	t.Setenv(keyring.CredentialStoreEnv, keyring.StoreKeyring)
	t.Setenv(keyring.CredentialStorePathEnv, filepath.Join(t.TempDir(), "credentials.json"))
	t.Setenv("BUILDKITE_REST_API_ENDPOINT", server.URL)
	t.Setenv("BUILDKITE_ORGANIZATION_SLUG", "other-org")
	t.Setenv("BUILDKITE_OUTPUT_FORMAT", "json")
	t.Setenv("BUILDKITE_API_TOKEN", "test-token")
	keyring.MockForTesting()
	t.Cleanup(keyring.ResetForTesting)
	if fixture.storedToken {
		t.Setenv("BUILDKITE_API_TOKEN", "")
		if err := keyring.New().Set("acme", "test-token"); err != nil {
			t.Fatal(err)
		}
	}
	if fixture.pipelines != nil {
		t.Setenv("BUILDKITE_ORGANIZATION_SLUG", "acme")
		data, err := yaml.Marshal(map[string]any{"selected_org": "acme", "pipelines": fixture.pipelines})
		if err != nil {
			t.Fatal(err)
		}
		if err := os.WriteFile(".bk.yaml", data, 0o600); err != nil {
			t.Fatal(err)
		}
	}
	if fixture.settings {
		t.Setenv("XDG_CONFIG_HOME", t.TempDir())
		if err := os.WriteFile(filepath.Join(os.Getenv("XDG_CONFIG_HOME"), "bk.yaml"), []byte("no_input: true\nquiet: true\n"), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	var cmd CompareCmd
	var stdout, stderr bytes.Buffer
	parser := kong.Must(&cmd, kong.Vars{"output_default_format": ""}, kong.Writers(&stdout, &stderr))
	ctx, err := parser.Parse(args)
	if err == nil {
		globals := cli.Globals{Quiet: true, NoPager: true}
		if fixture.globals != nil {
			globals = *fixture.globals
		}
		err = cmd.Run(ctx, globals)
	}
	var result buildComparison
	if err == nil {
		if slices.Contains(args, "--yaml") {
			err = yaml.Unmarshal(stdout.Bytes(), &result)
		} else {
			err = json.Unmarshal(stdout.Bytes(), &result)
		}
	}
	return result, stdout.String(), err
}

func TestCompareSelectorsAndErrors(t *testing.T) {
	for _, tt := range []struct {
		name string
		args []string
		f    comparisonFixture
		want string
	}{
		{name: "qualified explicit", args: []string{"acme/widgets/42", "--baseline", "39", "--json"}},
		{name: "number pipeline YAML", args: []string{"42", "-p", "acme/widgets", "--baseline", "39", "--yaml"}},
		{name: "URL organization credentials", args: []string{"https://buildkite.com/acme/widgets/builds/42", "--baseline", "39"}, f: comparisonFixture{storedToken: true}},
		{name: "inferred single pipeline", args: []string{"42", "--baseline", "39"}, f: comparisonFixture{pipelines: []string{"widgets"}}},
		{name: "ambiguous JSON", args: []string{"42", "--json"}, f: comparisonFixture{pipelines: []string{"widgets", "other"}}, want: "use --pipeline"},
		{name: "ambiguous YAML", args: []string{"42", "--yaml"}, f: comparisonFixture{pipelines: []string{"widgets", "other"}}, want: "use --pipeline"},
		{name: "same build", args: []string{"acme/widgets/42", "--baseline", "42"}, want: "different"},
		{name: "zero baseline", args: []string{"acme/widgets/42", "--baseline", "0"}, want: "positive"},
		{name: "negative baseline", args: []string{"acme/widgets/42", "--baseline=-3"}, want: "positive"},
		{name: "zero target", args: []string{"acme/widgets/0"}, want: "positive"},
		{name: "invalid target", args: []string{"not-a-build"}, want: "unable to parse"},
		{name: "no baseline", f: comparisonFixture{noBaseline: true}, want: "provide --baseline"},
		{name: "missing creation time", f: comparisonFixture{missingTime: true}, want: "provide --baseline"},
		{name: "metadata denied", f: comparisonFixture{failAt: "/42?"}, want: "requires read_builds"},
		{name: "baseline search denied", f: comparisonFixture{failAt: "/builds?"}, want: "requires read_builds"},
		{name: "baseline denied", args: []string{"acme/widgets/42", "--baseline", "39"}, f: comparisonFixture{failAt: "/39?"}, want: "requires read_builds"},
		{name: "later inventory denied", f: comparisonFixture{failAt: "/42/jobs?after"}, want: "requires read_builds"},
		{name: "baseline inventory denied", f: comparisonFixture{failAt: "/39/jobs"}, want: "requires read_builds"},
		{name: "inventory overflow", f: comparisonFixture{loopJobs: true}, want: "no partial comparison"},
		{name: "baseline bound", f: comparisonFixture{loopBuilds: true}, want: "500 candidates"},
	} {
		t.Run(tt.name, func(t *testing.T) {
			if len(tt.args) == 0 {
				tt.args = []string{"acme/widgets/42"}
			}
			r, stdout, err := runComparison(t, &tt.f, tt.args...)
			if tt.want != "" {
				if err == nil || !strings.Contains(err.Error(), tt.want) || stdout != "" {
					t.Fatalf("error=%v output=%q; want %q and no partial output", err, stdout, tt.want)
				}
				if tt.f.loopJobs && len(tt.f.requests) != 14 { // target, two baseline pages, baseline, ten job pages
					t.Fatalf("unbounded inventory: %v", tt.f.requests)
				}
				if tt.f.loopBuilds && len(tt.f.requests) != 6 {
					t.Fatalf("unbounded baseline: %v", tt.f.requests)
				}
				return
			}
			if err != nil {
				t.Fatal(err)
			}
			if r.Organization != "acme" || r.Pipeline != "widgets" || r.Target.Number != 42 || r.Baseline.Number != 39 || r.BaselineSelection != "explicit" || r.Steps == nil || len(r.Steps) != 0 || len(r.ChangeCounts) != 0 {
				t.Fatalf("unexpected empty comparison: %+v", r)
			}
			if !strings.Contains(strings.Join(r.Warnings, " "), "different branch") || !strings.Contains(strings.Join(r.Warnings, " "), "unfinished") {
				t.Fatalf("missing snapshot/branch warnings: %v", r.Warnings)
			}
		})
	}
}

func TestCompareFinalAttemptOutcomesAndTiming(t *testing.T) {
	base := []buildkite.Job{
		{ID: "b-new", StepKey: "test", Name: "old name", Type: "script", State: "passed", ScheduledAt: comparisonTime(1), StartedAt: comparisonTime(3), FinishedAt: comparisonTime(8), Step: &buildkite.StepInfo{ID: "baseline-step"}},
		{StepKey: "recovered", State: "timed_out"},
		{StepKey: "still", State: "failed", SoftFailed: true},
		{StepKey: "soft", State: "passed"},
		{StepKey: "hard", State: "failed", SoftFailed: true},
		{StepKey: "retry", State: "passed", StartedAt: comparisonTime(5), FinishedAt: comparisonTime(5)},
		{StepKey: "canceled", State: "passed"},
		{StepKey: "removed", State: "passed"},
		{Name: "unique", GroupKey: "group", Type: "trigger", State: "passed"},
	}
	target := []buildkite.Job{
		{ID: "t-new", StepKey: "test", Name: "renamed", Type: "script", State: "failed", RetriesCount: 2, ScheduledAt: comparisonTime(10), StartedAt: comparisonTime(13), FinishedAt: comparisonTime(24), ExitStatus: new(7), WebURL: "https://buildkite.com/job/t-new", Step: &buildkite.StepInfo{ID: "target-step"}},
		{StepKey: "recovered", State: "passed"},
		{StepKey: "still", State: "expired", SoftFailed: true, RetriesCount: 4},
		{StepKey: "soft", State: "failed", SoftFailed: true},
		{StepKey: "hard", State: "failed"},
		{StepKey: "retry", State: "passed", RetriesCount: 3, StartedAt: comparisonTime(8), FinishedAt: comparisonTime(6)},
		{StepKey: "canceled", State: "canceled"},
		{StepKey: "added", State: "failed"},
		{Name: "unique", GroupKey: "group", Type: "trigger", State: "passed"},
	}
	f := comparisonFixture{targetJobs: target, baselineJobs: base, candidates: []buildkite.Build{
		{Number: 42, State: "passed", Branch: "feature", CreatedAt: comparisonTime(40)},
		{Number: 43, State: "passed", Branch: "feature", CreatedAt: comparisonTime(1)},
		{Number: 41, State: "passed", Branch: "wrong", CreatedAt: comparisonTime(20)},
		{Number: 40, State: "failed", Branch: "feature", CreatedAt: comparisonTime(20)},
		{Number: 38, State: "passed", Branch: "feature", CreatedAt: comparisonTime(40)},
		{Number: 37, State: "passed", Branch: "feature"},
	}}
	r, _, err := runComparison(t, &f, "acme/widgets/42", "--json")
	if err != nil {
		t.Fatal(err)
	}
	want := map[string]int{"newly_failing": 1, "recovered": 1, "still_failing": 1, "state_changed": 3, "retries_changed": 1, "added": 1, "removed": 1, "unchanged": 1}
	if !reflect.DeepEqual(r.ChangeCounts, want) || r.TargetJobs != 9 || r.BaselineJobs != 9 || r.CandidatesScanned != 7 || r.Baseline.Number != 39 || r.BaselineSelection != "most_recently_created_earlier_passed_build_on_same_branch" {
		t.Fatalf("incorrect final-attempt comparison: %+v", r)
	}
	first := r.Steps[0]
	if first.Target.WebURL != "https://buildkite.com/job/t-new" || first.Baseline.WebURL != "https://buildkite.com/acme/widgets/builds/39/list?sid=baseline-step&tab=output" {
		t.Fatalf("lost job URL or wrong build in fallback: target=%s baseline=%s", first.Target.WebURL, first.Baseline.WebURL)
	}
	if first.Change != "newly_failing" || first.MatchMethod != "step_key" || first.Target.ID != "t-new" || first.Baseline.ID != "b-new" || first.Target.RetriesCount != 2 || *first.Target.ExitStatus != 7 || first.ExecutionDeltaSeconds == nil || *first.ExecutionDeltaSeconds != 6 || *first.Target.ExecutionSeconds != 11 || *first.Target.SchedulingSeconds != 3 {
		t.Fatalf("incorrect pairing/timing: %+v", first)
	}
	for _, step := range r.Steps {
		if step.StepKey == "retry" && (step.Target.ExecutionSeconds != nil || step.ExecutionDeltaSeconds != nil || step.Baseline.ExecutionSeconds == nil || *step.Baseline.ExecutionSeconds != 0) {
			t.Fatalf("invalid/zero durations: %+v", step)
		}
	}
	if !strings.Contains(strings.Join(r.Warnings, " "), "heuristic") || !strings.Contains(strings.Join(r.Warnings, " "), "does not prove") {
		t.Fatalf("missing uncertainty caveats: %v", r.Warnings)
	}
	for _, text := range []string{"build 42 vs 39", "newly_failing", "+6.000s", "name_fallback", "https://buildkite.com/job/t-new"} {
		if !strings.Contains(r.TextOutput(), text) {
			t.Errorf("text missing %q: %s", text, r.TextOutput())
		}
	}
}

func TestCompareCompleteInventoryBoundary(t *testing.T) {
	f := comparisonFixture{jobPages: 10, baselineJobs: []buildkite.Job{{ID: "orphan", StepKey: "removed", State: "passed"}}}
	for i := range 100 {
		f.targetJobs = append(f.targetJobs, buildkite.Job{ID: fmt.Sprint(i), StepKey: fmt.Sprintf("test-%d", i), State: "passed"})
	}
	r, _, err := runComparison(t, &f, "acme/widgets/42", "--baseline", "39")
	if err != nil {
		t.Fatal(err)
	}
	if len(r.Steps) != 101 || r.TargetJobs != 100 || r.BaselineJobs != 1 || r.ChangeCounts["added"] != 100 || r.ChangeCounts["removed"] != 1 || r.Steps[100].Baseline.ID != "orphan" {
		t.Fatalf("inventory or output truncated: %+v", r)
	}
}

func TestCompareIdentityCoordinatesAndAmbiguity(t *testing.T) {
	for _, tt := range []struct {
		name string
		t, b []buildkite.Job
		want map[string]int
	}{
		{name: "matrix and shards asymmetric", t: []buildkite.Job{
			{StepKey: "test", Type: "script", Matrix: map[string]any{"os": "linux"}, ParallelGroupIndex: new(0), ParallelGroupTotal: new(2), State: "failed"},
			{StepKey: "test", Type: "script", Matrix: map[string]any{"os": "linux"}, ParallelGroupIndex: new(1), ParallelGroupTotal: new(2), State: "passed"},
		}, b: []buildkite.Job{
			{StepKey: "test", Type: "script", Matrix: map[string]any{"os": "linux"}, ParallelGroupIndex: new(1), ParallelGroupTotal: new(2), State: "failed"},
			{StepKey: "test", Type: "script", Matrix: map[string]any{"os": "mac"}, ParallelGroupIndex: new(0), ParallelGroupTotal: new(2), State: "passed"},
		}, want: map[string]int{"recovered": 1, "added": 1, "removed": 1}},
		{name: "parallel total changes identity", t: []buildkite.Job{{StepKey: "test", ParallelGroupIndex: new(0), ParallelGroupTotal: new(3)}}, b: []buildkite.Job{{StepKey: "test", ParallelGroupIndex: new(0), ParallelGroupTotal: new(2)}}, want: map[string]int{"added": 1, "removed": 1}},
		{name: "absent index is not zero", t: []buildkite.Job{{StepKey: "test", ParallelGroupIndex: new(0)}}, b: []buildkite.Job{{StepKey: "test"}}, want: map[string]int{"added": 1, "removed": 1}},
		{name: "type changes identity", t: []buildkite.Job{{StepKey: "test", Type: "script"}}, b: []buildkite.Job{{StepKey: "test", Type: "trigger"}}, want: map[string]int{"added": 1, "removed": 1}},
		{name: "key does not match name", t: []buildkite.Job{{StepKey: "test", Name: "test"}}, b: []buildkite.Job{{Name: "test"}}, want: map[string]int{"added": 1, "removed": 1}},
		{name: "fallback respects group and exact whitespace", t: []buildkite.Job{{Name: "test", GroupKey: "a"}, {Name: " test", GroupKey: "b"}}, b: []buildkite.Job{{Name: "test", GroupKey: "b"}, {Name: "test ", GroupKey: "b"}}, want: map[string]int{"added": 2, "removed": 2}},
		{name: "key ignores rename and group move", t: []buildkite.Job{{StepKey: "test", Name: "new", GroupKey: "a", State: "passed"}}, b: []buildkite.Job{{StepKey: "test", Name: "old", GroupKey: "b", State: "passed"}}, want: map[string]int{"unchanged": 1}},
		{name: "target duplicates leave both sides unmatched", t: []buildkite.Job{{StepKey: "test"}, {StepKey: "test"}}, b: []buildkite.Job{{StepKey: "test"}}, want: map[string]int{"unmatched": 3}},
		{name: "baseline duplicate fallback leaves both unmatched", t: []buildkite.Job{{Name: "test"}}, b: []buildkite.Job{{Name: "test"}, {Name: "test"}}, want: map[string]int{"unmatched": 3}},
		{name: "blank names cannot match", t: []buildkite.Job{{Name: " "}}, b: []buildkite.Job{{Name: " "}}, want: map[string]int{"unmatched": 2}},
	} {
		t.Run(tt.name, func(t *testing.T) {
			f := comparisonFixture{targetJobs: tt.t, baselineJobs: tt.b}
			r, _, err := runComparison(t, &f, "acme/widgets/42", "--baseline", "39")
			if err != nil {
				t.Fatal(err)
			}
			if !reflect.DeepEqual(r.ChangeCounts, tt.want) {
				t.Fatalf("counts=%v want %v; rows=%+v", r.ChangeCounts, tt.want, r.Steps)
			}
			for _, step := range r.Steps {
				if step.Change == "unmatched" && (step.UnmatchedReason == "" || step.MatchMethod != "" || step.ExecutionDeltaSeconds != nil || (step.Target != nil && step.Baseline != nil)) {
					t.Fatalf("ambiguous job was paired: %+v", step)
				}
			}
		})
	}
}

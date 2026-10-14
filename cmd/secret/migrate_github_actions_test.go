package secret

import (
	"bufio"
	"bytes"
	"context"
	"crypto/sha256"
	"encoding/base64"
	"encoding/json"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"os/exec"
	"path/filepath"
	"slices"
	"strings"
	"testing"

	buildkite "github.com/buildkite/go-buildkite/v5"
	"gopkg.in/yaml.v3"
)

const (
	testMigrationCluster  = "11111111-2222-4333-8444-555555555555"
	testMigrationCommit   = "0123456789abcdef0123456789abcdef01234567"
	testMigrationPipeline = "aaaaaaaa-bbbb-4ccc-8ddd-eeeeeeeeeeee"
)

type githubResult struct {
	output []byte
	err    error
}

type githubCall struct {
	args  []string
	stdin []byte
}

type migrationGitHubMock struct {
	results []githubResult
	calls   []githubCall
}

func (m *migrationGitHubMock) Run(_ context.Context, args []string, stdin []byte) ([]byte, error) {
	m.calls = append(m.calls, githubCall{args: slices.Clone(args), stdin: bytes.Clone(stdin)})
	if len(m.results) == 0 {
		return nil, errors.New("unexpected gh command")
	}
	result := m.results[0]
	m.results = m.results[1:]
	return result.output, result.err
}

type migrationBuildkiteMock struct {
	pipelines    []buildkite.Pipeline
	secrets      [][]buildkite.ClusterSecret
	grant        migrationGrant
	grantErr     error
	grantRequest migrationGrantRequest
	grantOrg     string
	grantCluster string
}

func (m *migrationBuildkiteMock) GetPipeline(_ context.Context, _, slug string) (buildkite.Pipeline, error) {
	for _, pipeline := range m.pipelines {
		if pipeline.Slug == slug {
			return pipeline, nil
		}
	}
	return buildkite.Pipeline{}, errors.New("pipeline not found")
}

func (m *migrationBuildkiteMock) ListPipelines(_ context.Context, _, _ string, page int) ([]buildkite.Pipeline, error) {
	if page != 1 {
		return nil, nil
	}
	return slices.Clone(m.pipelines), nil
}

func (m *migrationBuildkiteMock) ListSecrets(_ context.Context, _, _ string, page int) ([]buildkite.ClusterSecret, error) {
	if page > len(m.secrets) {
		return nil, nil
	}
	return slices.Clone(m.secrets[page-1]), nil
}

func (m *migrationBuildkiteMock) CreateGrant(_ context.Context, organization, cluster string, request migrationGrantRequest) (migrationGrant, error) {
	m.grantOrg, m.grantCluster, m.grantRequest = organization, cluster, request
	return m.grant, m.grantErr
}

func TestValidateSecretsPolicyRejectsAmbiguousOrUnboundedAuthority(t *testing.T) {
	for _, test := range []struct {
		policy string
		want   string
	}{
		{"", "must not be empty"},
		{"- imaginary: value", "unknown claim"},
		{"- pipeline_id: nope", "must be a UUID"},
		{"- pipeline_id: &id " + testMigrationPipeline + "\n- pipeline_id: *id", "YAML aliases"},
		{"- pipeline_slug: !foo widgets", "custom YAML tags"},
		{"- build_branch: on", `value "on" must be quoted`},
		{"- pipeline_slug: ${{ secrets.OTHER }}", "GitHub expression syntax"},
	} {
		_, err := validateSecretsPolicy(test.policy)
		if err == nil || !strings.Contains(err.Error(), test.want) {
			t.Errorf("validateSecretsPolicy(%q) error = %v, want %q", test.policy, err, test.want)
		}
	}
}

func TestSelectMigrationSecretNamesSupportsGuidedAndNoInputFlows(t *testing.T) {
	var prompt bytes.Buffer
	names, err := selectMigrationSecretNames(
		[]string{"API_KEY", "DEPLOY_TOKEN", "OTHER"}, nil, nil, false,
		bufio.NewReader(strings.NewReader("2,1\n")), &prompt,
	)
	if err != nil || !slices.Equal(names, []string{"API_KEY", "DEPLOY_TOKEN"}) {
		t.Fatalf("guided selection = %v, %v", names, err)
	}
	if !strings.Contains(prompt.String(), "1) API_KEY") || !strings.Contains(prompt.String(), "type all") {
		t.Fatalf("prompt = %q", prompt.String())
	}

	names, err = selectMigrationSecretNames(
		[]string{"API_KEY", "DEPLOY_TOKEN", "OTHER"}, []string{"OTHER"}, []string{"*_TOKEN"}, true,
		bufio.NewReader(strings.NewReader("")), io.Discard,
	)
	if err != nil || !slices.Equal(names, []string{"DEPLOY_TOKEN", "OTHER"}) {
		t.Fatalf("no-input selection = %v, %v", names, err)
	}
	_, err = selectMigrationSecretNames([]string{"API_KEY"}, nil, nil, true, bufio.NewReader(strings.NewReader("")), io.Discard)
	if err == nil || !strings.Contains(err.Error(), "--secret or --match") {
		t.Fatalf("no-input without selector error = %v", err)
	}
}

func TestPrepareMigrationUsesNativeBuildkiteBoundaryAndStaticAllowlist(t *testing.T) {
	gh := &migrationGitHubMock{results: []githubResult{
		{output: repositoryJSON()},
		{output: []byte("OTHER\nDEPLOY_TOKEN\nAPI_KEY\n")},
	}}
	bk := &migrationBuildkiteMock{pipelines: []buildkite.Pipeline{{
		ID: testMigrationPipeline, Slug: "widgets", ClusterID: testMigrationCluster,
		Repository: "git@github.com:acme/widgets.git",
	}}}
	command := MigrateGitHubActionsPrepareCmd{SecretNames: []string{"API_KEY"}, Matches: []string{"*_TOKEN"}}
	var stdout bytes.Buffer
	migration := githubActionsMigration{
		bk: bk, gh: gh, input: bufio.NewReader(strings.NewReader("")), stdout: &stdout, stderr: io.Discard,
	}
	if err := migration.prepare(t.Context(), command, "acme", true); err != nil {
		t.Fatal(err)
	}
	workflow := stdout.String()
	for _, required := range []string{"${{ secrets.API_KEY }}", "${{ secrets.DEPLOY_TOKEN }}", "#   - pipeline_id: " + testMigrationPipeline} {
		if !strings.Contains(workflow, required) {
			t.Errorf("workflow missing %q", required)
		}
	}
	if strings.Contains(workflow, "${{ secrets.OTHER }}") || strings.Contains(workflow, "GITHUB_TOKEN") {
		t.Fatal("workflow includes authority outside the selected static allowlist")
	}
	if len(gh.calls) != 2 {
		t.Fatalf("gh calls = %d; Buildkite discovery should not invoke bk subprocesses", len(gh.calls))
	}
}

func TestGeneratedWorkflowRemainsCompatibleWithBuildkiteGHAV090(t *testing.T) {
	workflow := mustRenderWorkflow(t)
	const v090SHA256 = "9b442ce71aa40c0d06a5aa844c0423cfa296aa896fac197dd8816ab77ce5e5cd"
	if got := fmt.Sprintf("%x", sha256.Sum256(workflow)); got != v090SHA256 {
		t.Fatalf("workflow SHA-256 = %s, want buildkite-gha v0.90.0 output %s", got, v090SHA256)
	}
}

func TestGeneratedMigrationValidatesAllValuesBeforeNetworkAndDoesNotLeak(t *testing.T) {
	workflow, err := renderOIDCSecretsMigrationWorkflow(testManifest())
	if err != nil {
		t.Fatal(err)
	}
	command := exec.Command("bash", "-c", migrationWorkflowScript(t, workflow))
	command.Env = append(command.Environ(),
		"GITHUB_REF=refs/heads/main", "DEFAULT_BRANCH=main", "GRANT_ID=grant-identifier-123",
		"ACTIONS_ID_TOKEN_REQUEST_URL=http://127.0.0.1:1/should-not-run", "ACTIONS_ID_TOKEN_REQUEST_TOKEN=github-request-token",
		"MIGRATION_SECRET_000=first-secret-value", "MIGRATION_SECRET_001=   ",
	)
	output, runErr := command.CombinedOutput()
	if runErr == nil || !strings.Contains(string(output), "DEPLOY_TOKEN is missing or empty; no Buildkite secrets were created") {
		t.Fatalf("script error/output = %v/%q", runErr, output)
	}
	if strings.Contains(string(output), "first-secret-value") || strings.Contains(string(output), "github-request-token") {
		t.Fatalf("script leaked sensitive input: %q", output)
	}
}

func TestRunMigrationBindsExactCommitAndReportsDispatchNotCompletion(t *testing.T) {
	t.Chdir(t.TempDir())
	workflowPath := ".github/workflows/migrate#secrets.yml"
	workflow := writeMigrationWorkflow(t, workflowPath, true)
	remote, _ := json.Marshal(map[string]string{"encoding": "base64", "content": base64.StdEncoding.EncodeToString(workflow)})
	gh := &migrationGitHubMock{results: []githubResult{
		{output: repositoryJSON()},
		{output: []byte(testMigrationCommit + "\n")},
		{output: remote},
		{output: []byte("https://github.com/acme/widgets/actions/runs/12345\n")},
	}}
	grantURL := "https://api.buildkite.com/v2/organizations/acme/clusters/" + testMigrationCluster + "/github-actions-secret-migrations/grant-identifier-123/secrets"
	bk := &migrationBuildkiteMock{grant: migrationGrant{ID: "grant-identifier-123", MigrationURL: grantURL, Audience: grantURL}}
	var stdout bytes.Buffer
	migration := githubActionsMigration{bk: bk, gh: gh, stdout: &stdout}
	if err := migration.run(t.Context(), workflowPath); err != nil {
		t.Fatal(err)
	}
	if bk.grantOrg != "acme" || bk.grantCluster != testMigrationCluster || bk.grantRequest.WorkflowSHA != testMigrationCommit || bk.grantRequest.DefaultBranchRef != "refs/heads/main" || bk.grantRequest.WorkflowPath != workflowPath {
		t.Fatalf("grant binding = %q/%q/%#v", bk.grantOrg, bk.grantCluster, bk.grantRequest)
	}
	if !strings.Contains(strings.Join(gh.calls[2].args, " "), "migrate%23secrets.yml?ref="+testMigrationCommit) {
		t.Fatalf("committed workflow request = %q", gh.calls[2].args)
	}
	dispatch := gh.calls[3]
	if !slices.Equal(dispatch.args, []string{"workflow", "run", workflowPath, "--repo", "acme/widgets", "--ref", "main", "--json"}) || string(dispatch.stdin) != `{"grant_id":"grant-identifier-123"}` {
		t.Fatalf("dispatch = %#v", dispatch)
	}
	for _, required := range []string{"Dispatch accepted", "migration is not complete", "https://github.com/acme/widgets/actions/runs/12345", "grant-identifier-123", "rerun this command"} {
		if !strings.Contains(stdout.String(), required) {
			t.Errorf("output missing %q: %s", required, stdout.String())
		}
	}
}

func TestRunMigrationDispatchFailureExplainsGrantRecovery(t *testing.T) {
	t.Chdir(t.TempDir())
	workflowPath := ".github/workflows/migrate.yml"
	workflow := writeMigrationWorkflow(t, workflowPath, false)
	remote, _ := json.Marshal(map[string]string{"encoding": "base64", "content": base64.StdEncoding.EncodeToString(workflow)})
	gh := &migrationGitHubMock{results: []githubResult{
		{output: repositoryJSON()}, {output: []byte(testMigrationCommit + "\n")}, {output: remote}, {err: errors.New("dispatch rejected")},
	}}
	grantURL := "https://api.buildkite.com/v2/organizations/acme/clusters/" + testMigrationCluster + "/github-actions-secret-migrations/grant-identifier-123/secrets"
	bk := &migrationBuildkiteMock{grant: migrationGrant{ID: "grant-identifier-123", MigrationURL: grantURL, Audience: grantURL}}
	err := (&githubActionsMigration{bk: bk, gh: gh, stdout: io.Discard}).run(t.Context(), workflowPath)
	if err == nil || !strings.Contains(err.Error(), "grant-identifier-123 will expire unused") || !strings.Contains(err.Error(), "rerun this command") {
		t.Fatalf("dispatch error = %v", err)
	}
}

func TestRunMigrationRejectsLocalOrCommittedTamperingBeforeGrant(t *testing.T) {
	workflow := mustRenderWorkflow(t)
	remoteWorkflow := workflow
	for _, test := range []struct {
		name    string
		local   []byte
		remote  []byte
		want    string
		ghCalls int
	}{
		{"local", bytes.Replace(workflow, []byte("timeout-minutes: 10"), []byte("timeout-minutes: 11"), 1), workflow, "deterministic", 0},
		{"committed", bytes.ReplaceAll(workflow, []byte("\n"), []byte("\r\n")), bytes.Replace(remoteWorkflow, []byte("timeout-minutes: 10"), []byte("timeout-minutes: 11"), 1), "local workflow differs", 3},
	} {
		t.Run(test.name, func(t *testing.T) {
			t.Chdir(t.TempDir())
			workflowPath := ".github/workflows/migrate.yml"
			if err := os.MkdirAll(filepath.Dir(workflowPath), 0o755); err != nil {
				t.Fatal(err)
			}
			if err := os.WriteFile(workflowPath, test.local, 0o644); err != nil {
				t.Fatal(err)
			}
			remote, _ := json.Marshal(map[string]string{"encoding": "base64", "content": base64.StdEncoding.EncodeToString(test.remote)})
			gh := &migrationGitHubMock{results: []githubResult{{output: repositoryJSON()}, {output: []byte(testMigrationCommit + "\n")}, {output: remote}}}
			bk := &migrationBuildkiteMock{}
			err := (&githubActionsMigration{bk: bk, gh: gh, stdout: io.Discard}).run(t.Context(), workflowPath)
			if err == nil || !strings.Contains(err.Error(), test.want) || len(gh.calls) != test.ghCalls || bk.grantRequest.WorkflowSHA != "" {
				t.Fatalf("error/calls/grant = %v/%d/%#v", err, len(gh.calls), bk.grantRequest)
			}
		})
	}
}

func TestMigrationBuildkiteAPICreatesGrantDirectly(t *testing.T) {
	var received migrationGrantRequest
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if r.Method != http.MethodPost || r.URL.Path != "/v2/organizations/acme/clusters/"+testMigrationCluster+"/github-actions-secret-migrations" {
			t.Errorf("request = %s %s", r.Method, r.URL.Path)
		}
		if err := json.NewDecoder(r.Body).Decode(&received); err != nil {
			t.Error(err)
		}
		w.Header().Set("Content-Type", "application/json")
		fmt.Fprint(w, `{"id":"grant-identifier-123"}`)
	}))
	defer server.Close()
	client, err := buildkite.NewOpts(buildkite.WithBaseURL(server.URL+"/"), buildkite.WithTokenAuth("token"))
	if err != nil {
		t.Fatal(err)
	}
	want := migrationGrantRequest{WorkflowSHA: testMigrationCommit, SecretNames: []string{"API_KEY"}}
	grant, err := (migrationBuildkiteAPI{client: client}).CreateGrant(t.Context(), "acme", testMigrationCluster, want)
	if err != nil || grant.ID != "grant-identifier-123" || received.WorkflowSHA != want.WorkflowSHA || !slices.Equal(received.SecretNames, want.SecretNames) {
		t.Fatalf("grant/request/error = %#v/%#v/%v", grant, received, err)
	}
}

func repositoryJSON() []byte {
	return []byte(`{"id":42,"full_name":"acme/widgets","html_url":"https://github.com/acme/widgets","default_branch":"main","owner":{"id":7}}`)
}

func testManifest() migrationManifest {
	return migrationManifest{
		Version: 1, Organization: "acme", Cluster: testMigrationCluster,
		Policy: "- pipeline_id: " + testMigrationPipeline + "\n", Repository: "acme/widgets",
		RepositoryID: 42, RepositoryOwnerID: 7, DefaultBranch: "main",
		SecretNames: []string{"API_KEY", "DEPLOY_TOKEN"},
	}
}

func mustRenderWorkflow(t *testing.T) []byte {
	t.Helper()
	workflow, err := renderOIDCSecretsMigrationWorkflow(testManifest())
	if err != nil {
		t.Fatal(err)
	}
	return workflow
}

func writeMigrationWorkflow(t *testing.T, workflowPath string, crlf bool) []byte {
	t.Helper()
	workflow := mustRenderWorkflow(t)
	local := workflow
	if crlf {
		local = bytes.ReplaceAll(workflow, []byte("\n"), []byte("\r\n"))
	}
	if err := os.MkdirAll(filepath.Dir(workflowPath), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(workflowPath, local, 0o644); err != nil {
		t.Fatal(err)
	}
	return workflow
}

func migrationWorkflowScript(t *testing.T, generated []byte) string {
	t.Helper()
	var workflow struct {
		Jobs map[string]struct {
			Steps []struct {
				Run string `yaml:"run"`
			} `yaml:"steps"`
		} `yaml:"jobs"`
	}
	if err := yaml.Unmarshal(generated, &workflow); err != nil {
		t.Fatal(err)
	}
	return workflow.Jobs["migrate"].Steps[0].Run
}

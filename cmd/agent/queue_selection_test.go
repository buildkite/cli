package agent

import (
	"archive/tar"
	"archive/zip"
	"bytes"
	"compress/gzip"
	"crypto/sha256"
	"fmt"
	"io"
	"net"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"testing"

	"github.com/alecthomas/kong"
	bkAgent "github.com/buildkite/cli/v3/internal/agent"
	"github.com/buildkite/cli/v3/internal/cli"
)

func TestAgentQueueFlags(t *testing.T) {
	t.Parallel()
	for _, command := range []string{"install", "run"} {
		for _, key := range []string{"", "deploy"} {
			t.Run(command+"/"+key, func(t *testing.T) {
				t.Parallel()
				var cmds struct {
					Install InstallCmd `cmd:""`
					Run     RunCmd     `cmd:""`
				}
				args := []string{command}
				if key != "" {
					args = append(args, "--queue", key)
				}
				if _, err := kong.Must(&cmds).Parse(args); err != nil {
					t.Fatal(err)
				}
				got := cmds.Install.Queue
				if command == "run" {
					got = cmds.Run.Queue
				}
				if got != key {
					t.Errorf("queue = %q, want %q", got, key)
				}
			})
		}
	}
	t.Run("no-token conflict", func(t *testing.T) {
		t.Parallel()
		var cmd InstallCmd
		if _, err := kong.Must(&cmd).Parse([]string{"--queue", "deploy", "--no-token"}); err == nil || !strings.Contains(err.Error(), "--queue cannot be used with --no-token") {
			t.Fatalf("expected no-token conflict, got %v", err)
		}
	})
}

func TestAgentQueueSelectionBeforeSideEffects(t *testing.T) {
	for _, command := range []string{"install", "run"} {
		for _, tt := range []struct {
			name, key, defaultID, queues, wantErr string
		}{
			{name: "missing", key: "missing", wantErr: `no queue with key "missing"`},
			{name: "hosted", key: "cloud", wantErr: `queue "cloud" is hosted`},
			{name: "hosted default", defaultID: "q-cloud", wantErr: `queue "cloud" is hosted`},
			{name: "missing default", wantErr: "no default queue is configured"},
			{name: "inaccessible default", defaultID: "q-other", wantErr: "configured default queue"},
			{name: "no queues", queues: `[]`, wantErr: "no default queue is configured"},
		} {
			t.Run(command+"/"+tt.name, func(t *testing.T) {
				downloads, tokens := 0, 0
				stubAgentDownloads(t, func(r *http.Request) (*http.Response, error) {
					downloads++
					return nil, fmt.Errorf("unexpected download: %s", r.URL)
				})
				queues := tt.queues
				if queues == "" {
					queues = `[{"id":"q-build","key":"build","hosted":false},{"id":"q-cloud","key":"cloud","hosted":true}]`
				}
				configureAgentAPI(t, tt.defaultID, queues, func(w http.ResponseWriter, r *http.Request) {
					if r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/tokens") {
						tokens++
					}
					t.Errorf("unexpected API request before queue selection: %s %s", r.Method, r.URL.Path)
					w.WriteHeader(http.StatusNotFound)
				})
				dir := t.TempDir()
				dest, config := filepath.Join(dir, "bin"), filepath.Join(dir, "agent.cfg")
				var err error
				if command == "install" {
					err = (&InstallCmd{Version: "latest", ClusterUUID: "cluster-1", Queue: tt.key, Dest: dest, ConfigPath: config}).Run(nil, cli.Globals{NoInput: true})
				} else {
					err = (&RunCmd{Version: "latest", ClusterName: "Default", Queue: tt.key}).Run(nil, cli.Globals{NoInput: true})
				}
				if err == nil || !strings.Contains(err.Error(), tt.wantErr) || !strings.Contains(err.Error(), "--queue <key>") {
					t.Fatalf("error = %v, want %q and retry advice", err, tt.wantErr)
				}
				if tokens != 0 {
					t.Errorf("created %d tokens before queue selection", tokens)
				}
				if downloads != 0 {
					t.Errorf("made %d downloads before queue selection", downloads)
				}
				for _, path := range []string{dest, config} {
					if _, err := os.Stat(path); !os.IsNotExist(err) {
						t.Errorf("unexpected install side effect at %s: %v", path, err)
					}
				}
			})
		}
	}
}

func TestAgentQueueConfigPropagation(t *testing.T) {
	for _, command := range []string{"install", "run"} {
		if command == "run" && runtime.GOOS == "windows" {
			continue // The fake agent is a shell script.
		}
		for _, key := range []string{"", "deploy"} {
			t.Run(command+"/key="+key, func(t *testing.T) {
				stubAgentArchive(t)
				tokens, revocations := 0, 0
				configureAgentAPI(t, "q-build", `[{"id":"q-build","key":"build","hosted":false},{"id":"q-deploy","key":"deploy","hosted":false},{"id":"q-default","key":"default","hosted":false}]`, func(w http.ResponseWriter, r *http.Request) {
					switch {
					case r.Method == http.MethodPost && strings.HasSuffix(r.URL.Path, "/tokens"):
						tokens++
						w.WriteHeader(http.StatusCreated)
						fmt.Fprint(w, `{"id":"token-1","token":"secret-value"}`)
					case r.Method == http.MethodDelete && strings.HasSuffix(r.URL.Path, "/tokens/token-1"):
						revocations++
						w.WriteHeader(http.StatusNoContent)
					default:
						t.Errorf("unexpected API request: %s %s", r.Method, r.URL.Path)
						w.WriteHeader(http.StatusNotFound)
					}
				})
				dir := t.TempDir()
				config := filepath.Join(dir, "agent.cfg")
				buildPath := filepath.Join(dir, "custom-builds")
				wantKey := key
				if wantKey == "" {
					wantKey = "build"
				}
				var err error
				if command == "install" {
					err = (&InstallCmd{Version: "3.0.0", ClusterUUID: "cluster-1", Queue: key, Dest: filepath.Join(dir, "bin"), ConfigPath: config, BuildPath: buildPath}).Run(nil, cli.Globals{NoInput: true})
				} else {
					// The fake agent copies its temporary config for inspection before cleanup.
					t.Setenv("BK_TEST_AGENT_CONFIG", config)
					err = (&RunCmd{Version: "3.0.0", ClusterUUID: "cluster-1", Queue: key}).Run(nil, cli.Globals{NoInput: true})
				}
				if err != nil {
					t.Fatal(err)
				}
				content, err := os.ReadFile(config)
				if err != nil {
					t.Fatal(err)
				}
				for _, want := range []string{`token="secret-value"`, `tags="queue=` + wantKey + `"`} {
					if !strings.Contains(string(content), want+"\n") {
						t.Errorf("config missing %q:\n%s", want, content)
					}
				}
				if command == "install" && !strings.Contains(string(content), fmt.Sprintf("build-path=%q\n", buildPath)) {
					t.Errorf("config lost build path: %s", content)
				}
				wantRevocations := 0
				if command == "run" {
					wantRevocations = 1
				}
				if tokens != 1 || revocations != wantRevocations {
					t.Errorf("tokens, revocations = %d, %d; want 1, %d", tokens, revocations, wantRevocations)
				}
			})
		}
	}
}

func TestInstallNoTokenSkipsAPI(t *testing.T) {
	stubAgentArchive(t)
	configureAgentAPI(t, "", `[]`, func(w http.ResponseWriter, r *http.Request) {
		t.Errorf("unexpected API request: %s %s", r.Method, r.URL.Path)
		w.WriteHeader(http.StatusNotFound)
	})
	// Catch even cluster/queue reads, not just token creation.
	t.Setenv("BUILDKITE_REST_API_ENDPOINT", "http://api-must-not-be-used.invalid")
	dir := t.TempDir()
	cmd := &InstallCmd{Version: "3.0.0", NoToken: true, Dest: filepath.Join(dir, "bin"), ConfigPath: filepath.Join(dir, "agent.cfg")}
	if err := cmd.Run(nil, cli.Globals{}); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(filepath.Join(cmd.Dest, bkAgent.BinaryName(runtime.GOOS))); err != nil {
		t.Fatal(err)
	}
	if _, err := os.Stat(cmd.ConfigPath); !os.IsNotExist(err) {
		t.Fatalf("no-token created config: %v", err)
	}
	cmd.Queue = "deploy"
	if err := cmd.Run(nil, cli.Globals{}); err == nil || !strings.Contains(err.Error(), "--queue cannot be used with --no-token") {
		t.Fatalf("expected conflict before API/downloads, got %v", err)
	}
}

// configureAgentAPI only handles cluster/queue reads. Other requests go to extra.
func configureAgentAPI(t *testing.T, defaultID, queues string, extra http.HandlerFunc) {
	t.Helper()
	s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Content-Type", "application/json")
		switch {
		case r.Method == http.MethodGet && r.URL.Path == "/v2/organizations/test-org/clusters":
			fmt.Fprint(w, `[{"id":"cluster-1","name":"Default"}]`)
		case r.Method == http.MethodGet && r.URL.Path == "/v2/organizations/test-org/clusters/cluster-1":
			fmt.Fprintf(w, `{"id":"cluster-1","default_queue_id":%q}`, defaultID)
		case r.Method == http.MethodGet && r.URL.Path == "/v2/organizations/test-org/clusters/cluster-1/queues":
			fmt.Fprint(w, queues)
		default:
			extra(w, r)
		}
	}))
	t.Cleanup(s.Close)
	t.Setenv("BUILDKITE_REST_API_ENDPOINT", s.URL)
	t.Setenv("BUILDKITE_API_TOKEN", "test-token")
	t.Setenv("BUILDKITE_ORGANIZATION_SLUG", "test-org")
}

type agentDownloadTransport func(*http.Request) (*http.Response, error)

func (f agentDownloadTransport) RoundTrip(r *http.Request) (*http.Response, error) { return f(r) }

// stubAgentDownloads replaces a process-global transport. Tests using it must not
// run in parallel, including with other tests that make HTTP requests.
func stubAgentDownloads(t *testing.T, download agentDownloadTransport) {
	t.Helper()
	original := http.DefaultTransport
	http.DefaultTransport = agentDownloadTransport(func(r *http.Request) (*http.Response, error) {
		host := r.URL.Hostname()
		if host == "localhost" || net.ParseIP(host).IsLoopback() {
			return original.RoundTrip(r)
		}
		return download(r)
	})
	t.Cleanup(func() { http.DefaultTransport = original })
}

func stubAgentArchive(t *testing.T) {
	t.Helper()
	var archive bytes.Buffer
	binary := []byte("#!/bin/sh\ncp \"$3\" \"$BK_TEST_AGENT_CONFIG\"\n")
	if runtime.GOOS == "windows" {
		zw := zip.NewWriter(&archive)
		f, err := zw.Create("buildkite-agent.exe")
		if err != nil {
			t.Fatal(err)
		}
		if _, err := f.Write(binary); err != nil {
			t.Fatal(err)
		}
		if err := zw.Close(); err != nil {
			t.Fatal(err)
		}
	} else {
		gz := gzip.NewWriter(&archive)
		tw := tar.NewWriter(gz)
		if err := tw.WriteHeader(&tar.Header{Name: "buildkite-agent", Mode: 0o755, Size: int64(len(binary))}); err != nil {
			t.Fatal(err)
		}
		if _, err := tw.Write(binary); err != nil {
			t.Fatal(err)
		}
		if err := tw.Close(); err != nil {
			t.Fatal(err)
		}
		if err := gz.Close(); err != nil {
			t.Fatal(err)
		}
	}
	stubAgentDownloads(t, func(r *http.Request) (*http.Response, error) {
		var body []byte
		switch r.URL.String() {
		case bkAgent.BuildDownloadURL("3.0.0", runtime.GOOS, runtime.GOARCH):
			body = archive.Bytes()
		case bkAgent.BuildSHA256SumsURL("3.0.0"):
			body = fmt.Appendf(nil, "%x  %s\n", sha256.Sum256(archive.Bytes()), filepath.Base(bkAgent.BuildDownloadURL("3.0.0", runtime.GOOS, runtime.GOARCH)))
		default:
			t.Errorf("unexpected download: %s", r.URL)
			return nil, fmt.Errorf("unexpected download: %s", r.URL)
		}
		return &http.Response{StatusCode: http.StatusOK, Header: make(http.Header), Body: io.NopCloser(bytes.NewReader(body)), Request: r}, nil
	})
}

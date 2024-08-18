package pkg

import (
	"encoding/json"
	"errors"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/buildkite/cli/v3/internal/config"
	"github.com/buildkite/cli/v3/pkg/cmd/factory"
	"github.com/buildkite/go-buildkite/v3/buildkite"
	"github.com/spf13/afero"
	"github.com/spf13/cobra"
)

func TestPackagePush(t *testing.T) {
	// Can't be parallel, as we're messing with global state (the isStdInReadableFunc)
	// t.Parallel()

	tempFile, err := os.CreateTemp("", "test.pkg")
	if err != nil {
		t.Fatalf("creating temp file: %v", err)
	}

	t.Cleanup(func() { tempFile.Close() })
	t.Cleanup(func() { os.Remove(tempFile.Name()) })

	io.WriteString(tempFile, "test package file contents!")

	packageResponse := buildkite.Package{
		ID:           "pkg-123",
		URL:          "https://api.buildkite.com/org/pkg-123",
		WebURL:       "https://buildkite.com/org/pkg-123",
		Organization: buildkite.Organization{},
		Registry:     buildkite.PackageRegistry{},
	}

	packageResponseBytes, err := json.Marshal(packageResponse)
	if err != nil {
		t.Fatalf("marshalling package response: %v", err)
	}

	cases := []struct {
		name  string
		stdin io.Reader
		flags map[string]string
		args  []string

		apiResponseCode int
		apiResponseBody []byte

		wantUnwrappedContain string
		wantErr              error
	}{
		// Config validation errors
		{
			name:                 "no registry",
			flags:                map[string]string{},
			args:                 []string{},
			wantUnwrappedContain: "--registry is required",
			wantErr:              ErrInvalidConfig,
		},
		{
			name: "stdin and file",
			flags: map[string]string{
				"registry": "my-registry",
				"file":     "/path/to/some/file",
			},
			args:                 []string{"-"},
			wantErr:              ErrInvalidConfig,
			wantUnwrappedContain: "cannot use --file when package is passed via stdin",
			stdin:                strings.NewReader("test"),
		},
		{
			name:                 "stdin with no file name",
			flags:                map[string]string{"registry": "my-registry"},
			args:                 []string{"-"},
			wantErr:              ErrInvalidConfig,
			wantUnwrappedContain: "--file-name is required when package is passed via stdin",
			stdin:                strings.NewReader("test"),
		},
		{
			name: "file that's a directory",
			flags: map[string]string{
				"registry": "my-registry",
				"file":     "/",
			},
			args:                 []string{},
			wantErr:              ErrInvalidConfig,
			wantUnwrappedContain: "package file at / is not a regular file, was: directory",
		},
		{
			name: "file that doesn't exist",
			flags: map[string]string{
				"registry": "my-registry",
				"file":     "/does-not-exist",
			},
			args:                 []string{},
			wantErr:              ErrInvalidConfig,
			wantUnwrappedContain: "file /does-not-exist did not exist",
		},

		// Happy paths
		{
			name: "file upload",
			flags: map[string]string{
				"registry": "my-registry",
				"file":     tempFile.Name(),
			},
			args: []string{"-"},

			apiResponseCode: http.StatusCreated,
			apiResponseBody: packageResponseBytes,
		},
		{
			name:  "file upload from stdin",
			stdin: strings.NewReader("test package stream contents!"),
			flags: map[string]string{
				"registry":  "my-registry",
				"file-name": "test.pkg",
			},
			args: []string{"-"},

			apiResponseCode: http.StatusCreated,
			apiResponseBody: packageResponseBytes,
		},

		// uh oh, the API returned an error!
		{
			name: "API error",
			flags: map[string]string{
				"registry": "my-registry",
				"file":     tempFile.Name(),
			},
			args: []string{},

			apiResponseCode: http.StatusBadRequest,

			wantErr:              ErrAPIError,
			wantUnwrappedContain: "/v2/packages/organizations/test/registries/my-registry/packages: 400",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Can't be parallel, as we're messing with global state (the isStdInReadableFunc)
			// t.Parallel()

			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				w.WriteHeader(tc.apiResponseCode)
				if len(tc.apiResponseBody) != 0 {
					w.Write(tc.apiResponseBody)
				}
			}))

			cmd, err := createCommand(t, createCommandInput{
				testServer: s,
				flags:      tc.flags,
				args:       tc.args,
				stdin:      tc.stdin,
			})
			if err != nil {
				t.Fatalf("creating test command: %v", err)
			}

			err = cmd.Execute()
			if !errors.Is(err, tc.wantErr) {
				t.Errorf("Expected error %v, got %v", tc.wantErr, err)
			}

			if err != nil && !strings.Contains(err.Error(), tc.wantUnwrappedContain) {
				t.Errorf("Expected error to contain %q, got %q", tc.wantUnwrappedContain, err.Error())
			}
		})
	}
}

type createCommandInput struct {
	testServer *httptest.Server
	flags      map[string]string
	args       []string
	stdin      io.Reader
}

func createCommand(t *testing.T, cci createCommandInput) (*cobra.Command, error) {
	t.Helper()

	client, err := buildkite.NewOpts(buildkite.WithBaseURL(cci.testServer.URL))
	if err != nil {
		return nil, err
	}

	conf := config.New(afero.NewMemMapFs(), nil)
	conf.SelectOrganization("test")

	f := &factory.Factory{Config: conf, RestAPIClient: client}

	cmd := NewCmdPackagePush(f)

	args := []string{}
	for k, v := range cci.flags {
		args = append(args, "--"+k, v)
	}

	args = append(args, cci.args...)
	cmd.SetArgs(args)

	if cci.stdin != nil {
		cmd.SetIn(cci.stdin)
		// Override the isStdInReadableFunc to always return true, we want to test
		// the validation logic, not the actual stdin
		was := isStdInReadableFunc
		isStdInReadableFunc = func() (bool, error) { return true, nil }
		t.Cleanup(func() { isStdInReadableFunc = was })
	}

	cmd.SetOut(io.Discard)
	cmd.SetErr(io.Discard)

	return cmd, nil
}

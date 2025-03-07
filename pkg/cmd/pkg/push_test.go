package pkg

import (
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"strings"
	"testing"

	"github.com/buildkite/cli/v3/internal/testutil"
	"github.com/buildkite/go-buildkite/v4"
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

		wantErrContain string
		wantErr        error
	}{
		// Config validation errors
		{
			name:           "no args",
			flags:          map[string]string{},
			args:           []string{},
			wantErrContain: "Exactly 2 arguments are required, got: 0",
			wantErr:        ErrInvalidConfig,
		},
		{
			name:           "too many args",
			args:           []string{"one", "two", "three"},
			wantErrContain: "Exactly 2 arguments are required, got: 3",
			wantErr:        ErrInvalidConfig,
		},
		{
			name:           "file that's a directory",
			flags:          map[string]string{},
			args:           []string{"my-registry", "/"},
			wantErr:        ErrInvalidConfig,
			wantErrContain: "file at / is not a regular file, mode was: directory",
		},
		{
			name:           "file that doesn't exist",
			args:           []string{"my-registry", "/does-not-exist"},
			wantErr:        ErrInvalidConfig,
			wantErrContain: "stat /does-not-exist: no such file or directory",
		},
		{
			name:           "stdin without --stdin-file-name",
			stdin:          strings.NewReader("test package stream contents!"),
			args:           []string{"my-registry", "-"},
			wantErr:        ErrInvalidConfig,
			wantErrContain: "When passing a package file via stdin, the --stdin-file-name flag must be provided",
		},

		// Happy paths
		{
			name: "file upload",
			args: []string{"my-registry", tempFile.Name()},

			apiResponseCode: http.StatusCreated,
			apiResponseBody: packageResponseBytes,
		},
		{
			name:  "file upload from stdin",
			stdin: strings.NewReader("test package stream contents!"),
			flags: map[string]string{"stdin-file-name": "test.pkg"},
			args:  []string{"my-registry", "-"},

			apiResponseCode: http.StatusCreated,
			apiResponseBody: packageResponseBytes,
		},

		// uh oh, the API returned an error!
		{
			name: "API error",
			args: []string{"my-registry", tempFile.Name()},

			apiResponseCode: http.StatusBadRequest,

			wantErr:        ErrAPIError,
			wantErrContain: "/v2/packages/organizations/test/registries/my-registry/packages/upload: 400",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			// Can't be parallel, as we're messing with global state (the isStdInReadableFunc)
			// t.Parallel()

			s := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
				// Package upload is a three-step process:
				// 1. Request a presigned URL from the API
				// 2. Upload the file to the presigned URL
				// 3. "Finalize" the upload by telling the API that the upload is complete
				// This test server really jankily simulates that process - it's not a real presigned URL, and we're just responding
				// with a 200 to the upload and finalize requests, and returning the absolute minimum response for the presigned URL
				// request
				if r.URL.Path == "/v2/packages/organizations/test/registries/my-registry/packages/upload" {
					// Return juuuust enough of a response to make the client happy, we don't want to be testing the internals
					// of the API client here (any more than we absolutely have to)
					w.WriteHeader(tc.apiResponseCode)
					payload := buildkite.PackagePresignedUpload{
						Form: buildkite.PackagePresignedUploadForm{
							URL:    "http://" + r.Host + "/upload",
							Method: "POST",
							Data: map[string]string{
								"key": "pkg-123",
							},
						},
					}
					err := json.NewEncoder(w).Encode(payload)
					if err != nil {
						t.Fatalf("encoding response: %v", err)
					}
					return
				}

				code := tc.apiResponseCode
				if code == 0 {
					code = http.StatusOK
				}
				w.WriteHeader(code)
				if len(tc.apiResponseBody) != 0 {
					w.Write(tc.apiResponseBody)
				}
			}))

			// Create custom stdin readable function for testing
			if tc.stdin != nil {
				// Override the isStdInReadableFunc to always return true
				was := isStdInReadableFunc
				isStdInReadableFunc = func() (bool, error) { return true, nil }
				t.Cleanup(func() { isStdInReadableFunc = was })
			}

			cmd, err := testutil.CreateCommand(t, testutil.CommandInput{
				TestServerURL: s.URL,
				Flags:         tc.flags,
				Args:          tc.args,
				Stdin:         tc.stdin,
				NewCmd:        NewCmdPackagePush,
			})
			if err != nil {
				t.Fatalf("creating test command: %v", err)
			}

			err = cmd.Execute()

			if tc.wantErr != nil {
				testutil.AssertErrorIs(t, err, tc.wantErr)
			} else {
				testutil.AssertNoError(t, err)
			}

			if tc.wantErrContain != "" {
				testutil.AssertErrorContains(t, err, tc.wantErrContain)
			}
		})
	}
}

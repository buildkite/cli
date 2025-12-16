package pkg

import (
	"errors"
	"io"
	"strings"
	"testing"
)

func TestPackagePushCommandArgs(t *testing.T) {
	t.Parallel()

	cases := []struct {
		name  string
		stdin io.Reader
		cmd   PushCmd

		wantErrContain string
		wantErr        error
	}{
		// Config validation errors
		{
			name: "no args",
			cmd: PushCmd{
				RegistrySlug:  "my-registry",
				FilePath:      "",
				StdinFileName: "",
				StdInArg:      "",
			},
			wantErrContain: "either a file path argument or --stdin-file-name must be provided",
			wantErr:        ErrInvalidConfig,
		},
		{
			name: "file that's a directory",
			cmd: PushCmd{
				RegistrySlug:  "my-registry",
				FilePath:      "/",
				StdinFileName: "",
				StdInArg:      "",
			},
			wantErr:        ErrInvalidConfig,
			wantErrContain: "file at / is not a regular file, mode was: directory",
		},
		{
			name: "file that doesn't exist",
			cmd: PushCmd{
				RegistrySlug:  "my-registry",
				FilePath:      "/does-not-exist",
				StdinFileName: "",
				StdInArg:      "",
			},
			wantErr:        ErrInvalidConfig,
			wantErrContain: "stat /does-not-exist: no such file or directory",
		},
		{
			name: "cannot provide both file path and stdin file name",
			cmd: PushCmd{
				RegistrySlug:  "my-registry",
				FilePath:      "/a-test-package.pkg",
				StdinFileName: "a-test-package.pkg",
				StdInArg:      "",
			},
			wantErr:        ErrInvalidConfig,
			wantErrContain: "cannot provide both a file path argument and --stdin-file-name",
		},
		{
			name: "file path but with stdin arg '-'",
			cmd: PushCmd{
				RegistrySlug:  "my-registry",
				FilePath:      "/directory/test.pkg",
				StdinFileName: "",
				StdInArg:      "-",
			},
			stdin:          strings.NewReader("test package stream contents!"),
			wantErr:        ErrInvalidConfig,
			wantErrContain: "when passing a package file via stdin, --stdin-file-name must be provided",
		},
		{
			name: "stdin without --stdin-file-name",
			cmd: PushCmd{
				RegistrySlug:  "my-registry",
				FilePath:      "",
				StdinFileName: "test",
				StdInArg:      "",
			},
			stdin:          strings.NewReader("test package stream contents!"),
			wantErr:        ErrInvalidConfig,
			wantErrContain: "when passing a package file via stdin, the final argument must be '-'",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {

			t.Parallel()

			err := tc.cmd.Validate()
			if !errors.Is(err, tc.wantErr) {
				t.Errorf("Expected error %v, got %v", tc.wantErr, err)
			}

			if err != nil && !strings.Contains(err.Error(), tc.wantErrContain) {
				t.Errorf("Expected error to contain %q, got %q", tc.wantErrContain, err.Error())
			}
		})
	}
}

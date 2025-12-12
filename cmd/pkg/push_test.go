package pkg

import (
	"errors"
	"io"
	"os"
	"strings"
	"testing"
)

func TestPackagePushCommandArgs(t *testing.T) {
	t.Parallel()

	tempFile, err := os.CreateTemp("", "test.pkg")
	if err != nil {
		t.Fatalf("creating temp file: %v", err)
	}

	t.Cleanup(func() { tempFile.Close() })
	t.Cleanup(func() { os.Remove(tempFile.Name()) })

	io.WriteString(tempFile, "test package file contents!")

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
			},
			wantErr:        ErrInvalidConfig,
			wantErrContain: "stat /does-not-exist: no such file or directory",
		},
		{
			name: "stdin without --stdin-file-name",
			cmd: PushCmd{
				RegistrySlug:  "my-registry",
				FilePath:      "",
				StdinFileName: "",
			},
			stdin:          strings.NewReader("test package stream contents!"),
			wantErr:        ErrInvalidConfig,
			wantErrContain: "When passing a package file via stdin, the --stdin-file-name flag must be provided",
		},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {

			t.Parallel()

			err := tc.cmd.validateCmdArgs()
			if !errors.Is(err, tc.wantErr) {
				t.Errorf("Expected error %v, got %v", tc.wantErr, err)
			}

			if err != nil && !strings.Contains(err.Error(), tc.wantErrContain) {
				t.Errorf("Expected error to contain %q, got %q", tc.wantErrContain, err.Error())
			}
		})
	}
}

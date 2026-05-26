package version

import (
	"fmt"
	"os"
	"strings"

	"github.com/buildkite/cli/v3/internal/selfupdate"
)

var Version = "DEV"

type VersionCmd struct{}

func (c *VersionCmd) Run() error {
	fmt.Fprintf(os.Stdout, "%s\n", Format(Version))

	if latest, ok := CheckForUpdate(Version); ok {
		installation, err := selfupdate.CurrentInstallation()
		if err != nil {
			installation = selfupdate.Installation{Method: selfupdate.InstallMethodStandalone}
		}
		fmt.Fprint(os.Stderr, FormatUpdateNudge(latest, installation))
	}

	return nil
}

func Format(ver string) string {
	ver = strings.TrimPrefix(ver, "v")
	return fmt.Sprintf("bk version %s\n", ver)
}

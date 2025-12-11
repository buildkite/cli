package version

import (
	"fmt"
	"os"
	"strings"
)

var Version = "DEV"

type VersionCmd struct{}

func (c *VersionCmd) Run() error {
	fmt.Fprintf(os.Stdout, "%s\n", Format(Version))
	return nil
}

func Format(ver string) string {
	ver = strings.TrimPrefix(ver, "v")
	return fmt.Sprintf("bk version %s\n", ver)
}

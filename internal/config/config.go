package config

import (
	"os"
	"path/filepath"
	"runtime"
)

const (
	appData           = "AppData"
	xdgConfigHome     = "XDG_CONFIG_HOME"
	APITokenConfigKey = "api_token"
)

// Config path precedence: XDG_CONFIG_HOME, AppData (windows only), HOME.
func ConfigFile() string {
	var path string
	if a := os.Getenv(xdgConfigHome); a != "" {
		path = filepath.Join(a, "bk.yaml")
	} else if b := os.Getenv(appData); runtime.GOOS == "windows" && b != "" {
		path = filepath.Join(b, "Buildkite CLI", "bk.yaml")
	} else {
		c, _ := os.UserHomeDir()
		path = filepath.Join(c, ".config", "bk.yaml")
	}
	return path
}

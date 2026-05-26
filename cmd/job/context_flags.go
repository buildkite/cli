package job

import (
	"fmt"
	"io"
	"os"
)

func warnIgnoredJobContextFlags(w io.Writer, pipeline, buildNumber string) {
	if pipeline == "" && buildNumber == "" {
		return
	}
	if w == nil {
		w = os.Stderr
	}

	fmt.Fprintln(w, "Warning: --pipeline and --build are deprecated and ignored because job UUIDs no longer require pipeline or build context")
}

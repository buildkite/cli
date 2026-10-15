package output

import (
	"io"
	"strings"
)

// Guidance styles explicitly guided output, leaving scripted messages unchanged.
// Colours use the terminal's own palette, without probing its background.
type Guidance struct {
	out         io.Writer
	interactive bool
}

func NewGuidance(out io.Writer, interactive bool) Guidance {
	return Guidance{out: out, interactive: interactive}
}

func (g Guidance) render(style, message string) string {
	if g.interactive && ColorEnabledFor(g.out) {
		return style + message + ansiReset
	}
	return message
}

// Heading retains section spacing even when colour is disabled.
func (g Guidance) Heading(message string) string {
	if !g.interactive {
		return message
	}
	return "\n" + g.render(ansiBold+"\033[36m", message)
}

func (g Guidance) Success(message string) string {
	return g.render("\033[32m", message)
}

func (g Guidance) Warning(message string) string {
	if g.interactive && !strings.HasPrefix(message, "Warning:") {
		message = "Warning: " + message
	}
	return g.render("\033[33m", message)
}

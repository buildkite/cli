package emoji

import (
	"regexp"
	"strings"
	"sync"

	"github.com/buildkite/termoji"
)

var (
	once     sync.Once
	renderer *termoji.Renderer

	// leadingShortcodes matches one or more :shortcode: tokens (with
	// optional whitespace between them) anchored at the start of the string.
	leadingShortcodes = regexp.MustCompile(`^(:[[:alnum:]_.+-]+:\s*)+`)
)

func getRenderer() *termoji.Renderer {
	once.Do(func() {
		r, err := termoji.New(termoji.Options{})
		if err != nil {
			return
		}
		renderer = r
	})
	return renderer
}

// Render expands emoji shortcodes in text using termoji. Standard
// Unicode emoji (e.g. :checkered_flag:) are converted to their Unicode
// code points on all terminals. Buildkite custom emoji (e.g. :docker:)
// are rendered as inline images on terminals that support the iTerm2 or
// Kitty graphics protocol; on other terminals they are left unchanged.
//
// Because inline-image escape sequences embed foreground-colour resets,
// callers should not wrap the result in ANSI foreground styling (e.g.
// lipgloss). For coloured output, use [Split] to separate the emoji
// prefix so it can be rendered outside the colour span.
func Render(text string) string {
	if r := getRenderer(); r != nil {
		return r.Render(text)
	}
	return text
}

// Split separates leading emoji shortcodes from the rest of the text.
// Whitespace between the emoji and text is trimmed from both sides so
// callers can control spacing consistently.
//
//	Split(":docker: Build image")         → (":docker:", "Build image")
//	Split(":docker: :go: Build")          → (":docker: :go:", "Build")
//	Split("Build image")                  → ("", "Build image")
//	Split(":pipeline:")                   → (":pipeline:", "")
//
// This is useful for coloured output: render the prefix with [Render]
// outside the ANSI colour span, and style only the rest.
func Split(text string) (prefix, rest string) {
	loc := leadingShortcodes.FindStringIndex(text)
	if loc == nil {
		return "", text
	}
	return strings.TrimRight(text[:loc[1]], " \t"), text[loc[1]:]
}

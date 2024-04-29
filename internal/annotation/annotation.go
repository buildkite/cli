package annotation

import (
	"regexp"

	"github.com/charmbracelet/lipgloss"
)

const (
	STYLE_DEFAULT = lipgloss.Color("#DDD")
	STYLE_INFO    = lipgloss.Color("#337AB7")
	STYLE_WARNING = lipgloss.Color("#FF841C")
	STYLE_ERROR   = lipgloss.Color("#F45756")
	STYLE_SUCCESS = lipgloss.Color("#2ECC40")

	// Max length for annotation body
	ANNOTATION_MAX_LENGTH = 120
)

func StripTags(body string) string {
	// Removing the tags separately allows us to maintian new lines in the output
	// Remove closing tags first
	re := regexp.MustCompile(`</[^>]+>`)
	body = re.ReplaceAllString(body, "")
	// Then rremove opening tags
	re = regexp.MustCompile(`<[^>]*>`)
	body = re.ReplaceAllString(body, "")

	if len(body) > ANNOTATION_MAX_LENGTH {
		return body[:ANNOTATION_MAX_LENGTH] + "...(more)"
	}
	return body
}

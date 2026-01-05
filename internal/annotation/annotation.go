package annotation

import "regexp"

// StripTags removes HTML tags from a string
func StripTags(html string) string {
	re := regexp.MustCompile(`</[^>]+>`)
	html = re.ReplaceAllString(html, "")

	re = regexp.MustCompile(`<[^>]*>`)
	return re.ReplaceAllString(html, "")
}

package local

// This tool implements the iterm2 image support as described here:
// http://iterm2.com/images.html
//
// Be sure to install iterm2 nightly.

import (
	"encoding/base64"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"log"
	"net/http"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	homedir "github.com/mitchellh/go-homedir"
)

const (
	appleEmojiMapping     = `https://github.com/buildkite/emojis/raw/master/img-apple-64.json`
	buildkiteEmojiMapping = `https://github.com/buildkite/emojis/raw/master/img-buildkite-64.json`
	emojiCachePrefix      = `https://github.com/buildkite/emojis/raw/master/`
)

var emojiRegexp = regexp.MustCompile(`:\w+:`)

func emojiCachePath() (string, error) {
	home, err := homedir.Dir()
	if err != nil {
		return "", err
	}
	return filepath.Join(home, ".buildkite", "emoji"), nil
}

type emojiLoader struct {
	cache *emojiCache
}

func newEmojiLoader() (*emojiLoader, error) {
	cachePath, err := emojiCachePath()
	if err != nil {
		return nil, err
	}
	return &emojiLoader{
		cache: &emojiCache{Path: cachePath},
	}, nil
}

func (e *emojiLoader) appleEmojis() (appleEmojis, error) {
	var result appleEmojis

	if err := e.cache.httpGetJSON(appleEmojiMapping, &result); err != nil {
		return result, err
	}

	return result, nil
}

func (e *emojiLoader) buildkiteEmojis() (buildkiteEmojis, error) {
	var result buildkiteEmojis

	if err := e.cache.httpGetJSON(buildkiteEmojiMapping, &result); err != nil {
		return result, err
	}

	return result, nil
}

func (el *emojiLoader) Render(line string) string {
	return emojiRegexp.ReplaceAllStringFunc(line, func(s string) string {
		bkEmojis, err := el.buildkiteEmojis()
		if err != nil {
			log.Printf("Err: %v", err)
			return s
		}

		if e, ok := bkEmojis.Match(s); ok {
			return e.Render(el.cache)
		}

		appleEmojis, err := el.appleEmojis()
		if err != nil {
			log.Printf("Err: %v", err)
			return s
		}

		if e, ok := appleEmojis.Match(s); ok {
			return e.Render()
		}

		return s
	})
}

type emojiCache struct {
	Path string
}

func (e *emojiCache) httpGetJSON(u string, into interface{}) error {
	b, err := e.httpGet(u)
	if err != nil {
		return err
	}

	return json.Unmarshal(b, into)
}

func (e *emojiCache) httpGet(u string) ([]byte, error) {
	if !strings.HasPrefix(u, emojiCachePrefix) {
		return nil, fmt.Errorf("Url doesn't start with %s", emojiCachePrefix)
	}

	cacheFilePath := filepath.Join(e.Path, strings.TrimPrefix(u, emojiCachePrefix))
	if _, err := os.Stat(cacheFilePath); err == nil {
		return ioutil.ReadFile(cacheFilePath)
	}

	res, err := http.Get(u)
	if err != nil {
		return nil, err
	}
	defer res.Body.Close()

	data, err := ioutil.ReadAll(res.Body)
	if err != nil {
		return nil, err
	}

	if err := os.MkdirAll(filepath.Dir(cacheFilePath), 0700); err != nil {
		log.Printf("Failed to Mkdir: %v", err)
	}

	if err = ioutil.WriteFile(cacheFilePath, data, 0600); err != nil {
		log.Printf("Failed to write %s: %v", cacheFilePath, err)
	}

	return data, nil
}

type buildkiteEmoji struct {
	Name     string   `json:"name"`
	Image    string   `json:"image"`
	Category string   `json:"category"`
	Aliases  []string `json:"aliases"`
}

func (e buildkiteEmoji) Render(cache *emojiCache) string {
	defaultReturn := ":" + e.Name + ":"

	if os.Getenv(`TERM_PROGRAM`) != `iTerm.app` {
		return defaultReturn
	}

	img, err := cache.httpGet(emojiCachePrefix + e.Image)
	if err != nil {
		log.Printf("Err: %v", err)
		return defaultReturn
	}

	return renderITerm2Image(img)
}

type buildkiteEmojis []buildkiteEmoji

func (be buildkiteEmojis) Match(name string) (buildkiteEmoji, bool) {
	name = strings.Trim(name, ":")

	for _, m := range be {
		if m.Name == name {
			return m, true
		}
		for _, a := range m.Aliases {
			if a == name {
				return m, true
			}
		}
	}

	return buildkiteEmoji{}, false
}

type appleEmoji struct {
	Name      string        `json:"name"`
	Category  string        `json:"category"`
	Image     string        `json:"image"`
	Unicode   string        `json:"unicode"`
	Aliases   []interface{} `json:"aliases"`
	Modifiers []interface{} `json:"modifiers"`
}

func (e appleEmoji) Render() string {
	b := strings.Builder{}

	for _, s := range strings.Split(e.Unicode, "-") {
		var err error
		var decoded string

		if len(s) > 4 {
			decoded, err = strconv.Unquote(fmt.Sprintf(`"\U%08s"`, s))
		} else {
			decoded, err = strconv.Unquote(fmt.Sprintf(`"\u%04s"`, s))
		}

		if err != nil {
			log.Printf("Error decoding %q: %v", e.Unicode, decoded)
			return e.Name
		}

		b.WriteString(decoded)
	}

	return b.String()
}

type appleEmojis []appleEmoji

func (ae appleEmojis) Match(name string) (appleEmoji, bool) {
	name = strings.Trim(name, ":")

	for _, m := range ae {
		if m.Name == name {
			return m, true
		}
		for _, a := range m.Aliases {
			if a == name {
				return m, true
			}
		}
	}

	return appleEmoji{}, false
}

func renderITerm2Image(data []byte) string {
	var b strings.Builder

	b.WriteString("\033]1337;")
	b.WriteString("File=inline=1")
	b.WriteString(";height=1")
	b.WriteString(":")
	b.WriteString(base64.StdEncoding.EncodeToString(data))
	b.WriteString("\a\b")

	return b.String()
}

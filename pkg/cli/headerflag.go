package cli

import (
	"fmt"
	"strings"

	"github.com/alecthomas/kong"
)

// HeaderFlag accumulates repeated --header values and supports both
// colon format (KEY: VALUE) and equals format (KEY=VALUE) for compatibility
type HeaderFlag struct {
	Values map[string]string
}

func (h *HeaderFlag) Decode(ctx *kong.DecodeContext) error {
	token, err := ctx.Scan.PopValue("string")
	if err != nil {
		return err
	}

	raw := token.String()

	var parts []string
	switch {
	case strings.Contains(raw, "="):
		parts = strings.SplitN(raw, "=", 2)
	case strings.Contains(raw, ":"):
		parts = strings.SplitN(raw, ":", 2)
	default:
		return fmt.Errorf("invalid header %q (expected KEY=VAL or KEY: VAL)", raw)
	}

	if len(parts) != 2 {
		return fmt.Errorf("invalid header %q (expected KEY=VAL or KEY: VAL)", raw)
	}

	key := strings.TrimSpace(parts[0])
	val := strings.TrimSpace(parts[1])
	if key == "" {
		return fmt.Errorf("header name cannot be empty")
	}

	if h.Values == nil {
		h.Values = make(map[string]string)
	}
	h.Values[key] = val
	return nil
}

func (h HeaderFlag) String() string {
	// Needed for kong --help output
	var s []string
	for k, v := range h.Values {
		s = append(s, fmt.Sprintf("%s=%s", k, v))
	}
	return strings.Join(s, ",")
}

package validation

import (
	"testing"

	"github.com/buildkite/cli/v3/internal/config"
)

func TestCheckValidConfiguration(t *testing.T) {
	t.Parallel()

	t.Run("API token is configured", func(t *testing.T) {
		c := config.Config{
			Organization: "testing",
			APIToken:     "testing",
		}

		f := CheckValidConfiguration(&c)
		err := f(nil, nil)

		if err != nil {
			t.Error("expected no error returned")
		}
	})

	t.Run("API token is not configured", func(t *testing.T) {
		c := config.Config{}

		f := CheckValidConfiguration(&c)
		err := f(nil, nil)

		if err == nil {
			t.Error("expected an error returned")
		}
	})
}

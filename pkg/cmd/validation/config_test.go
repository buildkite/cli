package validation

import (
	"testing"

	"github.com/buildkite/cli/v3/internal/config"
	"github.com/spf13/viper"
)

func TestCheckValidConfiguration(t *testing.T) {
	t.Parallel()

	t.Run("API token is configured", func(t *testing.T) {
		v := viper.New()
		v.Set(config.APITokenConfigKey, "testing")

		f := CheckValidConfiguration(v)
		err := f(nil, nil)

		if err != nil {
			t.Error("expected no error returned")
		}
	})

	t.Run("API token is not configured", func(t *testing.T) {
		v := viper.New()

		f := CheckValidConfiguration(v)
		err := f(nil, nil)

		if err == nil {
			t.Error("expected an error returned")
		}
	})
}

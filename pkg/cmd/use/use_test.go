package use

import (
	"testing"

	"github.com/buildkite/cli/v3/internal/config"
	"github.com/spf13/viper"
)

type viperMock struct {
	v *viper.Viper
}

func (v viperMock) Set(k string, val interface{}) {
	v.v.Set(k, val)
}

func (v viperMock) GetStringMap(k string) map[string]interface{} {
	return v.v.GetStringMap(k)
}
func (viperMock) WriteConfig() error {
	return nil
}

func TestCmdUse(t *testing.T) {
	t.Parallel()

	t.Run("uses already selected org", func(t *testing.T) {
		t.Parallel()
		c := config.Config{
			Organization: "testing",
		}
		selected := "testing"
		err := useRun(&selected, &c)
		if err != nil {
			t.Error("expected no error")
		}
		if c.Organization != "testing" {
			t.Error("expected no change in organization")
		}
	})

	t.Run("uses existing org", func(t *testing.T) {
		t.Parallel()
		_v := viper.New()
		v := viperMock{v: _v}

		// add some configurations
		c := config.Config{
			Organization: "testing",
			APIToken:     "token",
			V:            v,
		}
		_ = c.Save()
		c = config.Config{
			Organization: "default",
			APIToken:     "token",
			V:            v,
		}
		_ = c.Save()
		// now get a new empty config
		c = config.Config{
			V: v,
		}
		selected := "testing"
		err := useRun(&selected, &c)
		if err != nil {
			t.Errorf("expected no error: %s", err)
		}
		if c.Organization != "testing" {
			t.Error("expected no change in organization")
		}
	})

	t.Run("errors if missing org", func(t *testing.T) {
		t.Parallel()
		selected := "testing"
		err := useRun(&selected, &config.Config{V: viper.New()})
		if err == nil {
			t.Error("expected an error")
		}
	})
}

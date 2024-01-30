package list_test

import (
	"testing"

	"github.com/buildkite/cli/v3/internal/keys"
	"github.com/buildkite/cli/v3/internal/list"
	"github.com/charmbracelet/bubbles/help"
)

func TestKeyMapHelpMenu(t *testing.T) {
	t.Parallel()

	t.Run("it renders a help menu", func(t *testing.T) {
		t.Parallel()

		keymap := list.DefaultKeyMap()
		help := help.New()

		got := help.View(keymap)
		want := "q quit • ↑/k up • ↓/j down • g/home go to start • G/end go to end"

		if got != want {
			t.Fatalf("Output does not match expected. %s != %s", got, want)
		}
	})

	t.Run("it adds more menu items", func(t *testing.T) {
		t.Parallel()

		keymap := list.KeyMap([]keys.Binding{keys.NewBinding(keys.WithKeys("v"), keys.WithHelp("v", "view"))})
		help := help.New()

		got := help.View(keymap)
		want := "v view"

		if got != want {
			t.Fatalf("Output does not match expected. %s != %s", got, want)
		}
	})

	t.Run("it doesnt show disabled bindings", func(t *testing.T) {
		t.Parallel()

		keymap := list.KeyMap([]keys.Binding{
			keys.NewBinding(keys.WithKeys("v"), keys.WithHelp("v", "view")),
			keys.NewBinding(keys.WithKeys("b"), keys.WithHelp("w", "open in browser"), keys.WithDisabled()),
		})
		help := help.New()

		got := help.View(keymap)
		want := "v view"

		if got != want {
			t.Fatalf("Output does not match expected. %s != %s", got, want)
		}
	})
}

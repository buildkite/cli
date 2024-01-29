package list

import (
	"github.com/buildkite/cli/v3/internal/keys"
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
)

type KeyMap []keys.Binding

// FullHelp implements help.KeyMap.
func (km KeyMap) FullHelp() [][]key.Binding {
	return [][]key.Binding{
		km.AsBindings(),
	}
}

// ShortHelp implements help.KeyMap.
func (km KeyMap) ShortHelp() []key.Binding {
	return km.AsBindings()
}

type selectable interface {
	MoveDown()
	MoveTop()
	MoveBottom()
	MoveUp()
}

func (km KeyMap) AsBindings() []key.Binding {
	bindings := make([]key.Binding, len(km))

	for i, b := range km {
		bindings[i] = *b.Binding
	}

	return bindings
}

func DefaultKeyMap() KeyMap {
	return KeyMap{
		keys.NewBinding(
			keys.WithKeys("q"),
			keys.WithHelp("q", "quit"),
			keys.WithAction(func(tea.Model) any {
				return tea.Quit()
			}),
		),
		keys.NewBinding(
			keys.WithKeys("k", "up"),
			keys.WithHelp("↑/k", "up"),
			keys.WithAction(func(m tea.Model) any {
				if l, ok := m.(selectable); ok {
					l.MoveUp()
				}
				return nil
			}),
		),
		keys.NewBinding(
			keys.WithKeys("j", "down"),
			keys.WithHelp("↓/j", "down"),
			keys.WithAction(func(m tea.Model) any {
				if l, ok := m.(selectable); ok {
					l.MoveDown()
				}
				return nil
			}),
		),
		keys.NewBinding(
			keys.WithKeys("home", "g"),
			keys.WithHelp("g/home", "go to start"),
			keys.WithAction(func(m tea.Model) any {
				if l, ok := m.(selectable); ok {
					l.MoveTop()
				}
				return nil
			}),
		),
		keys.NewBinding(
			keys.WithKeys("end", "G"),
			keys.WithHelp("G/end", "go to end"),
			keys.WithAction(func(m tea.Model) any {
				if l, ok := m.(selectable); ok {
					l.MoveBottom()
				}
				return nil
			}),
		),
	}
}

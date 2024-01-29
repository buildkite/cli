package list

import (
	"github.com/buildkite/cli/v3/internal/keys"
	tea "github.com/charmbracelet/bubbletea"
)

type KeyMap []keys.Binding

type selectable interface {
	MoveDown()
	MoveTop()
	MoveBottom()
	MoveUp()
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

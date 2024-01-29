// Package keys provides types and function for defining user-configurable keymappings in bubbletea components.
// Example:
//
//	var quit = NewBinding(
//		WithKeys("q"),
//		WithAction(func(m tea.Model) any {
//			return tea.Quit()
//		}),
//		WithHelp("q", "quit"),
//	)
package keys

import (
	"github.com/charmbracelet/bubbles/key"
	tea "github.com/charmbracelet/bubbletea"
)

// Action is a callback function executed when a keybinding is pressed
type Action func(tea.Model) any

// Help is help information for a given keybinding.
type Help struct {
	Key  string
	Desc string
}

// Binding describes a set of keybindings and a callback to execute when pressed, along with their associated help text.
type Binding struct {
	*key.Binding
	action Action
}

// BindingOpt is an initialization option for a keybinding. It's used as an
// argument to NewBinding.
type BindingOpt func(*Binding)

// NewBinding returns a new keybinding from a set of BindingOpt options.
func NewBinding(opts ...BindingOpt) Binding {
	b := Binding{}
	for _, opt := range opts {
		opt(&b)
	}
	return b
}

// WithAction initializes a keybinding with the given callback to execute when pressed.
func WithAction(act Action) BindingOpt {
	return func(b *Binding) {
		b.action = act
	}
}

// WithKeys initializes a keybinding with the given keystrokes.
func WithKeys(keys ...string) BindingOpt {
	return func(b *Binding) {
		key.WithKeys(keys...)(b.Binding)
	}
}

// WithHelp initializes a keybinding with the given help text.
func WithHelp(k, desc string) BindingOpt {
	return func(b *Binding) {
		key.WithHelp(k, desc)(b.Binding)
	}
}

// WithDisabled initializes a disabled keybinding.
func WithDisabled() BindingOpt {
	return func(b *Binding) {
		key.WithDisabled()(b.Binding)
	}
}

// ExecuteAction calls the action for the keybinding
func (b *Binding) ExecuteAction(model tea.Model) any {
	return b.action(model)
}

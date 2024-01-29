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

import tea "github.com/charmbracelet/bubbletea"

// Action is a callback function executed when a keybinding is pressed
type Action func(tea.Model) any

// Help is help information for a given keybinding.
type Help struct {
	Key  string
	Desc string
}

// Binding describes a set of keybindings and a callback to execute when pressed, along with their associated help text.
type Binding struct {
	action   Action
	disabled bool
	help     Help
	keys     []string
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
		b.keys = keys
	}
}

// WithHelp initializes a keybinding with the given help text.
func WithHelp(key, desc string) BindingOpt {
	return func(b *Binding) {
		b.help = Help{Key: key, Desc: desc}
	}
}

// WithDisabled initializes a disabled keybinding.
func WithDisabled() BindingOpt {
	return func(b *Binding) {
		b.disabled = true
	}
}

// ExecuteAction calls the action for the keybinding
func (b *Binding) ExecuteAction(model tea.Model) any {
	return b.action(model)
}

// SetKeys sets the keys for the keybinding.
func (b *Binding) SetKeys(keys ...string) {
	b.keys = keys
}

// Keys returns the keys for the keybinding.
func (b Binding) Keys() []string {
	return b.keys
}

// SetHelp sets the help text for the keybinding.
func (b *Binding) SetHelp(key, desc string) {
	b.help = Help{Key: key, Desc: desc}
}

// Help returns the Help information for the keybinding.
func (b Binding) Help() Help {
	return b.help
}

// Enabled returns whether or not the keybinding is enabled. Disabled
// keybindings won't be activated and won't show up in help. Keybindings are
// enabled by default.
func (b Binding) Enabled() bool {
	return !b.disabled && b.keys != nil
}

// SetEnabled enables or disables the keybinding.
func (b *Binding) SetEnabled(v bool) {
	b.disabled = !v
}

// Matches checks if the given KeyMsg matches the given bindings.
func Matches(k tea.KeyMsg, b ...Binding) bool {
	keys := k.String()
	for _, binding := range b {
		for _, v := range binding.keys {
			if keys == v && binding.Enabled() {
				return true
			}
		}
	}
	return false
}

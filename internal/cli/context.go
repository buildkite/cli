package cli

type GlobalFlags interface {
	SkipConfirmation() bool
	DisableInput() bool
}

type Globals struct {
	Yes     bool
	NoInput bool
}

func (g Globals) SkipConfirmation() bool {
	return g.Yes
}

func (g Globals) DisableInput() bool {
	return g.NoInput
}

package cli

type GlobalFlags interface {
	SkipConfirmation() bool
	DisableInput() bool
	IsQuiet() bool
}

type Globals struct {
	Yes     bool
	NoInput bool
	Quiet   bool
}

func (g Globals) SkipConfirmation() bool {
	return g.Yes
}

func (g Globals) DisableInput() bool {
	return g.NoInput
}

func (g Globals) IsQuiet() bool {
	return g.Quiet
}

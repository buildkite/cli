package output

// OutputFlags provides shorthand flags for output format selection.
// Embed this struct in command structs to get --json, --yaml, --text flags
// in addition to the existing --output/-o flag.
type OutputFlags struct {
	JSON   bool   `help:"Output as JSON" xor:"format"`
	YAML   bool   `help:"Output as YAML" xor:"format"`
	Text   bool   `help:"Output as text" xor:"format"`
	Output string `help:"Output format. One of: json, yaml, text" short:"o" default:"${output_default_format}" enum:",json,yaml,text"`
}

// AfterApply is called by Kong after parsing to map boolean flags to the Output string.
func (o *OutputFlags) AfterApply() error {
	switch {
	case o.JSON:
		o.Output = "json"
	case o.YAML:
		o.Output = "yaml"
	case o.Text:
		o.Output = "text"
	}
	return nil
}

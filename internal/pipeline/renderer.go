package pipeline

import "github.com/charmbracelet/huh"

func RenderOptions(defaultPipeline string, pipelines []string) (string, error) {

	options := huh.NewOptions(pipelines...)

	if len(options) == 1 {
		options[0].Selected(true)
	}
	for i, opt := range options {
		if defaultPipeline == opt.Value {
			options[i] = opt.Selected(true)
		}
	}

	var choice string
	err := huh.NewForm(
		huh.NewGroup(
			huh.NewSelect[string]().
				Options(options...).
				Value(&choice),
		),
	).
		WithShowHelp(false).
		Run()

	return choice, err
}

package blocks

import "swaystats/theme"

// ErrorBlock creates a red block for failures.
func ErrorBlock(name, msg string) Block {
	b := Block{
		Name:                name,
		FullText:            msg,
		Separator:           false,
		SeparatorBlockWidth: SeparatorWidth,
	}
	if c, ok := theme.ColorFor(theme.SeverityDanger); ok {
		b.Color = c
	}
	return b
}

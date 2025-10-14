package blocks

// Block represents an i3bar protocol block.
// Only fields actually needed now; others can be added later.
type Block struct {
	Name                string `json:"name,omitempty"`
	Instance            string `json:"instance,omitempty"`
	FullText            string `json:"full_text"`
	ShortText           string `json:"short_text,omitempty"`
	Color               string `json:"color,omitempty"`
	Background          string `json:"background,omitempty"`
	Separator           bool   `json:"separator"`
	SeparatorBlockWidth int    `json:"separator_block_width,omitempty"`
	Urgent              bool   `json:"urgent,omitempty"`
	Markup              string `json:"markup,omitempty"`
}

const SeparatorWidth = 12

// Provider supplies an up-to-date Block, refreshing internal state at most
// when MaybeRefresh is called and it decides enough time has passed or data changed.
// MaybeRefresh returns true if the underlying Block value changed (for change-driven rendering decisions).
type Provider interface {
	Name() string
	MaybeRefresh(now int64) (changed bool)
	Current() Block
}

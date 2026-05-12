package chat

// slashPickerState backs the dynamic slash-command picker overlay.
//
// It is driven by the textarea contents: the surrounding code calls
// setFilter(value) whenever the input changes and the input still
// starts with "/", and nils the picker out when it does not.
type slashPickerState struct {
	all          []slashCommand
	filtered     []slashCommand
	filter       string
	selectedIdx  int
	scrollOffset int
}

func newSlashPickerState() *slashPickerState {
	all := slashCommands()
	filtered := make([]slashCommand, len(all))
	copy(filtered, all)
	return &slashPickerState{
		all:      all,
		filtered: filtered,
	}
}

func (p *slashPickerState) setFilter(filter string) {
	p.filter = filter
	p.filtered = fuzzySlashFilter(p.all, filter)
	if p.selectedIdx >= len(p.filtered) {
		p.selectedIdx = len(p.filtered) - 1
	}
	if p.selectedIdx < 0 {
		p.selectedIdx = 0
	}
	if p.scrollOffset > p.selectedIdx {
		p.scrollOffset = p.selectedIdx
	}
	if p.scrollOffset < 0 {
		p.scrollOffset = 0
	}
}

func (p *slashPickerState) moveSelection(delta int, visible int) {
	if len(p.filtered) == 0 {
		p.selectedIdx = 0
		p.scrollOffset = 0
		return
	}
	idx := p.selectedIdx + delta
	if idx < 0 {
		idx = 0
	}
	if idx > len(p.filtered)-1 {
		idx = len(p.filtered) - 1
	}
	p.selectedIdx = idx
	if visible <= 0 {
		visible = 1
	}
	if p.selectedIdx < p.scrollOffset {
		p.scrollOffset = p.selectedIdx
	}
	if p.selectedIdx >= p.scrollOffset+visible {
		p.scrollOffset = p.selectedIdx - visible + 1
	}
	if p.scrollOffset < 0 {
		p.scrollOffset = 0
	}
}

func (p *slashPickerState) selected() (slashCommand, bool) {
	if len(p.filtered) == 0 {
		return slashCommand{}, false
	}
	if p.selectedIdx < 0 || p.selectedIdx >= len(p.filtered) {
		return slashCommand{}, false
	}
	return p.filtered[p.selectedIdx], true
}

package chat

import "testing"

func TestSlashPickerInitialState(t *testing.T) {
	p := newSlashPickerState()
	if len(p.filtered) != len(slashCommands()) {
		t.Fatalf("expected initial filtered = all (%d), got %d", len(slashCommands()), len(p.filtered))
	}
	if p.selectedIdx != 0 {
		t.Errorf("expected selectedIdx 0, got %d", p.selectedIdx)
	}
}

func TestSlashPickerSetFilterShrinks(t *testing.T) {
	p := newSlashPickerState()
	p.setFilter("/mo")
	if len(p.filtered) == 0 {
		t.Fatal("expected matches for /mo")
	}
	if p.filtered[0].Name == "" {
		t.Errorf("first match has no name")
	}
	p.selectedIdx = len(p.filtered) - 1
	p.setFilter("/help")
	if p.selectedIdx >= len(p.filtered) {
		t.Errorf("selectedIdx %d not clamped to %d", p.selectedIdx, len(p.filtered))
	}
}

func TestSlashPickerMoveSelection(t *testing.T) {
	p := newSlashPickerState()
	p.moveSelection(1, 5)
	if p.selectedIdx != 1 {
		t.Errorf("want 1 got %d", p.selectedIdx)
	}
	p.moveSelection(-5, 5)
	if p.selectedIdx != 0 {
		t.Errorf("want clamp to 0, got %d", p.selectedIdx)
	}
	p.moveSelection(9999, 5)
	if p.selectedIdx != len(p.filtered)-1 {
		t.Errorf("want clamp to last (%d), got %d", len(p.filtered)-1, p.selectedIdx)
	}
}

func TestSlashPickerSelectedEmpty(t *testing.T) {
	p := newSlashPickerState()
	p.setFilter("/zzzz")
	if _, ok := p.selected(); ok {
		t.Errorf("expected selected() == false for empty filtered list")
	}
}

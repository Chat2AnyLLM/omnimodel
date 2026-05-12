package chat

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
)

func newTestTUIModel() chatTUIModel {
	m := newChatTUIModel(nil, "sess", "model", "chat", "anthropic", "google-adk", nil, nil)
	m.ready = true
	m.width = 120
	m.height = 40
	m.mainWidth = 100
	return m
}

func typeRune(t *testing.T, m chatTUIModel, r rune) chatTUIModel {
	t.Helper()
	out, _ := m.Update(tea.KeyMsg{Type: tea.KeyRunes, Runes: []rune{r}})
	mm, ok := out.(chatTUIModel)
	if !ok {
		t.Fatalf("Update returned non chatTUIModel: %T", out)
	}
	return mm
}

func TestSlashPickerOpensOnSlash(t *testing.T) {
	m := newTestTUIModel()
	if m.slashPicker != nil {
		t.Fatalf("picker should start closed")
	}
	m = typeRune(t, m, '/')
	if m.slashPicker == nil {
		t.Fatalf("picker should open after typing /")
	}
	if got := len(m.slashPicker.filtered); got != len(slashCommands()) {
		t.Errorf("picker should show all commands; want %d got %d", len(slashCommands()), got)
	}
}

func TestSlashPickerFiltersAsYouType(t *testing.T) {
	m := newTestTUIModel()
	m = typeRune(t, m, '/')
	m = typeRune(t, m, 'm')
	m = typeRune(t, m, 'o')
	if m.slashPicker == nil {
		t.Fatalf("picker should still be open")
	}
	names := map[string]bool{}
	for _, c := range m.slashPicker.filtered {
		names[c.Name] = true
	}
	for _, want := range []string{"/model", "/models", "/mode"} {
		if !names[want] {
			t.Errorf("expected %q in filtered set; got %v", want, namesOf(m.slashPicker.filtered))
		}
	}
}

func TestSlashPickerClosesWhenSlashDeleted(t *testing.T) {
	m := newTestTUIModel()
	m = typeRune(t, m, '/')
	if m.slashPicker == nil {
		t.Fatal("picker should be open")
	}
	out, _ := m.Update(tea.KeyMsg{Type: tea.KeyBackspace})
	m = out.(chatTUIModel)
	if m.slashPicker != nil {
		t.Errorf("picker should close after deleting leading /")
	}
	// strings import used to silence unused on smaller test runs
	_ = strings.TrimSpace
}

func namesOf(cs []slashCommand) []string {
	out := make([]string, len(cs))
	for i, c := range cs {
		out[i] = c.Name
	}
	return out
}

func TestSlashPickerDownArrowMovesSelection(t *testing.T) {
	m := newTestTUIModel()
	m = typeRune(t, m, '/')
	out, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
	m = out.(chatTUIModel)
	if m.slashPicker == nil {
		t.Fatal("picker should still be open")
	}
	if m.slashPicker.selectedIdx != 1 {
		t.Errorf("expected selectedIdx=1, got %d", m.slashPicker.selectedIdx)
	}
}

func TestSlashPickerEscapeClosesKeepsText(t *testing.T) {
	m := newTestTUIModel()
	m = typeRune(t, m, '/')
	m = typeRune(t, m, 'm')
	out, _ := m.Update(tea.KeyMsg{Type: tea.KeyEscape})
	m = out.(chatTUIModel)
	if m.slashPicker != nil {
		t.Errorf("picker should be closed after Esc")
	}
	if got := m.textarea.Value(); got != "/m" {
		t.Errorf("textarea should retain %q, got %q", "/m", got)
	}
}

func moveSelectionTo(t *testing.T, m chatTUIModel, name string) chatTUIModel {
	t.Helper()
	for i := 0; i < len(m.slashPicker.filtered); i++ {
		if sel, ok := m.slashPicker.selected(); ok && sel.Name == name {
			return m
		}
		out, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
		m = out.(chatTUIModel)
	}
	if sel, _ := m.slashPicker.selected(); sel.Name != name {
		t.Fatalf("could not move selection to %q; ended on %q", name, sel.Name)
	}
	return m
}

func TestSlashPickerEnterOnArglessSubmits(t *testing.T) {
	m := newTestTUIModel()
	m = typeRune(t, m, '/')
	m = moveSelectionTo(t, m, "/models")
	out, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = out.(chatTUIModel)
	if m.slashPicker != nil {
		t.Errorf("picker should close after Enter")
	}
	if got := m.textarea.Value(); got != "" {
		t.Errorf("textarea should be empty after submit, got %q", got)
	}
}

func TestSlashPickerEnterOnArgTakingFillsInput(t *testing.T) {
	m := newTestTUIModel()
	m = typeRune(t, m, '/')
	m = moveSelectionTo(t, m, "/model")
	out, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = out.(chatTUIModel)
	if m.slashPicker != nil {
		t.Errorf("picker should close after Enter on arg-taking command")
	}
	if got := m.textarea.Value(); got != "/model " {
		t.Errorf("textarea should be %q, got %q", "/model ", got)
	}
}

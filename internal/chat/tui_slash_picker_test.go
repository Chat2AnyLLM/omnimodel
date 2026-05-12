package chat

import (
	"strings"
	"testing"

	tea "github.com/charmbracelet/bubbletea"
	xansi "github.com/charmbracelet/x/ansi"
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

func renderEntriesForTest(entries []transcriptEntry) string {
	parts := make([]string, 0, len(entries))
	for _, entry := range entries {
		parts = append(parts, entry.content)
	}
	return strings.Join(parts, "\n")
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

func TestSlashPickerEnterOnArgTakingWithArgsSubmits(t *testing.T) {
	m := newTestTUIModel()
	m.applyTextareaValue("/speckit.specify XXXXXX")
	m.updateSlashPicker()
	if m.slashPicker == nil {
		t.Fatal("picker should be open")
	}
	out, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = out.(chatTUIModel)
	if m.slashPicker != nil {
		t.Errorf("picker should close after Enter")
	}
	if got := m.textarea.Value(); got != "" {
		t.Errorf("textarea should be empty after submit, got %q", got)
	}
}

func TestSlashPickerViewShowsTypedCommandInInput(t *testing.T) {
	m := newTestTUIModel()
	m.height = 12
	m = typeRune(t, m, '/')
	m = typeRune(t, m, 's')
	m = typeRune(t, m, 'l')

	out := xansi.Strip(m.View())
	if !strings.Contains(out, "/sl") {
		t.Fatalf("View() should show typed slash command in input; got:\n%s", out)
	}
}

func TestSlashPickerViewKeepsInputVisibleInShortWindow(t *testing.T) {
	m := newTestTUIModel()
	m.height = 8
	m = typeRune(t, m, '/')
	m.streamActive = true
	out := xansi.Strip(m.View())
	if !strings.Contains(out, "/") {
		t.Fatalf("View() should keep slash input visible in short window; got:\n%s", out)
	}
	if strings.Contains(out, "Commands") {
		t.Fatalf("View() should hide slash picker before it covers the input in short window; got:\n%s", out)
	}
}

func TestTUIHandleSlashSpecKitAliasRoutesToAgent(t *testing.T) {
	m := newTestTUIModel()
	out, cmd := m.handleSlash("/speckit.specify add login flow")
	if cmd == nil {
		t.Fatalf("expected agent stream command")
	}
	mm, ok := out.(chatTUIModel)
	if !ok {
		t.Fatalf("handleSlash returned non chatTUIModel: %T", out)
	}
	if mm.mode != "agent" || mm.specMode != "spec-kit" {
		t.Fatalf("expected agent spec-kit mode, got mode=%q specMode=%q", mm.mode, mm.specMode)
	}
	if !mm.streamActive || !mm.spinning {
		t.Fatalf("expected active agent stream")
	}
	if len(mm.entries) == 0 {
		t.Fatalf("expected routing entries")
	}
	got := xansi.Strip(renderEntriesForTest(mm.entries))
	for _, want := range []string{"/speckit.specify", "speckit_specify", "Running agent workflow", "load the spec skill", "add login flow"} {
		if !strings.Contains(got, want) {
			t.Fatalf("TUI output missing %q\n---\n%s", want, got)
		}
	}
	if strings.Contains(got, "Unknown command") {
		t.Fatalf("TUI should not show unknown command for speckit alias\n---\n%s", got)
	}
}

func TestTUIHandleSlashOpenSpecAliasRoutesToAgent(t *testing.T) {
	m := newTestTUIModel()
	out, cmd := m.handleSlash("/opsx:explore auth ideas")
	if cmd == nil {
		t.Fatalf("expected agent stream command")
	}
	mm, ok := out.(chatTUIModel)
	if !ok {
		t.Fatalf("handleSlash returned non chatTUIModel: %T", out)
	}
	if mm.mode != "agent" || mm.specMode != "openspec" {
		t.Fatalf("expected agent openspec mode, got mode=%q specMode=%q", mm.mode, mm.specMode)
	}
	if !mm.streamActive || !mm.spinning {
		t.Fatalf("expected active agent stream")
	}
	if len(mm.entries) == 0 {
		t.Fatalf("expected routing entries")
	}
	got := xansi.Strip(renderEntriesForTest(mm.entries))
	for _, want := range []string{"/opsx:explore", "openspec_explore", "Running agent workflow", "load the spec skill", "auth ideas"} {
		if !strings.Contains(got, want) {
			t.Fatalf("TUI output missing %q\n---\n%s", want, got)
		}
	}
	if strings.Contains(got, "Unknown command") {
		t.Fatalf("TUI should not show unknown command for opsx alias\n---\n%s", got)
	}
}

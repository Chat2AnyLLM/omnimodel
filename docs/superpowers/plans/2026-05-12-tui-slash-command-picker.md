# TUI Dynamic Slash-Command Picker Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** When the user types `/` in the OmniCode TUI input, open a live-filtering command picker that lists slash commands and lets the user pick one with arrow keys + Enter.

**Architecture:** Add a static slash-command registry (`internal/chat/slash_commands.go`) plus a `slashPickerState` driven entirely by the textarea contents. Existing key handling and rendering in `internal/chat/tui.go` are extended to (a) intercept Up/Down/Enter/Esc while the picker is open, (b) recompute the filter after each textarea update, and (c) draw a small overlay above the input. `/help` is rewritten to render from the same registry.

**Tech Stack:** Go, Bubble Tea (`github.com/charmbracelet/bubbletea`), Bubbles textarea, Lipgloss.

**Spec:** `docs/superpowers/specs/2026-05-12-tui-slash-command-picker-design.md`

---

## File Plan

- Create: `internal/chat/slash_commands.go` — registry, fuzzy filter, help renderer.
- Create: `internal/chat/slash_commands_test.go` — registry + filter tests.
- Create: `internal/chat/slash_picker.go` — `slashPickerState`, lifecycle helpers, render helper.
- Create: `internal/chat/slash_picker_test.go` — state tests.
- Create: `internal/chat/tui_slash_picker_test.go` — TUI integration tests using `Update`.
- Modify: `internal/chat/tui.go` — add `slashPicker` field, lifecycle in `Update`, key gating, render call in `View`, rewrite `/help` case.

Conventions: package `chat`; tests use the standard `testing` package (consistent with `internal/chat/chat_test.go`). Run with `go test ./internal/chat/...` from the repo root in PowerShell. Build with `go build ./...`.

---

### Task 1: Slash command registry skeleton

**Files:**
- Create: `internal/chat/slash_commands.go`
- Test: `internal/chat/slash_commands_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/chat/slash_commands_test.go`:

```go
package chat

import "testing"

func TestSlashCommandsCatalogShape(t *testing.T) {
	cmds := slashCommands()
	if len(cmds) == 0 {
		t.Fatalf("slashCommands() returned empty catalog")
	}

	seen := map[string]bool{}
	for _, c := range cmds {
		if c.Name == "" || c.Name[0] != '/' {
			t.Errorf("invalid command name %q", c.Name)
		}
		if c.Summary == "" {
			t.Errorf("command %q has empty summary", c.Name)
		}
		if seen[c.Name] {
			t.Errorf("duplicate command name %q", c.Name)
		}
		seen[c.Name] = true
		for _, a := range c.Aliases {
			if seen[a] {
				t.Errorf("duplicate alias %q on %q", a, c.Name)
			}
			seen[a] = true
		}
	}

	for _, must := range []string{"/help", "/new", "/sessions", "/session", "/mode", "/apishape", "/permissions", "/model", "/agent", "/max-turns", "/models", "/spec", "/clear", "/quit"} {
		if !seen[must] {
			t.Errorf("catalog missing required command %q", must)
		}
	}
}
```

- [ ] **Step 2: Run test to verify it fails**

Run (PowerShell):

```
go test ./internal/chat/ -run TestSlashCommandsCatalogShape
```

Expected: FAIL — `slashCommands` undefined.

- [ ] **Step 3: Write minimal implementation**

Create `internal/chat/slash_commands.go`:

```go
package chat

// slashCommand describes one built-in TUI slash command.
type slashCommand struct {
	Name      string   // canonical name including leading "/"
	Aliases   []string // optional aliases (e.g. "?" for "/help")
	Summary   string   // one-line description shown in picker and /help
	TakesArgs bool     // true when the command accepts arguments
}

// slashCommands returns the static catalog of built-in slash commands.
// The order is the order in which they are presented in /help and as
// the initial picker order before any filter is applied.
func slashCommands() []slashCommand {
	return []slashCommand{
		{Name: "/help", Aliases: []string{"?"}, Summary: "show available commands"},
		{Name: "/new", TakesArgs: true, Summary: "start a new session [title]"},
		{Name: "/sessions", Summary: "browse and resume a previous session"},
		{Name: "/session", Summary: "show current session info"},
		{Name: "/mode", TakesArgs: true, Summary: "show or switch mode (chat|agent)"},
		{Name: "/apishape", Aliases: []string{"/api-shape"}, TakesArgs: true, Summary: "show or set the agent API request shape"},
		{Name: "/permissions", Summary: "toggle autopilot (auto-approve tool calls)"},
		{Name: "/model", TakesArgs: true, Summary: "show or switch model"},
		{Name: "/agent", TakesArgs: true, Summary: "show or set the agent backend"},
		{Name: "/max-turns", TakesArgs: true, Summary: "show or set max agent turns (1-100)"},
		{Name: "/models", Summary: "open model picker"},
		{Name: "/spec", TakesArgs: true, Summary: "spec-driven workflow commands"},
		{Name: "/clear", Aliases: []string{"/cls"}, Summary: "clear the screen"},
		{Name: "/quit", Aliases: []string{"/exit"}, Summary: "quit"},
	}
}
```

- [ ] **Step 4: Run test to verify it passes**

Run:

```
go test ./internal/chat/ -run TestSlashCommandsCatalogShape
```

Expected: PASS.

- [ ] **Step 5: Commit**

```
git add internal/chat/slash_commands.go internal/chat/slash_commands_test.go
git -c user.name="James Zhu" -c user.email="zhujian0805@gmail.com" commit -m "feat(tui): add slash-command registry"
```

---

### Task 2: Fuzzy filter for slash commands

**Files:**
- Modify: `internal/chat/slash_commands.go`
- Test: `internal/chat/slash_commands_test.go`

- [ ] **Step 1: Write the failing test**

Append to `internal/chat/slash_commands_test.go`:

```go
func TestFuzzySlashFilter(t *testing.T) {
	all := slashCommands()

	cases := []struct {
		name    string
		filter  string
		want    []string // ordered prefix of expected results
		notWant []string
	}{
		{name: "empty returns all", filter: "", want: []string{"/help"}},
		{name: "leading slash only returns all", filter: "/", want: []string{"/help"}},
		{name: "prefix match", filter: "/mo", want: []string{"/model", "/models", "/mode"}},
		{name: "no leading slash still matches", filter: "mo", want: []string{"/model", "/models", "/mode"}},
		{name: "question mark alias", filter: "?", want: []string{"/help"}},
		{name: "no match", filter: "/zzzz", notWant: []string{"/help"}},
	}

	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := fuzzySlashFilter(all, tc.filter)
			if tc.filter == "" || tc.filter == "/" {
				if len(got) != len(all) {
					t.Fatalf("filter %q: want all %d, got %d", tc.filter, len(all), len(got))
				}
				return
			}
			if len(tc.want) > 0 && len(got) < len(tc.want) {
				t.Fatalf("filter %q: want at least %v, got %v", tc.filter, tc.want, names(got))
			}
			for i, name := range tc.want {
				if got[i].Name != name {
					t.Errorf("filter %q: position %d want %q got %q (full=%v)", tc.filter, i, name, got[i].Name, names(got))
				}
			}
			for _, nw := range tc.notWant {
				for _, g := range got {
					if g.Name == nw {
						t.Errorf("filter %q: did not want %q", tc.filter, nw)
					}
				}
			}
		})
	}
}

func names(cs []slashCommand) []string {
	out := make([]string, len(cs))
	for i, c := range cs {
		out[i] = c.Name
	}
	return out
}
```

- [ ] **Step 2: Run test to verify it fails**

```
go test ./internal/chat/ -run TestFuzzySlashFilter
```

Expected: FAIL — `fuzzySlashFilter` undefined.

- [ ] **Step 3: Write minimal implementation**

Append to `internal/chat/slash_commands.go`:

```go
import (
	"sort"
	"strings"
)

// fuzzySlashFilter ranks the catalog against filter using the same
// scoring shape as fuzzyScore in tui.go: prefix > substring > subsequence.
// Aliases are considered. The leading "/" in filter is ignored so users
// who type "mo" get the same results as "/mo".
func fuzzySlashFilter(all []slashCommand, filter string) []slashCommand {
	q := strings.TrimSpace(strings.ToLower(filter))
	q = strings.TrimPrefix(q, "/")
	if q == "" {
		out := make([]slashCommand, len(all))
		copy(out, all)
		return out
	}

	type match struct {
		cmd   slashCommand
		score int
		order int
	}

	var matches []match
	for i, c := range all {
		best := 0
		matched := false
		candidates := append([]string{c.Name}, c.Aliases...)
		for _, name := range candidates {
			text := strings.TrimPrefix(strings.ToLower(name), "/")
			if score, ok := fuzzyScore(text, q); ok {
				matched = true
				if score > best {
					best = score
				}
			}
		}
		if matched {
			matches = append(matches, match{cmd: c, score: best, order: i})
		}
	}

	sort.SliceStable(matches, func(i, j int) bool {
		if matches[i].score != matches[j].score {
			return matches[i].score > matches[j].score
		}
		return matches[i].order < matches[j].order
	})

	out := make([]slashCommand, len(matches))
	for i, m := range matches {
		out[i] = m.cmd
	}
	return out
}
```

Note: `fuzzyScore` already exists in `internal/chat/tui.go` (same package).

- [ ] **Step 4: Run test to verify it passes**

```
go test ./internal/chat/ -run TestFuzzySlashFilter
```

Expected: PASS.

- [ ] **Step 5: Commit**

```
git add internal/chat/slash_commands.go internal/chat/slash_commands_test.go
git -c user.name="James Zhu" -c user.email="zhujian0805@gmail.com" commit -m "feat(tui): fuzzy filter for slash commands"
```

---

### Task 3: Help renderer from registry

**Files:**
- Modify: `internal/chat/slash_commands.go`
- Test: `internal/chat/slash_commands_test.go`

- [ ] **Step 1: Write the failing test**

Append to `internal/chat/slash_commands_test.go`:

```go
import "strings"

func TestRenderSlashHelp(t *testing.T) {
	out := renderSlashHelp(slashCommands())
	for _, want := range []string{"/help", "/models", "/quit", "show available commands"} {
		if !strings.Contains(out, want) {
			t.Errorf("renderSlashHelp output missing %q\n---\n%s", want, out)
		}
	}
	if !strings.HasPrefix(strings.TrimSpace(out), "**Commands:**") {
		t.Errorf("renderSlashHelp output should start with **Commands:** header; got:\n%s", out)
	}
}
```

(If `strings` import is already present in the test file, do not duplicate it.)

- [ ] **Step 2: Run test to verify it fails**

```
go test ./internal/chat/ -run TestRenderSlashHelp
```

Expected: FAIL — `renderSlashHelp` undefined.

- [ ] **Step 3: Write minimal implementation**

Append to `internal/chat/slash_commands.go`:

```go
import "fmt"

// renderSlashHelp produces the markdown body for the /help command.
func renderSlashHelp(cmds []slashCommand) string {
	var b strings.Builder
	b.WriteString("**Commands:**\n\n")
	for _, c := range cmds {
		names := c.Name
		if len(c.Aliases) > 0 {
			names = fmt.Sprintf("%s (%s)", c.Name, strings.Join(c.Aliases, ", "))
		}
		b.WriteString(fmt.Sprintf("- `%s` — %s\n", names, c.Summary))
	}
	b.WriteString("\n**Keyboard shortcuts:**\n\n")
	b.WriteString("- `/` — open the command picker; type to filter, ↑↓ to navigate, Enter to select, Esc to close\n")
	b.WriteString("- `Shift+Tab` — toggle autopilot (auto-approve tool calls)\n")
	b.WriteString("- `↑`/`↓` — focus expandable tool results (when input is empty)\n")
	b.WriteString("- `Space` — expand/collapse the focused tool result\n")
	b.WriteString("- `Esc` — cancel current running job\n")
	return b.String()
}
```

- [ ] **Step 4: Run test to verify it passes**

```
go test ./internal/chat/ -run TestRenderSlashHelp
```

Expected: PASS.

- [ ] **Step 5: Commit**

```
git add internal/chat/slash_commands.go internal/chat/slash_commands_test.go
git -c user.name="James Zhu" -c user.email="zhujian0805@gmail.com" commit -m "feat(tui): render /help from slash-command registry"
```

---

### Task 4: Slash picker state

**Files:**
- Create: `internal/chat/slash_picker.go`
- Test: `internal/chat/slash_picker_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/chat/slash_picker_test.go`:

```go
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
```

- [ ] **Step 2: Run test to verify it fails**

```
go test ./internal/chat/ -run TestSlashPicker
```

Expected: FAIL — `newSlashPickerState` undefined.

- [ ] **Step 3: Write minimal implementation**

Create `internal/chat/slash_picker.go`:

```go
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
```

- [ ] **Step 4: Run test to verify it passes**

```
go test ./internal/chat/ -run TestSlashPicker
```

Expected: PASS.

- [ ] **Step 5: Commit**

```
git add internal/chat/slash_picker.go internal/chat/slash_picker_test.go
git -c user.name="James Zhu" -c user.email="zhujian0805@gmail.com" commit -m "feat(tui): add slash picker state"
```

---

### Task 5: Wire picker lifecycle into the TUI model

**Files:**
- Modify: `internal/chat/tui.go`

- [ ] **Step 1: Add `slashPicker` field**

In the `chatTUIModel` struct (around line 496, near `picker *modelPickerState`), add:

```go
	slashPicker          *slashPickerState
```

- [ ] **Step 2: Add lifecycle helper**

Append (anywhere below `chatTUIModel` definition; keep package-level helpers grouped — recommended just before `func (m chatTUIModel) View()` near line 1167):

```go
// updateSlashPicker opens, updates, or closes the slash-command picker
// based on the current textarea contents. It must be called after every
// textarea update.
func (m *chatTUIModel) updateSlashPicker() {
	if m.streamActive || m.pendingPermission != nil || m.historySearchMode {
		m.slashPicker = nil
		return
	}
	value := m.textarea.Value()
	// Only single-line "/..." input opens the picker.
	if strings.Contains(value, "\n") {
		m.slashPicker = nil
		return
	}
	trimmed := strings.TrimLeft(value, " \t")
	if !strings.HasPrefix(trimmed, "/") {
		m.slashPicker = nil
		return
	}
	if m.slashPicker == nil {
		m.slashPicker = newSlashPickerState()
	}
	m.slashPicker.setFilter(trimmed)
}
```

- [ ] **Step 3: Call lifecycle after textarea updates**

Find the block ending around line 1159 (right after `if m.textareaExpanded && m.textarea.Value() == "" { ... m.textarea.SetHeight(1) }`). Add immediately after that block:

```go
	m.updateSlashPicker()
```

So the surrounding code looks like:

```go
	// Auto-collapse expanded textarea when it becomes empty.
	if m.textareaExpanded && m.textarea.Value() == "" {
		m.textareaExpanded = false
		m.textarea.SetHeight(1)
	}
	m.updateSlashPicker()
	if !m.textarea.Focused() || m.streamActive || m.pendingPermission != nil {
		m.pendingSubmitNewline = false
	}
```

- [ ] **Step 4: Build and run existing tests**

```
go build ./...
go test ./internal/chat/...
```

Expected: build succeeds; all existing tests pass.

- [ ] **Step 5: Commit**

```
git add internal/chat/tui.go
git -c user.name="James Zhu" -c user.email="zhujian0805@gmail.com" commit -m "feat(tui): track slash picker lifecycle from textarea contents"
```

---

### Task 6: TUI integration test for picker opening + filtering

**Files:**
- Create: `internal/chat/tui_slash_picker_test.go`

- [ ] **Step 1: Write the failing test**

Create `internal/chat/tui_slash_picker_test.go`:

```go
package chat

import (
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
}

func namesOf(cs []slashCommand) []string {
	out := make([]string, len(cs))
	for i, c := range cs {
		out[i] = c.Name
	}
	return out
}
```

- [ ] **Step 2: Run tests**

```
go test ./internal/chat/ -run TestSlashPicker
```

Expected: opening and filtering tests PASS; closing test may PASS already because `updateSlashPicker` handles empty input. If any fail, fix and re-run.

- [ ] **Step 3: Commit**

```
git add internal/chat/tui_slash_picker_test.go
git -c user.name="James Zhu" -c user.email="zhujian0805@gmail.com" commit -m "test(tui): slash picker open/filter/close lifecycle"
```

---

### Task 7: Key handling — Up/Down/Enter/Esc while picker is open

**Files:**
- Modify: `internal/chat/tui.go`

- [ ] **Step 1: Write the failing test**

Append to `internal/chat/tui_slash_picker_test.go`:

```go
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

func TestSlashPickerEnterOnArglessSubmits(t *testing.T) {
	m := newTestTUIModel()
	m = typeRune(t, m, '/')
	// Move selection until we land on /models (arg-less).
	for i := 0; i < len(m.slashPicker.filtered); i++ {
		if name, _ := m.slashPicker.selected(); name.Name == "/models" {
			break
		}
		out, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
		m = out.(chatTUIModel)
	}
	sel, _ := m.slashPicker.selected()
	if sel.Name != "/models" {
		t.Fatalf("expected to land on /models, got %q", sel.Name)
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

func TestSlashPickerEnterOnArgTakingFillsInput(t *testing.T) {
	m := newTestTUIModel()
	m = typeRune(t, m, '/')
	// Land on /model (TakesArgs=true).
	for i := 0; i < len(m.slashPicker.filtered); i++ {
		if sel, _ := m.slashPicker.selected(); sel.Name == "/model" {
			break
		}
		out, _ := m.Update(tea.KeyMsg{Type: tea.KeyDown})
		m = out.(chatTUIModel)
	}
	sel, _ := m.slashPicker.selected()
	if sel.Name != "/model" {
		t.Fatalf("expected to land on /model, got %q", sel.Name)
	}
	out, _ := m.Update(tea.KeyMsg{Type: tea.KeyEnter})
	m = out.(chatTUIModel)
	if m.slashPicker != nil {
		t.Errorf("picker should close after Enter on arg-taking command")
	}
	if got := m.textarea.Value(); got != "/model " {
		t.Errorf("textarea should be %q, got %q", "/model ", got)
	}
}
```

- [ ] **Step 2: Run tests to verify they fail**

```
go test ./internal/chat/ -run TestSlashPicker
```

Expected: the four new tests FAIL because key handling is not yet wired.

- [ ] **Step 3: Wire key handling in `Update`**

In `internal/chat/tui.go`, locate the `case tea.KeyMsg:` block inside `Update` (around line 664). Immediately AFTER the existing `if m.picker != nil { ... }` block (which ends near line 763, just before `switch msg.Type {` around line 764), insert a new block that handles the slash picker:

```go
		if m.slashPicker != nil {
			switch msg.Type {
			case tea.KeyEscape:
				m.slashPicker = nil
				return m, nil
			case tea.KeyUp, tea.KeyCtrlP:
				m.slashPicker.moveSelection(-1, slashPickerVisible)
				return m, nil
			case tea.KeyDown, tea.KeyCtrlN:
				m.slashPicker.moveSelection(1, slashPickerVisible)
				return m, nil
			case tea.KeyEnter:
				sel, ok := m.slashPicker.selected()
				if !ok {
					return m, nil
				}
				m.slashPicker = nil
				if sel.TakesArgs {
					m.applyTextareaValue(sel.Name + " ")
					m.textarea.CursorEnd()
					return m, nil
				}
				m.applyTextareaValue(sel.Name)
				return m.submitTextareaInput()
			}
			// Fall through: any other key edits the textarea and the
			// post-update lifecycle step will recompute the filter.
		}
```

Then, at the top of `internal/chat/tui.go` near the other constants (around line 113), add:

```go
const slashPickerVisible = 10
```

`applyTextareaValue` already exists in `tui.go`. If you cannot find it, search:

```
go doc -all ./internal/chat | Select-String "applyTextareaValue"
```

It is the existing helper used elsewhere (line ~767) to replace the textarea contents.

- [ ] **Step 4: Also gate Up/Down history navigation when picker is open**

In `tui.go` around line 855 (`case tea.KeyUp, tea.KeyCtrlP:`) and line 866 (`case tea.KeyDown, tea.KeyCtrlN:`), the slash-picker block above already returns before reaching these, so no further change is needed here. Verify by inspection that the new block uses `return m, nil` on every key it handles.

- [ ] **Step 5: Run tests**

```
go test ./internal/chat/...
```

Expected: all tests PASS (the four new ones plus all existing).

- [ ] **Step 6: Commit**

```
git add internal/chat/tui.go internal/chat/tui_slash_picker_test.go
git -c user.name="James Zhu" -c user.email="zhujian0805@gmail.com" commit -m "feat(tui): handle Up/Down/Enter/Esc in slash picker"
```

---

### Task 8: Render the slash-picker overlay

**Files:**
- Modify: `internal/chat/tui.go`

- [ ] **Step 1: Add render helper**

Append near `renderTextarea` (around line 1213) in `tui.go`:

```go
func (m chatTUIModel) renderSlashPicker() string {
	if m.slashPicker == nil {
		return ""
	}
	muted := lipgloss.NewStyle().Foreground(lipgloss.Color("8"))
	selectedStyle := lipgloss.NewStyle().Bold(true).Foreground(lipgloss.Color("0")).Background(lipgloss.Color("12")).Padding(0, 1)
	normalStyle := lipgloss.NewStyle().Padding(0, 1)

	width := tuiMax(40, m.transcriptBlockMaxWidth())
	visible := slashPickerVisible
	if visible > len(m.slashPicker.filtered) {
		visible = len(m.slashPicker.filtered)
	}
	if visible < 1 && len(m.slashPicker.filtered) == 0 {
		visible = 1
	}
	start := m.slashPicker.scrollOffset
	end := start + visible
	if end > len(m.slashPicker.filtered) {
		end = len(m.slashPicker.filtered)
	}

	var rows strings.Builder
	if len(m.slashPicker.filtered) == 0 {
		rows.WriteString(muted.Render("  No matching commands"))
	} else {
		for i := start; i < end; i++ {
			c := m.slashPicker.filtered[i]
			label := fmt.Sprintf("%-14s %s", c.Name, muted.Render(c.Summary))
			if i == m.slashPicker.selectedIdx {
				rows.WriteString(selectedStyle.Width(width - 4).Render(label))
			} else {
				rows.WriteString(normalStyle.Render(label))
			}
			if i < end-1 {
				rows.WriteString("\n")
			}
		}
	}

	header := lipgloss.NewStyle().Bold(true).Render("Commands")
	hint := muted.Render("Enter selects • Esc closes • ↑↓ navigate")
	body := lipgloss.JoinVertical(lipgloss.Left, header+"  "+hint, rows.String())
	return lipgloss.NewStyle().Border(lipgloss.RoundedBorder()).BorderForeground(lipgloss.Color("12")).Padding(0, 1).Width(width - 2).Render(body)
}
```

- [ ] **Step 2: Mount the overlay in `View`**

In `View()` around line 1182, change:

```go
	main.WriteString(m.renderTextarea())
```

to:

```go
	if overlay := m.renderSlashPicker(); overlay != "" {
		main.WriteString(overlay)
		main.WriteString("\n")
	}
	main.WriteString(m.renderTextarea())
```

- [ ] **Step 3: Build and run all tests**

```
go build ./...
go test ./internal/chat/...
```

Expected: PASS.

- [ ] **Step 4: Add a smoke test that View() renders the overlay**

Append to `internal/chat/tui_slash_picker_test.go`:

```go
import "strings"

func TestSlashPickerViewIncludesCommandRow(t *testing.T) {
	m := newTestTUIModel()
	m = typeRune(t, m, '/')
	out := m.View()
	if !strings.Contains(out, "Commands") {
		t.Errorf("View() missing header; got:\n%s", out)
	}
	if !strings.Contains(out, "/help") {
		t.Errorf("View() should list /help; got:\n%s", out)
	}
}
```

(If `strings` is already imported, do not add a second import.)

- [ ] **Step 5: Run the new test**

```
go test ./internal/chat/ -run TestSlashPickerViewIncludesCommandRow
```

Expected: PASS.

- [ ] **Step 6: Commit**

```
git add internal/chat/tui.go internal/chat/tui_slash_picker_test.go
git -c user.name="James Zhu" -c user.email="zhujian0805@gmail.com" commit -m "feat(tui): render slash picker overlay above input"
```

---

### Task 9: Rewrite `/help` to use registry

**Files:**
- Modify: `internal/chat/tui.go`

- [ ] **Step 1: Replace the hardcoded help string**

In `handleSlash` (around line 2725), replace:

```go
	case "/help", "?":
		add(m.renderMD("**Commands:**\n\n- `/help` or `?` — show this help\n- `/new [title]` — start a new session\n- `/sessions` — browse and resume a previous session\n- `/session` — show current session info\n- `/mode` — show current mode\n- `/mode <chat|agent>` — switch mode\n- `/apishape` — show the fixed agent API request shape\n- `/apishape <anthropic>` — keep the agent API request shape on `/v1/messages`\n- `/permissions` — toggle autopilot (auto-approve tool calls)\n- `/model` — show current model\n- `/model <id>` — switch model\n- `/agent` — show the fixed google-adk backend\n- `/agent <google-adk>` — keep the agent backend on google-adk\n- `/max-turns [1-100]` — show or set max agent turns (default 25)\n- `/models` — open model picker\n- `/spec` — show spec-driven workflow help\n- `/spec init <title>` — create a new spec directory\n- `/spec status` — list all specs and their artifact status\n- `/clear` or `/cls` — clear the screen\n- `/quit` or `/exit` — quit\n\n**Keyboard shortcuts:**\n\n- `Shift+Tab` — toggle autopilot (auto-approve tool calls)\n- `↑`/`↓` — focus expandable tool results (when input is empty)\n- `Space` — expand/collapse the focused tool result\n- `Esc` — cancel current running job\n- The right-hand panel always shows permission and session status\n"))
		return m, nil
```

with:

```go
	case "/help", "?":
		add(m.renderMD(renderSlashHelp(slashCommands())))
		return m, nil
```

- [ ] **Step 2: Build and run all tests**

```
go build ./...
go test ./internal/chat/...
```

Expected: PASS.

- [ ] **Step 3: Commit**

```
git add internal/chat/tui.go
git -c user.name="James Zhu" -c user.email="zhujian0805@gmail.com" commit -m "refactor(tui): render /help from slash-command registry"
```

---

### Task 10: Manual smoke check + final verification

**Files:** none

- [ ] **Step 1: Build the binary**

```
go build -o build/omnicode.exe ./cmd/omnicode
```

Expected: build succeeds.

- [ ] **Step 2: Quick interactive check (optional, requires running server)**

If a local OmniLLM server is reachable, run:

```
./build/omnicode.exe
```

Then verify:
- Typing `/` opens the picker showing all commands.
- Typing `mo` filters to `/model`, `/models`, `/mode`.
- `Down`+`Enter` on `/models` opens the model picker (existing behavior).
- `Down`+`Enter` on `/model` leaves textarea at `/model ` with cursor at end.
- `Esc` closes the picker, leaving the typed text.
- Backspacing the leading `/` closes the picker.

If no server is available, skip this step — automated tests already cover state transitions.

- [ ] **Step 3: Run the full test suite once more**

```
go test ./...
```

Expected: PASS for `internal/chat/...`; any unrelated failures pre-existed and are out of scope for this plan.

- [ ] **Step 4: Final commit (if any docs were updated during smoke check)**

If no changes were needed, skip.

---

## Self-Review

- Spec coverage:
  - Trigger on `/` typed → Tasks 5, 6.
  - Live fuzzy filtering as user types → Tasks 2, 6.
  - Up/Down/Enter/Esc handling, including arg-taking vs arg-less Enter behavior → Task 7.
  - Picker closes when `/` deleted or input is multi-line → Tasks 5, 6, plus multi-line guard in `updateSlashPicker`.
  - Overlay rendering above input → Task 8.
  - `/help` rendered from the same registry → Tasks 3, 9.
  - Tests for registry, filter, state, lifecycle, key handling, rendering → Tasks 1, 2, 3, 4, 6, 7, 8.
- Placeholder scan: no `TBD`/`TODO`/"add error handling" prose. Each step contains explicit code or commands.
- Type consistency: `slashCommand`, `slashPickerState`, `newSlashPickerState`, `setFilter`, `moveSelection`, `selected`, `slashPickerVisible`, `fuzzySlashFilter`, `renderSlashHelp`, `updateSlashPicker`, `renderSlashPicker` — names match across tasks.

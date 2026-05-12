# OmniCode TUI: Dynamic Slash-Command Picker

Date: 2026-05-12

## Goal

When the user types `/` in the OmniCode TUI input, immediately open a
command picker that lists all available slash commands. As the user
continues typing, the list filters dynamically; the user can navigate
with arrow keys and execute the selected command with Enter.

This brings slash commands to feature parity with the existing model
picker (`/models`) by providing live discoverability and selection,
instead of requiring users to type the full command name from memory.

## Non-Goals

- Adding new slash commands or changing existing command semantics.
- Discovery of dynamic/runtime-registered slash commands. The picker
  reflects the static built-in catalog.
- Argument autocompletion beyond the command name itself (e.g. no
  `/model <id>` value suggestions in this iteration).
- Changing the `!shell` or plain-chat input behavior.

## User Experience

1. The textarea is empty. The user types `/`.
2. A picker overlay opens above (or anchored to) the input area listing
   all built-in slash commands. The textarea still contains `/` and
   remains focused.
3. The user types more characters (e.g. `mo`). The textarea now reads
   `/mo`. The picker shows only commands matching `mo` — for example
   `/model`, `/models`, `/mode` — sorted by match quality.
4. The user moves the selection with `Up`/`Down`. `Enter` accepts the
   highlighted command:
   - If the command takes no arguments (e.g. `/help`, `/clear`,
     `/models`, `/sessions`, `/quit`), the textarea is replaced with
     the canonical command name and submitted immediately.
   - If the command takes arguments (e.g. `/model <id>`, `/mode
     <chat|agent>`, `/max-turns <n>`, `/new <title>`, `/spec <sub>`),
     the textarea is replaced with `<command> ` (trailing space) and
     the cursor is placed at the end so the user can type the
     arguments. The picker closes.
5. `Esc` closes the picker and leaves the typed input untouched, so
   the user can keep editing or submit manually.
6. If the user deletes the leading `/` (or empties the textarea) the
   picker closes automatically.
7. When the input does not start with `/`, the picker never opens, and
   key handling (history navigation, multi-line input, etc.) behaves
   exactly as today.

## Architecture

### Slash command registry

A new file `internal/chat/slash_commands.go` defines the static
catalog:

```go
type slashCommand struct {
    Name      string   // canonical name, e.g. "/models"
    Aliases   []string // optional, e.g. ["/cls"] for "/clear"
    Summary   string   // one-line description for picker and /help
    TakesArgs bool     // true if the command accepts arguments
}

func slashCommands() []slashCommand { /* static list */ }
```

The list contains every command currently handled by
`chatTUIModel.handleSlash` (see `internal/chat/tui.go`):
`/help`, `/new`, `/sessions`, `/session`, `/mode`, `/apishape`,
`/permissions`, `/model`, `/agent`, `/max-turns`, `/models`, `/spec`,
`/clear`, `/quit`, plus aliases (`?`, `/exit`, `/cls`, `/api-shape`).

The registry becomes the single source of truth:

- The picker reads it to enumerate and filter commands.
- `/help` is rewritten to render its output from this list, removing
  the hand-maintained markdown string in `handleSlash`.

### Picker state

In `internal/chat/tui.go`:

```go
type slashPickerState struct {
    all          []slashCommand
    filtered     []slashCommand
    filter       string
    selectedIdx  int
    scrollOffset int
}

func newSlashPickerState() *slashPickerState
func (s *slashPickerState) setFilter(filter string)
func (s *slashPickerState) moveSelection(delta int, visible int)
func (s *slashPickerState) selected() (slashCommand, bool)
```

`setFilter` calls `fuzzySlashFilter(all, filter)` and clamps the
selection. Scoring follows the same prefix / substring / subsequence
shape as `fuzzyScore` already used by the model picker.

The `chatTUIModel` gains one new field:

```go
slashPicker *slashPickerState
```

### Trigger and lifecycle

The picker is driven entirely by the textarea contents. After each
textarea update in `Update(...)`:

1. Read `value := m.textarea.Value()`.
2. Determine `firstLine := value` up to the first newline.
3. If `firstLine` starts with `/`, ensure `m.slashPicker` exists and
   call `setFilter(firstLine)` with the literal text (including the
   leading `/`). Matching is done against command names, so passing
   `/mo` matches `/model`, `/models`, `/mode`.
4. Otherwise (no leading `/`, or multi-line input), set
   `m.slashPicker = nil`.

This means the picker is purely a function of textarea state and
needs no special opening keystroke — typing `/` opens it, deleting
the `/` closes it.

### Key handling

In the existing key switch in `Update`:

- When `m.slashPicker != nil`:
  - `KeyUp`/`KeyDown` (or `KeyCtrlP`/`KeyCtrlN`) move the picker
    selection instead of cycling prompt history.
  - `KeyEnter` accepts the highlighted command:
    - If `TakesArgs == false`: replace textarea contents with
      `cmd.Name`, then run the normal submit path.
    - If `TakesArgs == true`: replace textarea contents with
      `cmd.Name + " "`, position the cursor at end, close the picker.
  - `KeyEscape` closes the picker (`m.slashPicker = nil`). The
    textarea contents are preserved.
  - All other keys (printable characters, backspace, etc.) flow
    through to the textarea update path; the post-update lifecycle
    step recomputes the filter.
- When `m.slashPicker == nil`: existing behavior is unchanged.

The history-search mode (`Ctrl-R`) takes precedence over the picker:
entering history-search closes any open picker.

### Rendering

The picker is rendered above the textarea (in `View()`), reusing
`lipgloss` styling already established in `tui.go` so the overlay
visually matches the model picker:

- Header: `Commands` plus the live filter substring.
- Each row: command name (bold) + summary (muted) on one line, with
  the highlighted row inverted.
- Footer: `Enter selects • Esc closes • ↑↓ navigate` and
  `Showing X of Y` counts.
- When no commands match: `No matching commands` muted message.

Height is bounded similarly to the model picker (e.g.
`min(10, len(filtered))`) and scrolls when the selection moves out of
the visible window.

### `/help` rendered from the registry

`handleSlash`'s `/help` case is rewritten:

```go
case "/help", "?":
    add(m.renderMD(renderSlashHelp(slashCommands())))
    return m, nil
```

`renderSlashHelp` produces a markdown list from the same registry so
the help output and the picker can never drift.

## Data Flow

```
keystroke ──▶ textarea.Update ──▶ value()
                                      │
                                      ▼
                            firstLine starts with "/"?
                            ┌────────┴────────┐
                           yes                no
                            │                  │
                            ▼                  ▼
                 ensure slashPicker      slashPicker = nil
                 setFilter(firstLine)
                            │
                            ▼
                  View() renders picker overlay above input

Enter (picker open):
   selected cmd ──▶ rewrite textarea ──▶ submit or wait for args
Esc (picker open):
   slashPicker = nil (textarea untouched)
```

## Error Handling

- An empty filtered list does not crash: `selected()` returns
  `(_, false)` and `Enter` becomes a no-op.
- If the user submits with the picker open but no entries match (e.g.
  typed `/zzzz`), the submission falls through to the existing
  `handleSlash` path which already produces an
  `"Unknown command: ..."` error transcript.
- `Esc` always closes the picker safely; no key event leaks into the
  textarea.

## Testing

New tests in `internal/chat`:

1. `slash_commands_test.go`:
   - Catalog has no duplicate names or aliases.
   - Each entry has a non-empty `Summary`.
   - `fuzzySlashFilter` returns expected matches and ordering for
     representative queries: `/m`, `/mo`, `/mod`, `/x`, `?`.
2. `slash_picker_test.go`:
   - `setFilter` clamps `selectedIdx` correctly when the result set
     shrinks.
   - `moveSelection` honors visible window boundaries.
   - `selected()` returns false for an empty filtered list.
3. `tui_slash_picker_test.go` (driven via Bubble Tea messages on
   `chatTUIModel`):
   - Typing `/` opens the picker with all commands.
   - Typing `/mo` filters to `/model`, `/models`, `/mode`.
   - `Down`+`Enter` on an arg-less command (`/models`) submits the
     command.
   - `Down`+`Enter` on an arg-taking command (`/model`) rewrites the
     textarea to `/model ` and closes the picker without submitting.
   - `Esc` closes the picker and keeps `/mo` in the textarea.
   - Deleting back to empty closes the picker.

## File-Level Changes

- `internal/chat/slash_commands.go` (new): registry + filter +
  help-renderer.
- `internal/chat/tui.go`: add `slashPickerState`, `chatTUIModel`
  field, trigger logic in `Update`, key handling, picker rendering
  in `View`, rewrite `/help` case.
- `internal/chat/slash_commands_test.go` (new).
- `internal/chat/slash_picker_test.go` (new).
- `internal/chat/tui_slash_picker_test.go` (new).

No external dependencies are added.

## Risks and Open Questions

- The picker must not interfere with the existing history navigation
  (`Up`/`Down` cycle previous prompts when the picker is closed).
  Resolved by gating history nav on `m.slashPicker == nil`.
- Multi-line input that happens to start with `/` could be ambiguous.
  Resolved by closing the picker as soon as the input contains a
  newline.
- The `?` alias for `/help` already short-circuits in
  `submitTextareaInput`. The picker treats `?` as a synonym of
  `/help` in the registry; typing `?` alone still triggers the help
  flow on submit even if the picker hasn't opened.

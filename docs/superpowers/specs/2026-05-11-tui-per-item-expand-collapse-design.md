# TUI per-item expand/collapse for tool results

**Date:** 2026-05-11
**Status:** Approved (design)
**Area:** `internal/chat/tui.go` (omnicode TUI)

## Goal

Replace the current global `Ctrl+O` "expand all tool results" shortcut with a
per-item expand/collapse interaction inspired by the `crush` TUI: focus a
truncated tool-result entry and press `space` (or click it without dragging) to
toggle just that entry. The collapsed-state hint mirrors crush's affordance:

```
… (N lines hidden) [click or space to expand]
```

## Scope

In scope:
- `transcriptToolResult` entries only. Existing `toolResultMaxLines = 10`
  threshold preserved.
- Keyboard focus movement among collapsible entries.
- Mouse click-without-drag toggle.
- Removing `Ctrl+O` binding and the now-unused `toggleAllExpandableEntries`
  helper.

Out of scope:
- Collapsing assistant, user, thinking, or info entries.
- Persisting expanded state across sessions.
- Any animation / progressive reveal.

## Files touched

All edits land in `internal/chat/tui.go` (the TUI is a single large file
today; this change does not justify splitting it).

## Interaction design

### Hint text (in `renderToolResultSection`)

State combinations:

| Expanded | Focused | Hint shown                                          |
| -------- | ------- | --------------------------------------------------- |
| no       | yes     | `… (N lines hidden) [click or space to expand]`     |
| no       | no      | `… (N lines hidden)`                                |
| yes      | yes     | `▾ (click or space to collapse)`                    |
| yes      | no      | *(no hint)*                                         |

Showing the actionable instruction only on the focused entry keeps the
transcript clean and matches crush's screenshot, where the hint sits on the
focused/hovered item.

### Focus model

- `hoveredEntry` is the single source of truth for "which entry is focused".
- Mouse motion sets it (today's behavior, unchanged).
- New: when the textarea is empty, `↑` and `↓` cycle `hoveredEntry` only among
  entries that are collapsible tool results with overflow
  (`len(strings.Split(content,"\n")) > toolResultMaxLines`). Wraps at top and
  bottom. If no eligible entries exist, the keys fall through to the viewport
  scroll behavior they have today.
- The textarea's own `↑`/`↓` history-navigation behavior (when textarea is
  non-empty or actively in history-navigation mode) is preserved by gating the
  new handler on `m.textarea.Value() == ""` AND no active history navigation.

### `space` toggle

- When textarea is empty AND `hoveredEntry` points at a `transcriptToolResult`
  with overflow → toggle `expandedEntries[id]`, then `syncViewport()`.
- Otherwise `space` flows through to the textarea as today (so typing a space
  in the input still works).

### Click-without-drag toggle

- On `MouseButtonLeft + MouseActionPress`, record `clickStartX`,
  `clickStartY` (extending today's selection-tracking state).
- On `MouseButtonLeft + MouseActionRelease`:
  - If `(X,Y) == (clickStartX, clickStartY)` (zero drag) AND the entry under
    the cursor is a `transcriptToolResult` with overflow → toggle that entry's
    `expandedEntries`, abort the pending selection start (so no zero-length
    selection is finalized), and return.
  - Otherwise behave as today (finalize selection, etc.).
- Any drag (≥ 1 cell of movement between press and release) preserves today's
  text-selection behavior — clicks that move are never treated as toggles.

### Removed: `Ctrl+O` and `toggleAllExpandableEntries`

- Delete the `Ctrl+O` case in the keybinding switch.
- Delete the `toggleAllExpandableEntries` method (no remaining callers).
- Remove `Ctrl+O expand/collapse tool results` from the inline help status
  line; add `Space expand focused tool result` in its place.

## Backward compatibility

- `expandedEntries` map and existing `expanded` parameter to
  `renderToolResultSection` are unchanged in shape; only the hint string and
  the trigger paths change.
- No persisted state, no config keys, no command-line flags affected.
- Users who relied on `Ctrl+O` lose the global toggle; the per-item flow is
  the replacement. Documented in the help line.

## Testing

Add table-driven tests in `internal/chat/chat_test.go`:

1. **Hint rendering** — `renderToolResultSection` produces the correct hint
   for each of the four (expanded × focused) states, plus the no-overflow
   case (no hint at all).
2. **Focus cycling** — given a transcript with three tool-result entries
   (two overflowing, one short) and a non-tool entry, `↑`/`↓` with empty
   textarea visit only the two overflowing entries and wrap correctly.
3. **`space` toggle**
   - With focus on an overflowing tool result and empty textarea: `space`
     flips `expandedEntries[id]`.
   - With non-empty textarea: `space` is consumed by the textarea
     (input contains the space).
   - With empty textarea but `hoveredEntry == -1`: `space` falls through
     to the textarea.
4. **Click-without-drag**
   - Simulated press + release at identical coords on an overflowing tool
     result toggles `expandedEntries[id]` and does not start a selection.
   - Press + motion + release on the same entry does NOT toggle and starts /
     finalizes a selection as today.

## Implementation order

1. Add `clickStartX/Y` tracking and click-without-drag detection in
   `handleMouseEvent`.
2. Update `renderToolResultSection` to the four-state hint table.
3. Add `↑`/`↓` keyboard focus cycling among eligible entries (gated on empty
   textarea + no active history navigation).
4. Add `space` toggle handler.
5. Remove `Ctrl+O` keybinding, delete `toggleAllExpandableEntries`, update
   help/status lines.
6. Add tests from the section above; run `go test ./internal/chat/...`.

## Risks

- **Mouse hover updates focus mid-stream.** When the assistant is streaming,
  mouse motion can shift `hoveredEntry` and therefore which entry shows the
  expand hint. This matches today's hover behavior and is acceptable.
- **`↑`/`↓` collision with viewport scroll.** Today, when no input is
  focused, these keys may scroll the viewport. The new behavior takes
  precedence only when at least one collapsible tool result exists; otherwise
  it falls through. Verify in test 2.
- **Terminals that don't report mouse motion.** Click-without-drag still
  works (press and release coords are reported). Hover-driven focus does not,
  so users in such terminals must use `↑`/`↓` to move focus before pressing
  `space`. Acceptable degradation.

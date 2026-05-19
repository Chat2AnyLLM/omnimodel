# CLI Output Formatting Design

**Date:** 2026-05-19  
**Scope:** `internal/commands/output.go` and all affected command files  
**Goal:** Improve human-readability of all `omnillm` terminal output using a consistent two-layer layout.

---

## Problem

Current output is hard to scan:
- Key/value lines use inconsistent alignment with `%-16s` padding that looks loose at all terminal widths
- Tables have no borders — just a header, a line of dashes, and rows separated only by whitespace
- Status sections mix free-form `printCheck` icon lines with key/value lines, making structure unpredictable
- Wide commands like `provider list` produce unreadable rows when names/IDs are long

---

## Chosen Direction: Summary + Clean Tables (Option B)

### Two-Layer Layout

Every command output falls into one of two layers:

**Layer 1 — Compact summary facts**
- Used for top-level scalar fields (status, uptime, counts, flags)
- Printed as aligned 2-column `Key  Value` lines, no border, padded consistently
- Consistent left-column width per section (width determined by longest key in that section)

**Layer 2 — Framed tables for collections**
- Used for any list of items (providers, services, models, virtual models, auth flow fields)
- Unicode box-drawing borders: `┌─┬─┐`, `├─┼─┤`, `└─┴─┘`, with `│` column separators
- Column widths auto-sized to content
- Long string columns truncated to a configurable max width with `…` suffix (default 40 chars for NAME, 32 for ID)

### Section Headers

Same style as today: `Title\n──────` underline, but underline length should match title length exactly (already correct).

---

## Rendering Changes

### `output.go` — shared helpers

#### `Table.Render`
Replace the current headerline-only renderer with a full bordered table:
```
┌──────┬───────┐
│ NAME │ VALUE │
├──────┼───────┤
│ foo  │ bar   │
└──────┴───────┘
```
- Pad cells with one space on each side
- Left-align all columns by default
- Add `Table.SetMaxWidth(col int, max int)` to configure truncation per column
- Add `Table.Truncate(s string, max int) string` helper (private): if `len(s) > max`, return `s[:max-1] + "…"`

#### `PrintKeyValue`
Replace the `%-16s` format with dynamic padding:
- `PrintKeyValueSection(w io.Writer, pairs [][2]string)` — takes a slice of key/value pairs, computes `maxKeyLen` across the batch, then prints all rows with `%-<maxKeyLen>s  %v` alignment
- Keep the old `PrintKeyValue` signature for backward compatibility but implement it as a single-pair call to the new function

#### No changes to `PrintSection` or `PrintEmpty`

---

## Command-by-Command Changes

### `omnillm status`

**Before (rough):**
```
Status:          healthy
Uptime:          6m48s
...
NAME                                         ID
───────────────────────────────────────────────
GitHub Copilot (James Zhu · jzhu+abk)   copilot-jzhu-abk
```

**After:**
```
Server status
─────────────
  Status         healthy
  Uptime         6m48s
  Model count    273
  Manual approve no
  Rate limit     none

Active providers
────────────────
┌───────────────────────────────────────┬─────────────────┐
│ Name                                  │ ID              │
├───────────────────────────────────────┼─────────────────┤
│ GitHub Copilot (James Zhu · jzhu+abk) │ copilot-jzhu-ab │
│ Kimi (global)                         │ kimi            │
└───────────────────────────────────────┴─────────────────┘

Services
────────
┌───────────┬──────────────────────┐
│ Service   │ Status               │
├───────────┼──────────────────────┤
│ API       │ running              │
│ Database  │ connected            │
│ Providers │ 19 total, 3 active   │
└───────────┴──────────────────────┘
```

Changes:
- Replace `PrintKeyValue` loops with `PrintKeyValueSection` batching
- Replace `NewTable("NAME","ID").Render()` calls with bordered render automatically

### `omnillm doctor`

**Before:**
```
  ✓  Config directory:      C:\...\omnillm
  ✓  Database:              C:\...\database.sqlite (19087360 bytes)
  ✗  Status:                healthy
Uptime:          39s
```

**After:**
- Status check rows: `printCheck` kept but right-align label column consistently using section-scoped max width
- Inline Uptime/Models under "Server status" printed via `PrintKeyValueSection` (not mixed with check rows)
- The "Everything looks good. ✓" footer becomes a visually distinct line (prefix with blank line, bold or uppercase)

Changes:
- Collect all check-rows per section and render after computing max label width
- Move scalar fields (Uptime, Models) into their own `PrintKeyValueSection` block, separate from the check rows
- Fix `printCheck` alignment: use consistent padding computed per section, not a fixed `%-22s`

### `omnillm provider list`

Columns: `ID`, `TYPE`, `NAME`, `AUTH`, `ACTIVE`, `MODELS`

- Set `MaxWidth` for `ID` → 32 chars, `NAME` → 36 chars
- `AUTH`, `ACTIVE`, `MODELS` are short — no truncation
- Bordered table, same as all others

### `omnillm model list`, `omnillm virtualmodel list`, `omnillm config list`

Same bordered table treatment. Column-specific max widths where names/IDs are unbounded.

### Detail views (`virtualmodel get`, `provider usage`, `status auth`)

Currently rendered as loose `PrintKeyValue` rows. Switch to a 2-column bordered table:
```
┌─────────────────┬───────────────────────────────┐
│ Field           │ Value                         │
├─────────────────┼───────────────────────────────┤
│ Name            │ My Virtual Model              │
│ Strategy        │ round-robin                   │
│ API Shape       │ openai                        │
│ Enabled         │ yes                           │
└─────────────────┴───────────────────────────────┘
```

---

## Shared Formatting Rules

| Concern         | Rule                                                                        |
|-----------------|-----------------------------------------------------------------------------|
| Borders         | Unicode box-drawing (`┌┐└┘├┤┬┴┼─│`), one space padding inside each cell    |
| Column widths   | Auto-sized to `max(header_len, max_content_len)`, capped by per-column max  |
| Truncation      | Truncate to `max-1` chars + `…` when content exceeds column max             |
| Section spacing | One blank line before each section header                                   |
| Key/value align | Batch-compute max key width per section; pad all keys in the batch uniformly |
| Boolean display | `yes`/`no` not `true`/`false`                                               |
| Empty tables    | Use existing `PrintEmpty` helper, no change                                 |

---

## Implementation Boundaries

All changes are in `internal/commands/`. No server-side or API changes.

Files that will change:
- `output.go` — core rendering helpers
- `output_test.go` — update existing tests, add bordered table and truncation tests
- `status.go` — use new helpers
- `doctor.go` — fix check-row alignment, split scalar fields from checks
- `provider.go` — add column max widths
- `model.go` — add column max widths
- `virtualmodel.go` — switch detail view to bordered table
- `check-usage.go` — use bordered tables
- `config.go` — use bordered tables

---

## Testing

- `TestTableRenderBordered` — verify bordered output includes `┌`, `┤`, `└` characters and correct column count
- `TestTableTruncation` — verify long values are truncated with `…` at correct length
- `TestPrintKeyValueSection` — verify batch alignment uses max key width
- Existing tests updated where output shape changes (assertions on `──────` underlines and column headers can stay; assertions on specific spacing may need adjustment)

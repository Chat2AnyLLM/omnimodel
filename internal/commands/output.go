package commands

import (
	"fmt"
	"io"
	"strings"
)

// Table renders a bordered, aligned table to any io.Writer.
type Table struct {
	headers   []string
	rows      [][]string
	widths    []int
	maxWidths map[int]int
}

// NewTable creates a Table with the given column headers.
func NewTable(headers ...string) *Table {
	widths := make([]int, len(headers))
	for i, h := range headers {
		widths[i] = len([]rune(h))
	}
	return &Table{
		headers:   headers,
		widths:    widths,
		maxWidths: map[int]int{},
	}
}

// SetMaxWidth limits column col to max visible characters; longer values are
// truncated with a trailing "…".
func (t *Table) SetMaxWidth(col, max int) {
	if col >= 0 && col < len(t.headers) {
		t.maxWidths[col] = max
		if max < t.widths[col] {
			t.widths[col] = max
		}
	}
}

// AddRow appends a row, applying any column max-width constraints.
func (t *Table) AddRow(columns ...string) {
	row := make([]string, len(t.headers))
	copy(row, columns)
	for i := range row {
		if max, ok := t.maxWidths[i]; ok {
			row[i] = truncate(row[i], max)
		}
		if len([]rune(row[i])) > t.widths[i] {
			t.widths[i] = len([]rune(row[i]))
		}
	}
	t.rows = append(t.rows, row)
}

// Render writes the table with Unicode box-drawing borders to w.
func (t *Table) Render(w io.Writer) error {
	if len(t.headers) == 0 {
		return nil
	}

	sep := t.buildSep("┌", "┬", "┐")
	mid := t.buildSep("├", "┼", "┤")
	bot := t.buildSep("└", "┴", "┘")

	if _, err := fmt.Fprintln(w, sep); err != nil {
		return err
	}
	if _, err := fmt.Fprintln(w, t.buildRow(t.headers)); err != nil {
		return err
	}
	if len(t.rows) > 0 {
		if _, err := fmt.Fprintln(w, mid); err != nil {
			return err
		}
		for _, row := range t.rows {
			if _, err := fmt.Fprintln(w, t.buildRow(row)); err != nil {
				return err
			}
		}
	}
	_, err := fmt.Fprintln(w, bot)
	return err
}

func (t *Table) buildSep(left, mid, right string) string {
	var sb strings.Builder
	sb.WriteString(left)
	for i, w := range t.widths {
		sb.WriteString(strings.Repeat("─", w+2)) // +2 for padding spaces
		if i < len(t.widths)-1 {
			sb.WriteString(mid)
		}
	}
	sb.WriteString(right)
	return sb.String()
}

func (t *Table) buildRow(cols []string) string {
	var sb strings.Builder
	sb.WriteString("│")
	for i, w := range t.widths {
		val := ""
		if i < len(cols) {
			val = cols[i]
		}
		sb.WriteString(" ")
		sb.WriteString(padRightRunes(val, w))
		sb.WriteString(" │")
	}
	return sb.String()
}

// padRightRunes pads s with spaces on the right so the visible rune count reaches w.
func padRightRunes(s string, w int) string {
	r := []rune(s)
	if len(r) >= w {
		return s
	}
	return s + strings.Repeat(" ", w-len(r))
}

// truncate shortens s to max runes, appending "…" when truncated.
func truncate(s string, max int) string {
	runes := []rune(s)
	if len(runes) <= max {
		return s
	}
	if max <= 1 {
		return "…"
	}
	return string(runes[:max-1]) + "…"
}


// PrintSection writes a titled underlined section header to w.
func PrintSection(w io.Writer, title string) error {
	if _, err := fmt.Fprintln(w, title); err != nil {
		return err
	}
	_, err := fmt.Fprintln(w, strings.Repeat("─", len([]rune(title))))
	return err
}

// PrintKeyValueSection writes a batch of key/value pairs with uniform left-column
// padding determined by the longest key in the batch.
func PrintKeyValueSection(w io.Writer, pairs [][2]string) error {
	maxLen := 0
	for _, p := range pairs {
		if len(p[0]) > maxLen {
			maxLen = len(p[0])
		}
	}
	for _, p := range pairs {
		if _, err := fmt.Fprintf(w, "  %-*s  %s\n", maxLen, p[0], p[1]); err != nil {
			return err
		}
	}
	return nil
}

// PrintKeyValue writes a single key/value pair. Kept for backward compatibility;
// callers with multiple related pairs should prefer PrintKeyValueSection.
func PrintKeyValue(w io.Writer, key string, value any) error {
	return PrintKeyValueSection(w, [][2]string{{key, fmt.Sprint(value)}})
}

// PrintEmpty writes a "No <entity>." message to w.
func PrintEmpty(w io.Writer, entity string) error {
	_, err := fmt.Fprintf(w, "No %s.\n", entity)
	return err
}
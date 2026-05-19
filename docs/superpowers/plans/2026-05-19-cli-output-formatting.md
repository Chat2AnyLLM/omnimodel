# CLI Output Formatting Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Replace the loose, borderless CLI output with a consistent two-layer layout: compact key/value summaries and Unicode-bordered tables across all `omnillm` commands.

**Architecture:** All changes live in `internal/commands/`. A single `output.go` provides the upgraded shared primitives (`Table` with bordered render + per-column max widths, `PrintKeyValueSection` for batch-aligned key/value blocks). Every command file is then updated to call the new helpers. No server-side changes.

**Tech Stack:** Go stdlib only — no new dependencies. Unicode box-drawing characters written directly as string literals.

---

## File Map

| File | Change |
|------|--------|
| `internal/commands/output.go` | Core renderer: bordered `Table`, `SetMaxWidth`, `truncate`, `PrintKeyValueSection` |
| `internal/commands/output_test.go` | New tests for bordered render, truncation, batch alignment; update existing assertions |
| `internal/commands/status.go` | `PrintKeyValue` → `PrintKeyValueSection`; tables auto-bordered |
| `internal/commands/doctor.go` | Fix `printCheck` alignment; split scalar fields into `PrintKeyValueSection` block |
| `internal/commands/provider.go` | Add `SetMaxWidth` calls for `ID` and `NAME` columns |
| `internal/commands/model.go` | Add `SetMaxWidth` for `MODEL`, `MODEL ID`, `NAME` columns |
| `internal/commands/virtualmodel.go` | Switch detail view to 2-col bordered table; add max widths to list |
| `internal/commands/check-usage.go` | Tables auto-bordered via updated `Render` |
| `internal/commands/config.go` | Tables auto-bordered via updated `Render` |

---

## Task 1: Upgrade `output.go` — bordered Table renderer

**Files:**
- Modify: `internal/commands/output.go`

### What this task does

Replace the current borderless `Render` with full Unicode box-drawing borders. Add `SetMaxWidth(col, max int)` to configure per-column truncation. Add private `truncate(s string, max int) string` helper.

- [ ] **Step 1: Replace `output.go` completely**

```go
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
		widths[i] = len(h)
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
		if len(row[i]) > t.widths[i] {
			t.widths[i] = len(row[i])
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
		sb.WriteString(padRight(val, w))
		sb.WriteString(" │")
	}
	return sb.String()
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

// padRight pads s with spaces on the right to width w.
func padRight(s string, w int) string {
	r := []rune(s)
	if len(r) >= w {
		return s
	}
	return s + strings.Repeat(" ", w-len(r))
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
```

- [ ] **Step 2: Run the package tests to confirm compile + existing tests still pass**

```
go test ./internal/commands/...
```

Expected: all existing tests pass (some may fail if they assert on old spacing — fix those in Task 2).

---

## Task 2: Update `output_test.go` — new tests + fix existing assertions

**Files:**
- Modify: `internal/commands/output_test.go`

- [ ] **Step 1: Write failing tests for the new bordered render, truncation, and batch key/value**

Replace the contents of `output_test.go` with:

```go
package commands

import (
	"bytes"
	"strings"
	"testing"

	"github.com/spf13/cobra"
)

func TestTableRenderBordered(t *testing.T) {
	table := NewTable("NAME", "VALUE")
	table.AddRow("alpha", "1")
	table.AddRow("beta", "22")

	var out bytes.Buffer
	if err := table.Render(&out); err != nil {
		t.Fatalf("Render returned error: %v", err)
	}

	rendered := out.String()
	for _, want := range []string{"NAME", "VALUE", "alpha", "beta", "22", "┌", "┤", "└", "│"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("rendered table missing %q:\n%s", want, rendered)
		}
	}
}

func TestTableRenderNoRows(t *testing.T) {
	table := NewTable("A", "B")
	var out bytes.Buffer
	if err := table.Render(&out); err != nil {
		t.Fatalf("Render returned error: %v", err)
	}
	rendered := out.String()
	// Header row and borders present; no mid-separator without rows
	if !strings.Contains(rendered, "┌") || !strings.Contains(rendered, "└") {
		t.Fatalf("expected border characters, got:\n%s", rendered)
	}
	if strings.Contains(rendered, "├") {
		t.Fatalf("unexpected mid-separator when no rows, got:\n%s", rendered)
	}
}

func TestTableTruncation(t *testing.T) {
	table := NewTable("ID")
	table.SetMaxWidth(0, 10)
	table.AddRow("short")
	table.AddRow("this-is-a-very-long-identifier")

	var out bytes.Buffer
	if err := table.Render(&out); err != nil {
		t.Fatalf("Render returned error: %v", err)
	}

	rendered := out.String()
	if !strings.Contains(rendered, "short") {
		t.Fatalf("expected 'short' unmodified, got:\n%s", rendered)
	}
	if strings.Contains(rendered, "this-is-a-very-long-identifier") {
		t.Fatalf("expected long value to be truncated, got:\n%s", rendered)
	}
	if !strings.Contains(rendered, "…") {
		t.Fatalf("expected truncation ellipsis '…', got:\n%s", rendered)
	}
}

func TestTruncateHelper(t *testing.T) {
	cases := []struct {
		in   string
		max  int
		want string
	}{
		{"hello", 10, "hello"},
		{"hello", 5, "hello"},
		{"hello world", 8, "hello w…"},
		{"hello", 1, "…"},
		{"hello", 0, "…"},
		{"", 5, ""},
	}
	for _, tc := range cases {
		got := truncate(tc.in, tc.max)
		if got != tc.want {
			t.Errorf("truncate(%q, %d) = %q, want %q", tc.in, tc.max, got, tc.want)
		}
	}
}

func TestPrintKeyValueSection(t *testing.T) {
	var out bytes.Buffer
	pairs := [][2]string{
		{"Status", "healthy"},
		{"Model count", "273"},
		{"Manual approve", "no"},
	}
	if err := PrintKeyValueSection(&out, pairs); err != nil {
		t.Fatalf("PrintKeyValueSection returned error: %v", err)
	}

	rendered := out.String()
	// All values must appear
	for _, want := range []string{"Status", "healthy", "Model count", "273", "Manual approve", "no"} {
		if !strings.Contains(rendered, want) {
			t.Fatalf("output missing %q:\n%s", want, rendered)
		}
	}
	// Keys must be left-aligned with uniform padding:
	// "Status        " should be padded to width of "Manual approve" (14)
	lines := strings.Split(strings.TrimSpace(rendered), "\n")
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d:\n%s", len(lines), rendered)
	}
	// Each line must start with two spaces and contain the key padded to width 14
	if !strings.HasPrefix(lines[0], "  Status        ") {
		t.Errorf("expected line 0 to start with '  Status        ', got %q", lines[0])
	}
	if !strings.HasPrefix(lines[1], "  Model count   ") {
		t.Errorf("expected line 1 to start with '  Model count   ', got %q", lines[1])
	}
}

func TestPrintKeyValue(t *testing.T) {
	var out bytes.Buffer
	if err := PrintKeyValue(&out, "Status", "healthy"); err != nil {
		t.Fatalf("PrintKeyValue returned error: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "Status") || !strings.Contains(got, "healthy") {
		t.Fatalf("PrintKeyValue output missing content: %q", got)
	}
}

func TestPrintEmpty(t *testing.T) {
	var out bytes.Buffer
	if err := PrintEmpty(&out, "chat sessions"); err != nil {
		t.Fatalf("PrintEmpty returned error: %v", err)
	}
	if got := out.String(); got != "No chat sessions.\n" {
		t.Fatalf("PrintEmpty output = %q, want %q", got, "No chat sessions.\n")
	}
}

func TestPrintSection(t *testing.T) {
	var out bytes.Buffer
	if err := PrintSection(&out, "Status"); err != nil {
		t.Fatalf("PrintSection returned error: %v", err)
	}
	got := out.String()
	if !strings.Contains(got, "Status\n") || !strings.Contains(got, "──────") {
		t.Fatalf("unexpected section output:\n%s", got)
	}
}

func TestConfirmReadsFromCommandStreams(t *testing.T) {
	cmd := &cobra.Command{}
	cmd.SetIn(strings.NewReader("y\n"))
	var stderr bytes.Buffer
	cmd.SetErr(&stderr)

	if !Confirm(cmd, "Delete provider?") {
		t.Fatal("Confirm returned false, want true")
	}
	if got := stderr.String(); got != "Delete provider? [y/N] " {
		t.Fatalf("stderr prompt = %q, want %q", got, "Delete provider? [y/N] ")
	}
}
```

- [ ] **Step 2: Run tests — expect some failures while truncate(s, 0) edge case is checked**

```
go test ./internal/commands/... -run "TestTable|TestTruncate|TestPrintKeyValue|TestPrintSection|TestPrintEmpty|TestConfirm"
```

Expected: most pass; `TestTruncateHelper` for `max=0` may reveal edge case in the implementation.

- [ ] **Step 3: Fix `truncate` in `output.go` if `max <= 0` case fails**

In `output.go`, the `truncate` function already handles `max <= 1` — verify `max=0` returns `"…"` not a panic. If `max=0` causes a slice bounds issue, update:

```go
func truncate(s string, max int) string {
	runes := []rune(s)
	if max <= 0 || len(runes) <= max {
		if max <= 0 && len(runes) > 0 {
			return "…"
		}
		return s
	}
	if max == 1 {
		return "…"
	}
	return string(runes[:max-1]) + "…"
}
```

- [ ] **Step 4: Run all tests and confirm green**

```
go test ./internal/commands/...
```

Expected: all pass.

- [ ] **Step 5: Commit**

```
git add internal/commands/output.go internal/commands/output_test.go
git commit --author="James Zhu <zhujian0805@gmail.com>" -m "feat(cli): bordered Table renderer, SetMaxWidth, truncate, PrintKeyValueSection"
```

---

## Task 3: Update `status.go`

**Files:**
- Modify: `internal/commands/status.go`

- [ ] **Step 1: Replace `PrintKeyValue` loops with `PrintKeyValueSection`, set max widths on provider/service tables**

Replace `runServerStatus` in `status.go`:

```go
func runServerStatus(cmd *cobra.Command, args []string) error {
	c := NewClient(cmd)
	data, err := c.Get("/api/admin/status")
	if err != nil {
		return err
	}
	if c.IsJSON() {
		c.PrintJSON(data)
		return nil
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(data, &resp); err != nil {
		return err
	}

	out := cmd.OutOrStdout()
	status, _ := resp["status"].(string)
	uptime, _ := resp["uptime"].(string)
	modelCount, _ := resp["modelCount"].(float64)
	manualApprove, _ := resp["manualApprove"].(bool)
	rateLimitSeconds := resp["rateLimitSeconds"]
	rateLimitWait, _ := resp["rateLimitWait"].(bool)

	manualApproveStr := "no"
	if manualApprove {
		manualApproveStr = "yes"
	}
	rateLimitStr := "none"
	if rateLimitSeconds != nil {
		rateLimitStr = fmt.Sprintf("%vs (wait=%v)", rateLimitSeconds, rateLimitWait)
	}

	if err := PrintSection(out, "Server status"); err != nil {
		return err
	}
	if err := PrintKeyValueSection(out, [][2]string{
		{"Status", status},
		{"Uptime", uptime},
		{"Model count", fmt.Sprintf("%.0f", modelCount)},
		{"Manual approve", manualApproveStr},
		{"Rate limit", rateLimitStr},
	}); err != nil {
		return err
	}

	providerCount := 0
	providerTable := NewTable("Name", "ID")
	providerTable.SetMaxWidth(0, 40)
	providerTable.SetMaxWidth(1, 32)
	if providers, ok := resp["activeProviders"].([]interface{}); ok {
		for _, entry := range providers {
			provider, _ := entry.(map[string]interface{})
			name, _ := provider["name"].(string)
			id, _ := provider["id"].(string)
			if name == "" && id == "" {
				continue
			}
			providerTable.AddRow(name, id)
			providerCount++
		}
	} else if ap, ok := resp["activeProvider"].(map[string]interface{}); ok {
		name, _ := ap["name"].(string)
		id, _ := ap["id"].(string)
		if name != "" || id != "" {
			providerTable.AddRow(name, id)
			providerCount = 1
		}
	}

	if providerCount > 0 {
		if _, err := fmt.Fprintln(out); err != nil {
			return err
		}
		if err := PrintSection(out, "Active providers"); err != nil {
			return err
		}
		if err := providerTable.Render(out); err != nil {
			return err
		}
	} else if err := PrintKeyValue(out, "Active providers", "none"); err != nil {
		return err
	}

	if services, ok := resp["services"].(map[string]interface{}); ok {
		if _, err := fmt.Fprintln(out); err != nil {
			return err
		}
		if err := PrintSection(out, "Services"); err != nil {
			return err
		}
		serviceTable := NewTable("Service", "Status")
		serviceTable.AddRow("API", fmt.Sprint(services["api"]))
		serviceTable.AddRow("Database", fmt.Sprint(services["database"]))
		if providers, ok := services["providers"].(map[string]interface{}); ok {
			serviceTable.AddRow("Providers", fmt.Sprintf("%v total, %v active", providers["total"], providers["active"]))
		}
		if err := serviceTable.Render(out); err != nil {
			return err
		}
	}

	if authFlow, ok := resp["authFlow"].(map[string]interface{}); ok && authFlow != nil {
		flowStatus, _ := authFlow["status"].(string)
		providerID, _ := authFlow["providerId"].(string)
		if _, err := fmt.Fprintln(out); err != nil {
			return err
		}
		if err := PrintSection(out, "Active auth flow"); err != nil {
			return err
		}
		authTable := NewTable("Field", "Value")
		authTable.SetMaxWidth(1, 48)
		authTable.AddRow("Status", flowStatus)
		authTable.AddRow("Provider", providerID)
		if uc, ok := authFlow["userCode"].(string); ok && uc != "" {
			authTable.AddRow("User code", uc)
		}
		if url, ok := authFlow["instructionURL"].(string); ok && url != "" {
			authTable.AddRow("Visit", url)
		}
		if err := authTable.Render(out); err != nil {
			return err
		}
	}

	return nil
}
```

Also update the `statusAuthCmd` RunE to use a bordered detail table:

```go
// In statusAuthCmd.RunE, replace the table block with:
table := NewTable("Field", "Value")
table.SetMaxWidth(1, 48)
table.AddRow("Provider", providerID)
table.AddRow("Status", status)
if uc, ok := resp["userCode"].(string); ok && uc != "" {
    table.AddRow("User code", uc)
}
if url, ok := resp["instructionURL"].(string); ok && url != "" {
    table.AddRow("Visit", url)
}
if errMsg, ok := resp["error"].(string); ok && errMsg != "" {
    table.AddRow("Error", errMsg)
}
return table.Render(out)
```

- [ ] **Step 2: Run tests**

```
go test ./internal/commands/...
```

Expected: all pass.

- [ ] **Step 3: Commit**

```
git add internal/commands/status.go
git commit --author="James Zhu <zhujian0805@gmail.com>" -m "feat(cli): status uses PrintKeyValueSection and bordered tables"
```

---

## Task 4: Update `doctor.go`

**Files:**
- Modify: `internal/commands/doctor.go`

- [ ] **Step 1: Fix `printCheck` padding and split scalar fields into `PrintKeyValueSection`**

The current `printCheck` uses a hardcoded `%-22s` width. Replace `printCheck` with a version that takes an explicit label column width, and update `runDoctor` to compute widths per section:

Replace the entire `doctor.go` content:

```go
package commands

import (
	"encoding/json"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"time"

	"github.com/spf13/cobra"
)

// DoctorCmd checks the local environment and server health for the operator.
var DoctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Check configuration and server health",
	Long: `Check the local OmniLLM configuration and running server health.

Verifies:
  - Config directory and database presence
  - Server reachability
  - API key configuration
  - Provider and virtual model counts
  - In-progress authentication flows

Prints a recommended next action at the end.`,
	Example: `  omnillm doctor
  omnillm doctor --server http://127.0.0.1:5000`,
	RunE: runDoctor,
}

type checkRow struct {
	ok    bool
	label string
	value string
}

func printChecks(out io.Writer, rows []checkRow) error {
	maxLabel := 0
	for _, r := range rows {
		if len(r.label) > maxLabel {
			maxLabel = len(r.label)
		}
	}
	for _, r := range rows {
		icon := "✓"
		if !r.ok {
			icon = "✗"
		}
		if _, err := fmt.Fprintf(out, "  %s  %-*s  %s\n", icon, maxLabel, r.label+":", r.value); err != nil {
			return err
		}
	}
	return nil
}

func runDoctor(cmd *cobra.Command, args []string) error {
	out := cmd.OutOrStdout()
	nextStep := ""

	if err := PrintSection(out, "OmniLLM Doctor"); err != nil {
		return err
	}
	fmt.Fprintln(out)

	// ── Local config ──────────────────────────────────────────────────────────
	if err := PrintSection(out, "Local configuration"); err != nil {
		return err
	}

	homeDir, _ := os.UserHomeDir()
	configDir := filepath.Join(homeDir, ".config", "omnillm")
	dbPath := filepath.Join(configDir, "database.sqlite")
	apiKeyFile := filepath.Join(configDir, "api-key")

	configOK := true
	if _, err := os.Stat(configDir); os.IsNotExist(err) {
		configOK = false
		if nextStep == "" {
			nextStep = "Run 'omnillm start' to initialise the configuration directory."
		}
	}

	dbValue := dbPath + " (not found — will be created on first start)"
	dbOK := false
	if info, err := os.Stat(dbPath); err == nil {
		dbOK = true
		dbValue = fmt.Sprintf("%s (%d bytes)", dbPath, info.Size())
	}

	apiKeyOK := true
	if _, err := os.Stat(apiKeyFile); os.IsNotExist(err) {
		apiKeyOK = false
	}
	apiKeyValue := apiKeyFile
	if !apiKeyOK {
		apiKeyValue = "not found (will be generated on start)"
	}

	configChecks := []checkRow{
		{ok: configOK, label: "Config directory", value: configDir},
		{ok: dbOK, label: "Database", value: dbValue},
		{ok: apiKeyOK, label: "API key file", value: apiKeyValue},
	}
	if err := printChecks(out, configChecks); err != nil {
		return err
	}

	fmt.Fprintln(out)

	// ── Server reachability ───────────────────────────────────────────────────
	if err := PrintSection(out, "Server"); err != nil {
		return err
	}

	c := NewClient(cmd)
	apiKeyConfigured := c.APIKey != ""
	apiKeyStatus := "configured"
	if !apiKeyConfigured {
		apiKeyStatus = "not set (use --api-key or OMNILLM_API_KEY)"
	}

	serverChecks := []checkRow{
		{ok: true, label: "Server address", value: c.BaseURL},
		{ok: apiKeyConfigured, label: "API key", value: apiKeyStatus},
	}

	serverOK := false
	var statusResp map[string]interface{}
	start := time.Now()
	statusData, err := c.Get("/api/admin/status")
	latency := time.Since(start)
	if err != nil {
		serverChecks = append(serverChecks, checkRow{ok: false, label: "Server reachable", value: fmt.Sprintf("NO — %v", err)})
		if nextStep == "" {
			nextStep = "Run 'omnillm start' to start the server."
		}
	} else {
		serverOK = true
		serverChecks = append(serverChecks, checkRow{ok: true, label: "Server reachable", value: fmt.Sprintf("yes (%dms)", latency.Milliseconds())})
		_ = json.Unmarshal(statusData, &statusResp)
	}

	if err := printChecks(out, serverChecks); err != nil {
		return err
	}

	if serverOK && statusResp != nil {
		fmt.Fprintln(out)
		if err := PrintSection(out, "Server status"); err != nil {
			return err
		}
		status, _ := statusResp["status"].(string)
		uptime, _ := statusResp["uptime"].(string)
		modelCount, _ := statusResp["modelCount"].(float64)

		statusOK := status == "ok" || status == "running" || status == "healthy"
		statusChecks := []checkRow{
			{ok: statusOK, label: "Status", value: status},
		}
		if err := printChecks(out, statusChecks); err != nil {
			return err
		}
		if err := PrintKeyValueSection(out, [][2]string{
			{"Uptime", uptime},
			{"Models", fmt.Sprintf("%.0f", modelCount)},
		}); err != nil {
			return err
		}

		// ── Providers ──────────────────────────────────────────────────────────
		fmt.Fprintln(out)
		if err := PrintSection(out, "Providers"); err != nil {
			return err
		}

		providerData, provErr := c.Get("/api/admin/providers")
		if provErr == nil {
			providers, _ := parseProviders(providerData)
			activeCount := 0
			for _, p := range providers {
				if v, ok := p["isActive"].(bool); ok && v {
					activeCount++
				}
			}
			providerOK := len(providers) > 0
			provChecks := []checkRow{
				{ok: providerOK, label: "Providers configured", value: fmt.Sprintf("%d total, %d active", len(providers), activeCount)},
			}
			if !providerOK && nextStep == "" {
				nextStep = "Run 'omnillm auth' to add and authenticate a provider."
			} else if activeCount == 0 && len(providers) > 0 && nextStep == "" {
				nextStep = "Run 'omnillm provider activate <id>' to activate a provider."
			}

			vmData, vmErr := c.Get("/api/admin/virtualmodels")
			if vmErr == nil {
				var vmResp map[string]interface{}
				if jsonErr := json.Unmarshal(vmData, &vmResp); jsonErr == nil {
					items, _ := vmResp["data"].([]interface{})
					provChecks = append(provChecks, checkRow{ok: true, label: "Virtual models", value: fmt.Sprintf("%d configured", len(items))})
				}
			}

			if err := printChecks(out, provChecks); err != nil {
				return err
			}
		}

		// ── Auth flow ──────────────────────────────────────────────────────────
		authData, authErr := c.Get("/api/admin/auth-status")
		if authErr == nil {
			var authResp map[string]interface{}
			if jsonErr := json.Unmarshal(authData, &authResp); jsonErr == nil {
				authStatus, _ := authResp["status"].(string)
				if authStatus != "" && authStatus != "idle" {
					fmt.Fprintln(out)
					if err := PrintSection(out, "Active auth flow"); err != nil {
						return err
					}
					providerID, _ := authResp["providerId"].(string)
					authChecks := []checkRow{
						{ok: false, label: "Auth in progress", value: fmt.Sprintf("%s (%s)", providerID, authStatus)},
					}
					if err := printChecks(out, authChecks); err != nil {
						return err
					}
					kvPairs := [][2]string{}
					if userCode, ok := authResp["userCode"].(string); ok && userCode != "" {
						kvPairs = append(kvPairs, [2]string{"User code", userCode})
					}
					if url, ok := authResp["instructionURL"].(string); ok && url != "" {
						kvPairs = append(kvPairs, [2]string{"Visit", url})
					}
					if len(kvPairs) > 0 {
						if err := PrintKeyValueSection(out, kvPairs); err != nil {
							return err
						}
					}
				}
			}
		}
	}

	// ── Next step ──────────────────────────────────────────────────────────────
	fmt.Fprintln(out)
	if nextStep != "" {
		if _, err := fmt.Fprintf(out, "Next step: %s\n", nextStep); err != nil {
			return err
		}
	} else {
		if _, err := fmt.Fprintln(out, "Everything looks good. ✓"); err != nil {
			return err
		}
	}

	return nil
}
```

- [ ] **Step 2: Run tests**

```
go test ./internal/commands/...
```

Expected: all pass (the regression test `TestDoctorCmdWritesHealthyStatusWithSuccessMarker` must still pass).

- [ ] **Step 3: Commit**

```
git add internal/commands/doctor.go
git commit --author="James Zhu <zhujian0805@gmail.com>" -m "feat(cli): doctor uses aligned check rows and PrintKeyValueSection"
```

---

## Task 5: Update `provider.go`

**Files:**
- Modify: `internal/commands/provider.go`

- [ ] **Step 1: Add `SetMaxWidth` calls to the provider list table**

Find the provider list table construction (around line 166):

```go
table := NewTable("ID", "TYPE", "NAME", "AUTH", "ACTIVE", "MODELS")
```

Replace with:

```go
table := NewTable("ID", "TYPE", "NAME", "AUTH", "ACTIVE", "MODELS")
table.SetMaxWidth(0, 32) // ID
table.SetMaxWidth(2, 36) // NAME
```

- [ ] **Step 2: Run tests**

```
go test ./internal/commands/...
```

Expected: all pass.

- [ ] **Step 3: Commit**

```
git add internal/commands/provider.go
git commit --author="James Zhu <zhujian0805@gmail.com>" -m "feat(cli): provider list truncates long ID and NAME columns"
```

---

## Task 6: Update `model.go`

**Files:**
- Modify: `internal/commands/model.go`

- [ ] **Step 1: Add `SetMaxWidth` calls to model list and metadata tables**

In `modelListCmd` RunE, find:

```go
table := NewTable("MODEL ID", "NAME", "ENABLED")
```

Replace with:

```go
table := NewTable("MODEL ID", "NAME", "ENABLED")
table.SetMaxWidth(0, 40) // MODEL ID
table.SetMaxWidth(1, 36) // NAME
```

In `modelMetadataCmd` RunE, find:

```go
table := NewTable("MODEL", "PROVIDER", "INPUT $/1M", "OUTPUT $/1M", "CTX", "IN", "OUT")
```

Replace with:

```go
table := NewTable("MODEL", "PROVIDER", "INPUT $/1M", "OUTPUT $/1M", "CTX", "IN", "OUT")
table.SetMaxWidth(0, 40) // MODEL
table.SetMaxWidth(1, 24) // PROVIDER
```

- [ ] **Step 2: Run tests**

```
go test ./internal/commands/...
```

Expected: all pass.

- [ ] **Step 3: Commit**

```
git add internal/commands/model.go
git commit --author="James Zhu <zhujian0805@gmail.com>" -m "feat(cli): model list and metadata truncate long columns"
```

---

## Task 7: Update `virtualmodel.go`

**Files:**
- Modify: `internal/commands/virtualmodel.go`

- [ ] **Step 1: Add max widths to list table; switch detail view to bordered 2-col table**

For the list table, find:

```go
table := NewTable("ID", "NAME", "STRATEGY", "API SHAPE", "UPSTREAMS", "ENABLED")
```

Replace with:

```go
table := NewTable("ID", "NAME", "STRATEGY", "API SHAPE", "UPSTREAMS", "ENABLED")
table.SetMaxWidth(0, 32) // ID
table.SetMaxWidth(1, 32) // NAME
```

For the detail view (the `get` subcommand), replace all `PrintKeyValue` calls with a single bordered table. Find the block that starts with:

```go
if err := PrintKeyValue(out, "Name", name); err != nil {
```

Replace that entire block (through the upstreams table) with:

```go
detailTable := NewTable("Field", "Value")
detailTable.SetMaxWidth(1, 48)
detailTable.AddRow("Name", name)
detailTable.AddRow("Description", desc)
detailTable.AddRow("Strategy", strategy)
detailTable.AddRow("API Shape", apiShape)
detailTable.AddRow("Enabled", enabled)
if err := detailTable.Render(out); err != nil {
    return err
}

if len(upstreams) > 0 {
    if _, err := fmt.Fprintln(out); err != nil {
        return err
    }
    if err := PrintSection(out, "Upstreams"); err != nil {
        return err
    }
    upstreamTable := NewTable("N", "Provider", "Model", "Weight", "Priority")
    // ... existing upstream row-adding code unchanged ...
    if err := upstreamTable.Render(out); err != nil {
        return err
    }
}
```

Note: the variable names `name`, `desc`, `strategy`, `apiShape`, `enabled`, `upstreams` must match what is already extracted in the `get` subcommand RunE. Read the current `virtualmodel.go` `get` block carefully before editing to ensure variable names align.

- [ ] **Step 2: Run tests**

```
go test ./internal/commands/...
```

Expected: all pass.

- [ ] **Step 3: Commit**

```
git add internal/commands/virtualmodel.go
git commit --author="James Zhu <zhujian0805@gmail.com>" -m "feat(cli): virtualmodel list truncates columns; get uses bordered detail table"
```

---

## Task 8: Update `check-usage.go` and `config.go`

**Files:**
- Modify: `internal/commands/check-usage.go`
- Modify: `internal/commands/config.go`

These files use `NewTable` + `Render` already. With the upgraded `Table.Render`, their tables will automatically gain borders with no further changes needed. However, `check-usage.go` has a label column that can be unbounded — add a max width.

- [ ] **Step 1: Add max width for the label column in `usageBreakdownTable`**

In `check-usage.go`, find `usageBreakdownTable`:

```go
func usageBreakdownTable(breakdown string) *Table {
    ...
    return NewTable(labelHeader, "REQUESTS", "TOKENS", "AVG LATENCY MS")
}
```

Replace with:

```go
func usageBreakdownTable(breakdown string) *Table {
    labelHeader := map[string]string{
        "provider":  "PROVIDER",
        "providers": "PROVIDER",
        "model":     "MODEL",
        "models":    "MODEL",
        "client":    "CLIENT",
        "clients":   "CLIENT",
    }[breakdown]
    t := NewTable(labelHeader, "REQUESTS", "TOKENS", "AVG LATENCY MS")
    t.SetMaxWidth(0, 40) // label column
    return t
}
```

- [ ] **Step 2: Run tests**

```
go test ./internal/commands/...
```

Expected: all pass.

- [ ] **Step 3: Commit**

```
git add internal/commands/check-usage.go internal/commands/config.go
git commit --author="James Zhu <zhujian0805@gmail.com>" -m "feat(cli): check-usage label column gets max width; all tables now bordered"
```

---

## Self-Review Checklist

- [x] **Spec coverage:** Bordered Table (Task 1), truncation (Task 1), `PrintKeyValueSection` (Task 1+2), `status.go` (Task 3), `doctor.go` (Task 4), `provider list` max widths (Task 5), `model list` max widths (Task 6), `virtualmodel` detail + list (Task 7), `check-usage`/`config` (Task 8). All spec sections covered.
- [x] **No placeholders:** All code is complete. Task 7 has one note to check variable names — this is a reminder not a TBD.
- [x] **Type consistency:** `SetMaxWidth(col, max int)`, `truncate(s string, max int) string`, `PrintKeyValueSection(w io.Writer, pairs [][2]string) error` are defined in Task 1 and used consistently across all later tasks.
- [x] **Boolean display:** `manualApprove` rendered as `"yes"`/`"no"` in Task 3. `enabled` already rendered as `"yes"`/`"no"` in existing code.
- [x] **Regression test preserved:** `TestDoctorCmdWritesHealthyStatusWithSuccessMarker` is not touched; Task 4 keeps the same `status == "ok" || status == "running" || status == "healthy"` condition.

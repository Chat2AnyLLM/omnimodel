package chat

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"omnillm/internal/specdriven"
)

// specHelpMarkdown returns the /spec entrypoint help text as markdown.
// Used by both TUI (rendered via glamour) and plain REPL.
func specHelpMarkdown() string {
	return `**Choose a spec-driven method**

` + "`/spec`" + ` starts spec-driven mode selection. Pick one of the supported workflows:

| Workflow | Use when | Command |
| --- | --- | --- |
| **spec-kit** | Building a feature through specify -> plan -> tasks -> implement, then complete/archive cleanly | ` + "`/spec mode spec-kit`" + ` |
| **OpenSpec** | Managing a requirements-driven change through propose -> apply -> archive | ` + "`/spec mode openspec`" + ` |

**Slash commands**

- ` + "`/spec mode spec-kit`" + ` -> enter spec-kit mode and show its commands
- ` + "`/spec mode openspec`" + ` -> enter OpenSpec mode and show its commands
- ` + "`/spec mode off`" + ` -> leave spec-driven mode
- ` + "`/spec init <title>`" + ` -> create ` + "`specs/<N>-<slug>/`" + ` + ` + "`spec.md`" + ` template
- ` + "`/spec status [dir]`" + ` -> show specs and artifact status
- ` + "`/spec help`" + ` -> show this chooser

**Core Spec Kit commands**

` + specdriven.RenderSpecKitCommandTable() + `
**OpenSpec commands**

` + specdriven.RenderOpenSpecCommandTable() + `
Tip: in the TUI, type ` + "`/spec `" + `, ` + "`/speckit`" + `, ` + "`/opsx`" + `, or ` + "`/openspec`" + ` to see these choices in the slash-command picker.
`
}

// specKitWorkflowSummary returns a concise markdown summary of the spec-kit workflow.
func specKitWorkflowSummary() string {
	return `**spec-kit mode** -> constitution -> specify -> clarify -> plan -> tasks -> analyze -> implement

**Clean lifecycle**

- ` + "`draft`" + ` -> spec exists and is being refined
- ` + "`in_progress`" + ` -> implementation has started
- ` + "`completed`" + ` -> implementation is done; keep ` + "`spec.md`" + `, ` + "`plan.md`" + `, and ` + "`tasks.md`" + `
- ` + "`archived`" + ` -> optional move to ` + "`specs/archive/`" + ` to reduce clutter

After implementation: validate -> mark completed -> optionally archive.

**Slash commands**

- ` + "`/spec init <title>`" + ` -> create a numbered spec dir + blank spec.md
- ` + "`/spec status [dir]`" + ` -> scan artifact completion
- ` + "`/spec mode openspec`" + ` -> switch to OpenSpec mode
- ` + "`/spec mode off`" + ` -> leave spec-driven mode

**Core Spec Kit commands**

` + specdriven.RenderSpecKitCommandTable() + `
`
}

// openSpecWorkflowSummary returns a concise markdown summary of the OpenSpec workflow.
func openSpecWorkflowSummary() string {
	return "**openspec mode** -> propose -> apply -> archive\n\n" +
		"**Slash commands**\n\n" +
		"- `/opsx:propose [change]` -> create a change and planning artifacts\n" +
		"- `/opsx:explore [topic]` -> investigate before committing to a change\n" +
		"- `/opsx:apply [change]` -> implement or report pending tasks\n" +
		"- `/opsx:sync [change]` -> merge delta specs into `openspec/specs/`\n" +
		"- `/opsx:archive [change]` -> archive a completed change\n" +
		"- `/opsx:new`, `/opsx:continue`, `/opsx:ff`, `/opsx:verify`, `/opsx:bulk-archive`, `/opsx:onboard` -> expanded workflow\n" +
		"- `/spec mode spec-kit` -> switch to spec-kit mode\n" +
		"- `/spec mode off` -> leave spec-driven mode\n\n" +
		"**OpenSpec commands** (after `load_skill(\"spec\")`)\n\n" +
		specdriven.RenderOpenSpecCommandTable() + "\n"
}

// validSpecModes are the recognized spec mode values.
var validSpecModes = []string{"spec-kit", "openspec"}

// isValidSpecMode returns true if mode is a recognized spec mode.
func isValidSpecMode(mode string) bool {
	for _, m := range validSpecModes {
		if m == mode {
			return true
		}
	}
	return false
}

// specREPLInit is the implementation behind "/spec init <title>".
// It creates the spec directory and spec.md directly without going through the
// agent loop 閳?useful for quickly kicking off a new feature before switching to
// agent mode for the rich speckit_specify / speckit_plan / speckit_tasks steps.
func specREPLInit(w io.Writer, title string) error {
	specsRoot := "specs"
	number, err := nextSpecNumber(specsRoot)
	if err != nil {
		return err
	}
	slug := specdriven.Slugify(title)
	spec := &specdriven.Spec{
		Number:    number,
		Slug:      slug,
		Title:     title,
		Overview:  "TODO: describe the feature",
		CreatedAt: specdriven.NowISO(),
	}

	dirPath := filepath.Join(specsRoot, spec.DirName())
	if err := os.MkdirAll(dirPath, 0o755); err != nil {
		return fmt.Errorf("create dir: %w", err)
	}

	specFile := filepath.Join(dirPath, "spec.md")
	content := specdriven.RenderSpec(spec)
	if err := os.WriteFile(specFile, []byte(content), 0o644); err != nil {
		return fmt.Errorf("write spec.md: %w", err)
	}
	if _, err := specdriven.EnsureLifecycle(dirPath, spec.CreatedAt, true); err != nil {
		return fmt.Errorf("write lifecycle metadata: %w", err)
	}

	_, _ = fmt.Fprintf(w, "Created %s\n", specFile)
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Next steps:")
	_, _ = fmt.Fprintln(w, "  1. Switch to agent mode:  /mode agent")
	_, _ = fmt.Fprintln(w, "  2. Load the spec skill and add user stories:")
	_, _ = fmt.Fprintf(w, "     \"load the spec skill and write the spec for %s with user stories and acceptance scenarios\"\n", title)
	return nil
}

// specREPLStatus is the implementation behind "/spec status [dir]".
func specREPLStatus(w io.Writer, specsRoot string) error {
	entries, err := os.ReadDir(specsRoot)
	if err != nil {
		if os.IsNotExist(err) {
			_, _ = fmt.Fprintf(w, "No specs directory found at %q. Use /spec init <title> to create one.\n", specsRoot)
			return nil
		}
		return err
	}

	found := 0
	for _, entry := range entries {
		if !entry.IsDir() || entry.Name() == "archive" {
			continue
		}
		if found > 0 {
			_, _ = fmt.Fprintln(w, "")
			_, _ = fmt.Fprintln(w, strings.Repeat("-", 72))
			_, _ = fmt.Fprintln(w, "")
		}
		dirPath := filepath.Join(specsRoot, entry.Name())
		present := specdriven.ArtifactPresence(dirPath)
		record, err := specdriven.EnsureLifecycle(dirPath, "", false)
		if err != nil {
			return fmt.Errorf("read lifecycle metadata for %s: %w", entry.Name(), err)
		}
		_, _ = fmt.Fprint(w, specdriven.RenderLifecycleStatus(entry.Name(), present, record))
		found++
	}
	if found == 0 {
		_, _ = fmt.Fprintf(w, "No spec directories in %q. Use /spec init <title> to create one.\n", specsRoot)
	}
	return nil
}

// nextSpecNumber scans specsRoot for existing numbered directories and returns
// the next zero-padded number string.  Mirrors the same helper in specdriven.go.
func nextSpecNumber(specsRoot string) (string, error) {
	entries, err := os.ReadDir(specsRoot)
	if err != nil {
		if os.IsNotExist(err) {
			return "001", nil
		}
		return "", err
	}
	max := 0
	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		if len(name) >= 3 {
			var n int
			if _, err := fmt.Sscanf(name[:3], "%d", &n); err == nil && n > max {
				max = n
			}
		}
	}
	return fmt.Sprintf("%03d", max+1), nil
}

package chat

import (
	"fmt"
	"io"
	"os"
	"path/filepath"

	"omnillm/internal/specdriven"
)

// specHelpMarkdown returns the full /spec help text as markdown.
// Used by both TUI (rendered via glamour) and plain REPL.
func specHelpMarkdown() string {
	return `**Spec-driven development workflow**

Combines ideas from **spec-kit** and **OpenSpec** into a unified tool set.
All agent commands require ` + "`/mode agent`" + ` + ` + "`load_skill(\"spec\")`" + `.

**REPL shortcuts** (no agent needed):

- ` + "`/spec init <title>`" + ` — create ` + "`specs/<N>-<slug>/`" + ` + ` + "`spec.md`" + ` template
- ` + "`/spec status`" + ` — show all specs and which artifacts are present
- ` + "`/spec help`" + ` — show this help

---

**spec-kit workflow** — specify → plan → tasks → implement

Full pipeline for structured feature development with prioritized user stories,
Given-When-Then acceptance scenarios, and phase-based planning.

- ` + "`spec_init`" + ` — create numbered spec dir + blank spec.md
- ` + "`spec_write`" + ` — add user stories (P1/P2/P3), Given-When-Then scenarios
- ` + "`spec_plan`" + ` — scaffold plan.md (Phase 0-3, tech context, data model)
- ` + "`spec_tasks`" + ` — generate tasks.md (atomic tasks per story, ` + "`[P]`" + ` = parallelizable)

Example — build a feature from scratch:

` + "```" + `
/mode agent
> load the spec skill
> spec_init: title="Photo Album Organizer"
> spec_write with user stories:
  - As a user I can create albums grouped by date (P1)
  - As a user I can drag-and-drop photos between albums (P1)
  - As a user I can search photos by metadata (P2)
> spec_plan: language=TypeScript, framework=Vite, database=SQLite
> spec_tasks
` + "```" + `

Example — constitution + full specify:

` + "```" + `
> Create a spec for a Kanban board with drag-and-drop task management,
  user assignment, comments, and due dates. Focus on code quality,
  testing standards, and performance requirements.
` + "```" + `

---

**OpenSpec workflow** — propose → apply → archive

Lightweight lifecycle for requirements-driven changes with artifact dependency
tracking (spec → plan → tasks → code) and SHALL/MUST requirement language.

- ` + "`spec_write`" + ` — propose requirements (SHALL/MUST), entities, edge cases
- ` + "`spec_read`" + ` — review spec + artifact dependency graph
- ` + "`spec_plan`" + ` — apply: generate implementation plan from spec
- ` + "`spec_tasks`" + ` — apply: break plan into atomic tasks
- ` + "`spec_status`" + ` — scan artifact completion (spec → plan → tasks → code)

Example — quick feature lifecycle:

` + "```" + `
/mode agent
> load the spec skill
> spec_init: title="Add Dark Mode"
> spec_write with requirements:
  - The system SHALL support a dark color scheme toggle
  - The system MUST persist theme preference across sessions
  - Edge case: system preference changes while app is open
> spec_read                    # review artifact status board
> spec_plan                    # generate implementation plan
> spec_tasks                   # break into atomic tasks
  ... implement tasks ...
> spec_status                  # verify all artifacts complete
` + "```" + `

Example — exploratory then structured:

` + "```" + `
> spec_init: title="Optimize Product List Fetching"
> spec_write with requirements:
  - The system SHALL reduce API response time below 200ms (P95)
  - The system SHALL support cursor-based pagination
  - The system MUST NOT break existing client integrations
> spec_plan
> spec_tasks
> spec_status
` + "```" + `
`
}

// specKitWorkflowSummary returns a concise markdown summary of the spec-kit workflow.
func specKitWorkflowSummary() string {
	return `**spec-kit** — specify → plan → tasks → implement

- ` + "`spec_init`" + ` — create numbered spec dir + blank spec.md
- ` + "`spec_write`" + ` — add user stories (P1/P2/P3), Given-When-Then scenarios
- ` + "`spec_plan`" + ` — scaffold plan.md (Phase 0-3, tech context, data model)
- ` + "`spec_tasks`" + ` — generate tasks.md (atomic tasks per story)
`
}

// openSpecWorkflowSummary returns a concise markdown summary of the OpenSpec workflow.
func openSpecWorkflowSummary() string {
	return `**openspec** — propose → apply → archive

- ` + "`spec_init`" + ` — create numbered spec dir
- ` + "`spec_write`" + ` — propose requirements (SHALL/MUST), entities, edge cases
- ` + "`spec_read`" + ` — review spec + artifact dependency graph
- ` + "`spec_plan`" + ` — generate implementation plan from spec
- ` + "`spec_tasks`" + ` — break plan into atomic tasks
- ` + "`spec_status`" + ` — scan artifact completion
`
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
// agent loop — useful for quickly kicking off a new feature before switching to
// agent mode for the rich spec_write / spec_plan / spec_tasks steps.
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
		if !entry.IsDir() {
			continue
		}
		dirPath := filepath.Join(specsRoot, entry.Name())
		present := map[specdriven.ArtifactKind]bool{}
		for _, kind := range []specdriven.ArtifactKind{
			specdriven.ArtifactSpec,
			specdriven.ArtifactPlan,
			specdriven.ArtifactTasks,
		} {
			fname := string(kind) + ".md"
			if _, err := os.Stat(filepath.Join(dirPath, fname)); err == nil {
				present[kind] = true
			}
		}
		_, _ = fmt.Fprint(w, specdriven.RenderStatus(entry.Name(), present))
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

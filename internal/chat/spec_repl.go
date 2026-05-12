package chat

import (
	"fmt"
	"io"
	"os"
	"path/filepath"
	"strings"

	"omnillm/internal/specdriven"
)

// specREPLInit is the implementation behind "/specify.init <title>".
// It creates the spec directory and spec.md directly without going through
// the agent loop — useful for quickly kicking off a new feature before
// switching to agent mode for the rich speckit_specify / speckit_plan /
// speckit_tasks steps.
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
	_, _ = fmt.Fprintln(w, "  2. Refine the spec via the agent:")
	_, _ = fmt.Fprintf(w, "     /speckit.specify %s\n", title)
	return nil
}

// openSpecREPLInit is the implementation behind "/opsx:init <change-name>".
// It scaffolds the OpenSpec change directory layout (proposal.md, design.md,
// tasks.md, specs/general/spec.md) without going through the agent loop.
func openSpecREPLInit(w io.Writer, changeName string) error {
	changeName = strings.TrimSpace(changeName)
	if changeName == "" {
		return fmt.Errorf("change name required")
	}
	slug := specdriven.Slugify(changeName)
	if slug == "" {
		return fmt.Errorf("change name produced empty slug")
	}

	changesRoot := filepath.Join("openspec", "changes")
	dirPath := filepath.Join(changesRoot, slug)
	if err := os.MkdirAll(dirPath, 0o755); err != nil {
		return fmt.Errorf("create dir: %w", err)
	}

	created := []string{}
	for _, art := range specdriven.OpenSpecArtifacts() {
		artPath := filepath.Join(dirPath, art.Filename)
		if err := os.MkdirAll(filepath.Dir(artPath), 0o755); err != nil {
			return fmt.Errorf("create dir for %s: %w", art.Filename, err)
		}
		if _, err := os.Stat(artPath); err == nil {
			continue // do not overwrite existing artifacts
		}
		body := fmt.Sprintf("# %s — %s\n\nTODO: fill in this artifact.\n",
			strings.ToTitle(art.ID), changeName)
		if err := os.WriteFile(artPath, []byte(body), 0o644); err != nil {
			return fmt.Errorf("write %s: %w", art.Filename, err)
		}
		created = append(created, artPath)
	}

	_, _ = fmt.Fprintf(w, "Created OpenSpec change scaffold at %s\n", dirPath)
	for _, p := range created {
		_, _ = fmt.Fprintf(w, "  + %s\n", p)
	}
	_, _ = fmt.Fprintln(w, "")
	_, _ = fmt.Fprintln(w, "Next steps:")
	_, _ = fmt.Fprintln(w, "  1. Switch to agent mode:  /mode agent")
	_, _ = fmt.Fprintf(w, "  2. Refine via the agent:  /opsx:propose %s\n", changeName)
	return nil
}

// specREPLStatus is the implementation behind "/speckit.status [dir]".
func specREPLStatus(w io.Writer, specsRoot string) error {
	entries, err := os.ReadDir(specsRoot)
	if err != nil {
		if os.IsNotExist(err) {
			_, _ = fmt.Fprintf(w, "No specs directory found at %q. Use /specify.init <title> to create one.\n", specsRoot)
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
		_, _ = fmt.Fprintf(w, "No spec directories in %q. Use /specify.init <title> to create one.\n", specsRoot)
	}
	return nil
}

// nextSpecNumber scans specsRoot for existing numbered directories and returns
// the next zero-padded number string. Mirrors the same helper in specdriven.go.
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

// specKitHelpMarkdown returns the help text for the Spec Kit workflow,
// shown by /speckit.help. It documents the lifecycle, the offline scaffold
// commands, and the full inventory of agent-routed Spec Kit commands.
func specKitHelpMarkdown() string {
	return `**Spec Kit workflow**

Spec Kit organises feature work as durable artifacts under ` + "`specs/<NNN>-<slug>/`" + `:

- ` + "`spec.md`" + ` — what & why (user stories, requirements, scenarios)
- ` + "`plan.md`" + ` — how (technical context, phases, contracts)
- ` + "`tasks.md`" + ` — ordered, dependency-aware implementation tasks
- ` + "`.speckit-state.json`" + ` — lifecycle metadata

**Lifecycle**

` + "`draft`" + ` -> ` + "`in_progress`" + ` -> ` + "`completed`" + ` -> ` + "`archived`" + `

After implementation: validate -> mark completed (` + "`/speckit.complete`" + `) -> optionally archive (` + "`/speckit.archive`" + `).

**Typical pipeline**

` + "`/specify.init <title>`" + ` (offline) -> ` + "`/speckit.specify`" + ` -> ` + "`/speckit.clarify`" + ` -> ` + "`/speckit.plan`" + ` -> ` + "`/speckit.tasks`" + ` -> ` + "`/speckit.implement`" + ` -> ` + "`/speckit.taskstoissues`" + ` (optional, GitHub) -> ` + "`/speckit.complete`" + ` -> ` + "`/speckit.archive`" + `

**Offline commands** (no LLM call; run instantly)

- ` + "`/specify.init <title>`" + ` — scaffold a new ` + "`specs/<NNN>-<slug>/spec.md`" + ` + lifecycle file
- ` + "`/speckit.status [dir]`" + ` — list specs and artifact-presence + lifecycle state across the repo
- ` + "`/speckit.help`" + ` — this help

**Agent-routed commands** (load the spec skill and run via the agent)

` + specdriven.RenderSpecKitCommandTable() + `

Tip: in the TUI, type ` + "`/speckit`" + ` to filter the picker to all Spec Kit commands.
`
}

// openSpecHelpMarkdown returns the help text for the OpenSpec workflow,
// shown by /openspec:help (alias /opsx:help). It documents the change-delta
// lifecycle and the full inventory of agent-routed OpenSpec commands.
func openSpecHelpMarkdown() string {
	return `**OpenSpec workflow**

OpenSpec manages requirements-driven changes under ` + "`openspec/changes/<change>/`" + `:

- ` + "`proposal.md`" + ` — why this change is being proposed
- ` + "`design.md`" + ` — technical design and trade-offs
- ` + "`specs/general/spec.md`" + ` — delta requirements (ADDED/MODIFIED/REMOVED)
- ` + "`tasks.md`" + ` — implementation work items

Completed changes are archived to ` + "`openspec/changes/archive/`" + ` and their delta specs are merged into ` + "`openspec/specs/`" + `.

**Lifecycle**

` + "`propose`" + ` -> ` + "`apply`" + ` -> ` + "`archive`" + ` (with ` + "`sync`" + ` to merge deltas)

**Offline commands** (no LLM call; run instantly)

- ` + "`/openspec:init <change-name>`" + ` — scaffold ` + "`openspec/changes/<slug>/`" + ` with proposal/design/tasks/spec stubs
- ` + "`/openspec:help`" + ` — this help

**Agent-routed commands** (load the spec skill and run via the agent)

` + specdriven.RenderOpenSpecCommandTable() + `

Profiles: **core** is the standard propose/apply/archive flow. **expanded** adds scaffolding and verification helpers.

**Deprecated aliases:** the previous ` + "`/opsx:*`" + ` namespace remains accepted for backwards compatibility but is no longer documented; please prefer ` + "`/openspec:*`" + `.

Tip: in the TUI, type ` + "`/openspec`" + ` to filter the picker to all OpenSpec commands.
`
}


package specdriven

import (
	"fmt"
	"strings"
)

// SpecKitCommand describes a core upstream Spec Kit command and its OmniCode
// agent-tool equivalent. The list is intentionally limited to core commands
// documented by github/spec-kit README/templates, excluding community extension
// commands.
type SpecKitCommand struct {
	Slash    string
	Tool     string
	Summary  string
	Artifact string
}

// SpecKitCommands returns the supported core Spec Kit command inventory.
func SpecKitCommands() []SpecKitCommand {
	return []SpecKitCommand{
		{Slash: "/speckit.constitution", Tool: "speckit_constitution", Summary: "create or update project principles", Artifact: "memory/constitution.md"},
		{Slash: "/speckit.specify", Tool: "speckit_specify", Summary: "create or update the feature specification", Artifact: "specs/<N>-<slug>/spec.md"},
		{Slash: "/speckit.clarify", Tool: "speckit_clarify", Summary: "capture targeted clarifications for the current spec", Artifact: "spec.md"},
		{Slash: "/speckit.plan", Tool: "speckit_plan", Summary: "create the technical implementation plan", Artifact: "plan.md"},
		{Slash: "/speckit.tasks", Tool: "speckit_tasks", Summary: "generate dependency-ordered implementation tasks", Artifact: "tasks.md"},
		{Slash: "/speckit.analyze", Tool: "speckit_analyze", Summary: "check spec/plan/tasks consistency", Artifact: "analysis.md"},
		{Slash: "/speckit.implement", Tool: "speckit_implement", Summary: "summarize or execute implementation tasks", Artifact: "tasks.md status"},
		{Slash: "/speckit.taskstoissues", Tool: "speckit_taskstoissues", Summary: "convert tasks.md into GitHub issues via `gh`", Artifact: "GitHub issues"},
		{Slash: "/speckit.lifecycle", Tool: "speckit_lifecycle_status", Summary: "show lifecycle state and next-step guidance", Artifact: ".speckit-state.json"},
		{Slash: "/speckit.complete", Tool: "speckit_complete", Summary: "mark a spec completed while preserving artifacts", Artifact: ".speckit-state.json"},
		{Slash: "/speckit.archive", Tool: "speckit_archive", Summary: "archive a completed spec under specs/archive/", Artifact: "specs/archive/<N>-<slug>/"},
		{Slash: "/speckit.checklist", Tool: "speckit_checklist", Summary: "generate a quality checklist", Artifact: "checklists/*.md"},
	}
}

// RenderSpecKitCommandTable returns a compact markdown table for help output.
func RenderSpecKitCommandTable() string {
	var sb strings.Builder
	sb.WriteString("| Command | Agent tool | Purpose | Artifact |\n")
	sb.WriteString("| --- | --- | --- | --- |\n")
	for _, cmd := range SpecKitCommands() {
		sb.WriteString(fmt.Sprintf("| `%s` | `%s` | %s | `%s` |\n", cmd.Slash, cmd.Tool, cmd.Summary, cmd.Artifact))
	}
	return sb.String()
}

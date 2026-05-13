package specdriven

import (
	"fmt"
	"strings"
)

// SpecCommand describes a slash command and its OmniCode agent-tool equivalent.
// It is the unified type for both Spec Kit and OpenSpec command inventories.
type SpecCommand struct {
	Slash     string
	Tool      string
	Summary   string
	Artifact  string
	Framework string // "spec-kit" or "openspec"
	Profile   string // e.g. "core", "expanded" (OpenSpec only)
	Prompt    string // rich workflow instructions injected into the agent prompt
}

var specKitCommands = []SpecCommand{
	{Slash: "/speckit.constitution", Tool: "speckit_constitution", Summary: "create or update project principles", Artifact: "memory/constitution.md", Framework: "spec-kit"},
	{Slash: "/speckit.specify", Tool: "speckit_specify", Summary: "create or update the feature specification", Artifact: "specs/<N>-<slug>/spec.md", Framework: "spec-kit"},
	{Slash: "/speckit.clarify", Tool: "speckit_clarify", Summary: "capture targeted clarifications for the current spec", Artifact: "spec.md", Framework: "spec-kit"},
	{Slash: "/speckit.plan", Tool: "speckit_plan", Summary: "create the technical implementation plan", Artifact: "plan.md", Framework: "spec-kit"},
	{Slash: "/speckit.tasks", Tool: "speckit_tasks", Summary: "generate dependency-ordered implementation tasks", Artifact: "tasks.md", Framework: "spec-kit"},
	{Slash: "/speckit.analyze", Tool: "speckit_analyze", Summary: "check spec/plan/tasks consistency", Artifact: "analysis.md", Framework: "spec-kit"},
	{Slash: "/speckit.implement", Tool: "speckit_implement", Summary: "summarize or execute implementation tasks", Artifact: "tasks.md status", Framework: "spec-kit"},
	{Slash: "/speckit.taskstoissues", Tool: "speckit_taskstoissues", Summary: "convert tasks.md into GitHub issues via `gh`", Artifact: "GitHub issues", Framework: "spec-kit"},
	{Slash: "/speckit.lifecycle", Tool: "speckit_lifecycle_status", Summary: "show lifecycle state and next-step guidance", Artifact: ".speckit-state.json", Framework: "spec-kit"},
	{Slash: "/speckit.complete", Tool: "speckit_complete", Summary: "mark a spec completed while preserving artifacts", Artifact: ".speckit-state.json", Framework: "spec-kit"},
	{Slash: "/speckit.archive", Tool: "speckit_archive", Summary: "archive a completed spec under specs/archive/", Artifact: "specs/archive/<N>-<slug>/", Framework: "spec-kit"},
	{Slash: "/speckit.checklist", Tool: "speckit_checklist", Summary: "generate a quality checklist", Artifact: "checklists/*.md", Framework: "spec-kit"},
}

// SpecKitCommands returns the supported core Spec Kit command inventory.
// Returned slice is a copy; callers may mutate it freely.
func SpecKitCommands() []SpecCommand {
	out := make([]SpecCommand, len(specKitCommands))
	copy(out, specKitCommands)
	return out
}

// RenderSpecKitCommandTable returns a compact markdown table for help output.
func RenderSpecKitCommandTable() string {
	var sb strings.Builder
	sb.WriteString("| Command | Agent tool | Purpose | Artifact |\n")
	sb.WriteString("| --- | --- | --- | --- |\n")
	for _, cmd := range specKitCommands {
		fmt.Fprintf(&sb, "| `%s` | `%s` | %s | `%s` |\n", cmd.Slash, cmd.Tool, cmd.Summary, cmd.Artifact)
	}
	return sb.String()
}

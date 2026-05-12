package specdriven

import (
	"fmt"
	"strings"
)

var openSpecCommands = []SpecCommand{
	{Slash: "/openspec:propose", Tool: "openspec_propose", Summary: "create a change and generate planning artifacts", Artifact: "openspec/changes/<change>/", Framework: "openspec", Profile: "core"},
	{Slash: "/openspec:explore", Tool: "openspec_explore", Summary: "investigate ideas before committing to a change", Artifact: "exploration notes", Framework: "openspec", Profile: "core"},
	{Slash: "/openspec:apply", Tool: "openspec_apply", Summary: "implement or report pending tasks from a change", Artifact: "tasks.md status", Framework: "openspec", Profile: "core"},
	{Slash: "/openspec:sync", Tool: "openspec_sync", Summary: "merge delta specs into main OpenSpec specs", Artifact: "openspec/specs/", Framework: "openspec", Profile: "core"},
	{Slash: "/openspec:archive", Tool: "openspec_archive", Summary: "archive a completed change", Artifact: "openspec/changes/archive/", Framework: "openspec", Profile: "core"},
	{Slash: "/openspec:new", Tool: "openspec_new", Summary: "start a new change scaffold", Artifact: ".openspec.yaml", Framework: "openspec", Profile: "expanded"},
	{Slash: "/openspec:continue", Tool: "openspec_continue", Summary: "create the next ready artifact", Artifact: "next missing artifact", Framework: "openspec", Profile: "expanded"},
	{Slash: "/openspec:ff", Tool: "openspec_ff", Summary: "fast-forward all planning artifacts", Artifact: "proposal/specs/design/tasks", Framework: "openspec", Profile: "expanded"},
	{Slash: "/openspec:verify", Tool: "openspec_verify", Summary: "validate implementation against artifacts", Artifact: "verification.md", Framework: "openspec", Profile: "expanded"},
	{Slash: "/openspec:bulk-archive", Tool: "openspec_bulk_archive", Summary: "archive multiple completed changes", Artifact: "archive folders", Framework: "openspec", Profile: "expanded"},
	{Slash: "/openspec:onboard", Tool: "openspec_onboard", Summary: "guided tutorial through the workflow", Artifact: "onboarding plan", Framework: "openspec", Profile: "expanded"},
}

// OpenSpecCommands returns the supported OpenSpec command inventory.
// Returned slice is a copy; callers may mutate it freely.
func OpenSpecCommands() []SpecCommand {
	out := make([]SpecCommand, len(openSpecCommands))
	copy(out, openSpecCommands)
	return out
}

// AllSpecCommands returns the merged Spec Kit + OpenSpec command inventory.
func AllSpecCommands() []SpecCommand {
	out := make([]SpecCommand, 0, len(specKitCommands)+len(openSpecCommands))
	out = append(out, specKitCommands...)
	out = append(out, openSpecCommands...)
	return out
}

// RenderOpenSpecCommandTable returns a compact markdown table for help output.
func RenderOpenSpecCommandTable() string {
	var sb strings.Builder
	sb.WriteString("| Command | Agent tool | Profile | Purpose | Artifact |\n")
	sb.WriteString("| --- | --- | --- | --- | --- |\n")
	for _, cmd := range openSpecCommands {
		fmt.Fprintf(&sb, "| `%s` | `%s` | %s | %s | `%s` |\n", cmd.Slash, cmd.Tool, cmd.Profile, cmd.Summary, cmd.Artifact)
	}
	return sb.String()
}

// OpenSpecArtifact describes one artifact in the default spec-driven OpenSpec
// workflow. Requires contains artifact IDs that should exist before creation.
type OpenSpecArtifact struct {
	ID               string
	Filename         string
	Requires         []string
	RequiredForApply bool
}

// OpenSpecArtifacts returns the default artifact graph used for built-in
// scaffolding. It mirrors OpenSpec's proposal/specs/design/tasks flow.
func OpenSpecArtifacts() []OpenSpecArtifact {
	return []OpenSpecArtifact{
		{ID: "proposal", Filename: "proposal.md", RequiredForApply: true},
		{ID: "specs", Filename: "specs/general/spec.md", Requires: []string{"proposal"}, RequiredForApply: true},
		{ID: "design", Filename: "design.md", Requires: []string{"proposal"}, RequiredForApply: true},
		{ID: "tasks", Filename: "tasks.md", Requires: []string{"specs", "design"}, RequiredForApply: true},
	}
}

package specdriven

import (
	"fmt"
	"strings"
)

// OpenSpecCommand describes an upstream OpenSpec slash command and its OmniCode
// agent-tool equivalent. The inventory follows OpenSpec docs/commands.md and
// includes core OPSX, expanded OPSX, and legacy /openspec:* commands.
type OpenSpecCommand struct {
	Slash    string
	Tool     string
	Summary  string
	Artifact string
	Profile  string
}

// OpenSpecCommands returns the supported OpenSpec command inventory.
func OpenSpecCommands() []OpenSpecCommand {
	return []OpenSpecCommand{
		{Slash: "/opsx:propose", Tool: "openspec_propose", Summary: "create a change and generate planning artifacts", Artifact: "openspec/changes/<change>/", Profile: "core"},
		{Slash: "/opsx:explore", Tool: "openspec_explore", Summary: "investigate ideas before committing to a change", Artifact: "exploration notes", Profile: "core"},
		{Slash: "/opsx:apply", Tool: "openspec_apply", Summary: "implement or report pending tasks from a change", Artifact: "tasks.md status", Profile: "core"},
		{Slash: "/opsx:sync", Tool: "openspec_sync", Summary: "merge delta specs into main OpenSpec specs", Artifact: "openspec/specs/", Profile: "core"},
		{Slash: "/opsx:archive", Tool: "openspec_archive", Summary: "archive a completed change", Artifact: "openspec/changes/archive/", Profile: "core"},
		{Slash: "/opsx:new", Tool: "openspec_new", Summary: "start a new change scaffold", Artifact: ".openspec.yaml", Profile: "expanded"},
		{Slash: "/opsx:continue", Tool: "openspec_continue", Summary: "create the next ready artifact", Artifact: "next missing artifact", Profile: "expanded"},
		{Slash: "/opsx:ff", Tool: "openspec_ff", Summary: "fast-forward all planning artifacts", Artifact: "proposal/specs/design/tasks", Profile: "expanded"},
		{Slash: "/opsx:verify", Tool: "openspec_verify", Summary: "validate implementation against artifacts", Artifact: "verification.md", Profile: "expanded"},
		{Slash: "/opsx:bulk-archive", Tool: "openspec_bulk_archive", Summary: "archive multiple completed changes", Artifact: "archive folders", Profile: "expanded"},
		{Slash: "/opsx:onboard", Tool: "openspec_onboard", Summary: "guided tutorial through the workflow", Artifact: "onboarding plan", Profile: "expanded"},
		{Slash: "/openspec:proposal", Tool: "openspec_legacy_proposal", Summary: "legacy all-at-once proposal workflow", Artifact: "openspec/changes/<change>/", Profile: "legacy"},
		{Slash: "/openspec:apply", Tool: "openspec_legacy_apply", Summary: "legacy apply workflow", Artifact: "tasks.md status", Profile: "legacy"},
		{Slash: "/openspec:archive", Tool: "openspec_legacy_archive", Summary: "legacy archive workflow", Artifact: "openspec/changes/archive/", Profile: "legacy"},
	}
}

// RenderOpenSpecCommandTable returns a compact markdown table for help output.
func RenderOpenSpecCommandTable() string {
	var sb strings.Builder
	sb.WriteString("| Command | Agent tool | Profile | Purpose | Artifact |\n")
	sb.WriteString("| --- | --- | --- | --- | --- |\n")
	for _, cmd := range OpenSpecCommands() {
		sb.WriteString(fmt.Sprintf("| `%s` | `%s` | %s | %s | `%s` |\n", cmd.Slash, cmd.Tool, cmd.Profile, cmd.Summary, cmd.Artifact))
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

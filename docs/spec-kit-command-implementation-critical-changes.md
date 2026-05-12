# Spec Kit Command Surface Critical Changes

## Context

OmniCode previously exposed a local `spec` skill through legacy `spec_*` agent tools and `/spec` helper commands. The upstream GitHub Spec Kit workflow documents a command surface using `/speckit.*` phase names. Users requested that OmniCode show and implement those commands directly, alongside the existing compatibility aliases.

## What changed

### Before

- `/spec` focused on OmniCode's legacy local workflow.
- The `spec` skill exposed only legacy tools such as `spec_init`, `spec_write`, `spec_plan`, and `spec_tasks`.
- Spec Kit phase names like `/speckit.specify` and `speckit_specify` were not discoverable as first-class commands/tools.

### After

- `/spec` and `/spec mode spec-kit` list the core Spec Kit commands.
- The slash-command catalog includes discoverable `/speckit.*` entries:
  - `/speckit.constitution`
  - `/speckit.specify`
  - `/speckit.clarify`
  - `/speckit.plan`
  - `/speckit.tasks`
  - `/speckit.analyze`
  - `/speckit.implement`
  - `/speckit.checklist`
- The `spec` skill registers matching agent tools:
  - `speckit_constitution`
  - `speckit_specify`
  - `speckit_clarify`
  - `speckit_plan`
  - `speckit_tasks`
  - `speckit_analyze`
  - `speckit_implement`
  - `speckit_checklist`
- Legacy `spec_*` tools remain registered for backward compatibility.

## Why this is critical

- The agent tool surface changed: loading the `spec` skill now activates additional tools beyond the legacy `spec_*` set.
- Slash command discovery changed: `/spec` now presents workflow selection and Spec Kit command inventory.
- Tool-count expectations changed because new Spec Kit-compatible tools are registered in the core registry.
- Compatibility must be preserved for existing sessions and scripts that call legacy `spec_*` tools.

## Affected files

- `internal/specdriven/speckit.go`
- `internal/chat/spec_repl.go`
- `internal/chat/repl.go`
- `internal/chat/slash_commands.go`
- `internal/chat/slash_commands_test.go`
- `internal/tools/specdriven.go`
- `internal/tools/specdriven_test.go`
- `internal/tools/groups.go`
- `internal/tools/tools_test.go`
- `specs/001-spec-kit-commands-implementation/spec.md`
- `specs/001-spec-kit-commands-implementation/plan.md`
- `specs/001-spec-kit-commands-implementation/tasks.md`

## Commit range

Uncommitted working-tree changes in the current feature branch/session.

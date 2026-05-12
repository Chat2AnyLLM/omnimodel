# Plan: 001 Spec Kit Commands Implementation

**Created**: 2026-05-12T04:16:25Z

## Technical Context

- **Language**: Go
- **Framework**: Cobra-style CLI / internal chat REPL and agent tool registry
- **Database**: N/A
- **Deployment**: OmniCode CLI binary
- **Performance goals**:
  - Help and picker entries remain static and fast.
  - Tool commands perform small local filesystem operations only.
- **Constraints**:
  - Preserve existing `spec_*` tools and tests.
  - Use PowerShell-compatible behavior on Windows where shell examples are needed.
  - Do not require installing upstream Spec Kit CLI to use OmniCode's local workflow.

## Command Inventory from Spec Kit

Verified from `github/spec-kit` README and `templates/commands` in the upstream repository. Core command set:

| Upstream command | OmniCode slash visibility | Agent tool implementation |
| --- | --- | --- |
| `/speckit.constitution` | show in picker/help | `speckit_constitution` |
| `/speckit.specify` | show in picker/help | `speckit_specify` |
| `/speckit.clarify` | show in picker/help | `speckit_clarify` |
| `/speckit.plan` | show in picker/help | `speckit_plan` |
| `/speckit.tasks` | show in picker/help | `speckit_tasks` |
| `/speckit.analyze` | show in picker/help | `speckit_analyze` |
| `/speckit.implement` | show in picker/help | `speckit_implement` |
| `/speckit.checklist` | show in picker/help | `speckit_checklist` |

Note: upstream currently also has `taskstoissues` template and extension commands. This implementation focuses on README/core workflow commands plus the core template commands above.

## Phases

### Phase 0 - Research

- Fetch/inspect upstream Spec Kit README and command templates.
- Determine core commands vs extension/community commands.
- Map slash commands to valid Go tool names (`/speckit.plan` -> `speckit_plan`).

### Phase 1 - Design

- Add a `SpecKitCommand` inventory in `internal/specdriven`.
- Update `/spec` help and spec-kit workflow summary to display the inventory.
- Add slash picker entries for `/speckit.*` and `/spec speckit *` discovery.
- Add tool wrappers for Spec Kit-compatible names:
  - `speckit_specify` wraps init/write-oriented creation.
  - `speckit_plan` wraps or delegates to existing `spec_plan` behavior.
  - `speckit_tasks` wraps or delegates to existing `spec_tasks` behavior.
  - `speckit_constitution`, `speckit_clarify`, `speckit_analyze`, `speckit_implement`, and `speckit_checklist` create/check local artifacts and provide explicit guidance.

### Phase 2 - Setup

- Register new tools in `RegisterCoreTools` and `InitSkillMembership` under `SkillSpec`.
- Update `load_skill` description and tests for increased tool count.

### Phase 3 - Implementation

- Implement new tool types in `internal/tools/specdriven.go`.
- Add render helpers or artifact-kind support as needed.
- Update `internal/chat/spec_repl.go`, `repl.go`, and `slash_commands.go`.
- Add tests for command inventory, registration, help text, picker suggestions, and basic tool execution.

## Data Model

- `SpecKitCommand`
  - `Slash`: string such as `/speckit.plan`
  - `Tool`: Go tool name such as `speckit_plan`
  - `Summary`: one-line description
  - `Artifact`: expected output artifact or status
  - `Core`: boolean for core support

## API Contracts

### Agent tools

- `speckit_constitution(principles?: string, constitution_path?: string)`
- `speckit_specify(feature: string, title?: string, spec_dir?: string, specs_dir?: string)`
- `speckit_clarify(spec_dir?: string, clarifications?: string[])`
- `speckit_plan(spec_dir?: string, language?: string, framework?: string, database?: string, deployment?: string, perf_goals?: string[], constraints?: string[])`
- `speckit_tasks(spec_dir?: string)`
- `speckit_analyze(spec_dir?: string)`
- `speckit_implement(spec_dir?: string, dry_run?: bool)`
- `speckit_checklist(spec_dir?: string, purpose?: string, items?: string[])`

### Slash command behavior

- `/spec` shows workflow chooser and core Spec Kit command table.
- `/spec mode spec-kit` switches to agent mode and shows Spec Kit command table.
- `/speckit.*` typed directly in REPL prints guidance to use agent tools or `/spec mode spec-kit`.

## Validation

- Run `go test ./internal/specdriven ./internal/tools ./internal/chat`.
- Confirm tests cover:
  - new commands in inventory/help
  - skill activation count
  - tool registration
  - representative artifact creation

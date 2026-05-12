# SpecKit Clean Lifecycle

## Context

SpecKit work creates durable planning artifacts in `specs/<N>-<slug>/`: `spec.md`, `plan.md`, and `tasks.md`. Users needed clear guidance for what to do with those files after implementation finishes, without deleting traceability records.

## Lifecycle

OmniCode now treats SpecKit folders as moving through these states:

1. `draft` - the spec exists and requirements are being refined.
2. `in_progress` - implementation planning or task execution has started.
3. `completed` - implementation is done and validated; artifacts remain in place.
4. `archived` - the completed folder has been moved under `specs/archive/`.

Lifecycle metadata is stored per spec folder in `.speckit-state.json`.

## What Changed

### Before

- SpecKit artifacts existed as plain markdown files with no explicit lifecycle status.
- After implementation, users had to decide manually whether to keep, delete, or move `spec.md`, `plan.md`, and `tasks.md`.
- Archive conflict handling was not documented for SpecKit folders.

### After

- New SpecKit specs initialize with lifecycle state `draft`.
- Generating tasks moves a draft spec to `in_progress`.
- `speckit_lifecycle_status` reports artifact presence, state, timestamps, notes, follow-ups, and next-step guidance.
- `speckit_complete` marks a spec `completed` while preserving `spec.md`, `plan.md`, and `tasks.md`.
- `speckit_archive` moves completed specs to `specs/archive/` and chooses a unique destination instead of overwriting existing archive folders.
- `/spec` and spec-kit workflow help explain the clean lifecycle and after-implementation flow: validate -> complete -> optionally archive.

## Why This Is Critical

This changes SpecKit's repository lifecycle and introduces a canonical archival convention. It affects how users maintain historical spec records and how agent workflows should treat finished specs.

## Affected Files

- `internal/specdriven/lifecycle.go`
- `internal/specdriven/speckit.go`
- `internal/tools/specdriven.go`
- `internal/tools/groups.go`
- `internal/chat/spec_repl.go`
- `internal/chat/slash_commands.go`
- `internal/specdriven/specdriven_test.go`
- `internal/tools/specdriven_test.go`
- `internal/chat/slash_commands_test.go`
- `docs/SPECKIT_CLEAN_LIFECYCLE.md`

## Commit Range

Pending local changes in the current working tree for spec `003-speckit-clean-lifecycle`.

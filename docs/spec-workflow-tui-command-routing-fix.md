# Spec Workflow TUI Command Routing Fix

## Context

Users could see spec workflow aliases in slash-command help and picker, but executing commands like `/speckit.specify` or `/opsx:explore` in the TUI produced an unknown-command error.

The non-TUI REPL already routed these aliases through `handleSpecWorkflowSlashCommand`, but the TUI `handleSlash` default branch did not share that routing path.

## What Changed

### Before

- `/speckit.*`, `/opsx:*`, and `/openspec:*` aliases were listed in slash-command metadata.
- REPL execution returned workflow guidance and switched to agent/spec mode.
- TUI execution fell through to:
  - `Unknown command: /speckit.specify -- use /help`
  - `Unknown command: /opsx:explore -- use /help`

### After

- TUI unknown-command fallback first attempts `handleSpecWorkflowSlashCommand`.
- Recognized spec workflow aliases now:
  - switch TUI mode to `agent`
  - set `SpecMode` to `spec-kit` or `openspec`
  - render guidance showing the mapped spec tool
  - save updated config
- Unknown non-spec commands still report the normal unknown-command error.

## Why This Is Critical

The commands were advertised as available but could not be executed from the main TUI path. This blocked users from using the implemented spec-kit and OpenSpec workflows from the primary OmniCode interface.

## Affected Files

- `internal/chat/tui.go`
  - Routes TUI `/speckit.*`, `/opsx:*`, and `/openspec:*` aliases through the shared spec workflow handler.
- `internal/chat/tui_slash_picker_test.go`
  - Adds regression coverage for TUI execution of spec-kit and OpenSpec aliases.

## Validation

- `go test ./...` passes.

## Commit Range

Pending commit.

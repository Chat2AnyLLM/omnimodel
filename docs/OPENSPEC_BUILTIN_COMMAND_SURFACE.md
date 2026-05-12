# OpenSpec Built-in Command Surface

## Context

OmniCode already supported a built-in `spec` skill and Spec Kit style commands. The OpenSpec workflow was requested as a parallel spec-driven method so users can select either Spec Kit or OpenSpec from `/spec` and then use OpenSpec-style slash commands and agent tools.

## What changed

### Before

- `/spec` primarily exposed the Spec Kit workflow.
- The `spec` skill did not include OpenSpec command tools.
- OpenSpec slash commands such as `/opsx:propose` and `/openspec:proposal` were not discoverable from the TUI command picker.

### After

- `/spec` lets users choose `spec-kit` or `openspec` mode.
- OpenSpec command help is rendered from a shared command inventory.
- The TUI slash picker includes OpenSpec commands:
  - `/opsx:propose`
  - `/opsx:explore`
  - `/opsx:apply`
  - `/opsx:sync`
  - `/opsx:archive`
  - `/opsx:new`
  - `/opsx:continue`
  - `/opsx:ff`
  - `/opsx:verify`
  - `/opsx:bulk-archive`
  - `/opsx:onboard`
  - legacy `/openspec:proposal`, `/openspec:apply`, `/openspec:archive`
- The `spec` skill now registers matching OpenSpec tools:
  - `openspec_propose`, `openspec_explore`, `openspec_new`, `openspec_continue`, `openspec_ff`
  - `openspec_apply`, `openspec_verify`, `openspec_sync`, `openspec_archive`
  - `openspec_bulk_archive`, `openspec_onboard`
  - legacy wrapper tools for `/openspec:*`
- Rendering now clamps the TUI output to the terminal height, preventing the expanded slash command catalog from overflowing small windows.

## Why this is critical

This changes the built-in tool surface and spec-driven workflow contract. Users and agents can now create and manage OpenSpec artifacts directly through OmniCode without relying on an external OpenSpec CLI. It also changes discoverability and mode selection behavior under `/spec`.

## Affected files

- `internal/specdriven/openspec.go`
- `internal/tools/specdriven.go`
- `internal/tools/groups.go`
- `internal/chat/repl.go`
- `internal/chat/spec_repl.go`
- `internal/chat/slash_commands.go`
- `internal/chat/tui.go`
- `internal/chat/*slash*_test.go`
- `internal/tools/tools_test.go`
- `specs/002-openspec-commands-implementation/`

## Commit range

Uncommitted working-tree change at the time of documentation.

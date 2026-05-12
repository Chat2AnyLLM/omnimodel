# Plan: 002 OpenSpec Commands Implementation

**Created**: 2026-05-12T00:00:00Z

## Technical Context

- **Language**: Go
- **Framework**: Cobra/TUI internal command catalog and OmniCode tool registry
- **Database**: Filesystem markdown artifacts
- **Deployment**: Built-in agent skill, no external OpenSpec CLI required
- **Constraint**: Keep implementation compatible with existing `spec` skill and Spec Kit tools.
- **Constraint**: Use deterministic, safe filesystem operations.

## Phase 0: Research

- Upstream OpenSpec command inventory from `docs/commands.md`:
  - Core: `/opsx:propose`, `/opsx:explore`, `/opsx:apply`, `/opsx:sync`, `/opsx:archive`
  - Expanded: `/opsx:new`, `/opsx:continue`, `/opsx:ff`, `/opsx:verify`, `/opsx:bulk-archive`, `/opsx:onboard`
  - Legacy: `/openspec:proposal`, `/openspec:apply`, `/openspec:archive`
- Canonical artifact tree: `openspec/changes/<change>/` with `.openspec.yaml`, `proposal.md`, `design.md`, `tasks.md`, and `specs/<area>/spec.md`.

## Phase 1: Design

- Add `internal/specdriven/openspec.go` for command inventory and artifact renderers.
- Extend `internal/tools/specdriven.go` with OpenSpec-compatible tools mirroring Spec Kit wrappers.
- Extend `internal/tools/groups.go` registration and spec skill membership.
- Extend `internal/chat/slash_commands.go` and help output with OpenSpec commands.
- Add tests for inventory, slash command discoverability, tool registration, and artifact creation.

## Phase 2: Setup

- Use current `specs/002-openspec-commands-implementation/` as the feature record.
- Avoid new third-party dependencies.

## Phase 3: Implementation

- Implement command metadata table.
- Implement artifact helpers:
  - change name derivation
  - change directory creation
  - proposal/design/tasks/spec delta rendering
  - archive destination uniquing
- Implement tools:
  - `openspec_propose`, `openspec_explore`, `openspec_new`, `openspec_continue`, `openspec_ff`
  - `openspec_apply`, `openspec_verify`, `openspec_sync`, `openspec_archive`, `openspec_bulk_archive`, `openspec_onboard`
  - legacy wrappers `openspec_legacy_proposal`, `openspec_legacy_apply`, `openspec_legacy_archive`
- Update help and docs.
- Validate with focused Go tests.

# Tasks: 002 OpenSpec Commands Implementation

Legend: `[ ]` pending - `[~]` in progress - `[x]` done - `[P]` parallelizable

## SETUP - Project Setup & Dependencies

- [x] **T001** Capture OpenSpec command inventory from upstream docs
- [x] **T002** Create spec and plan artifacts for this change

## US1 - Discover OpenSpec commands

- [x] **T003** [P] Add OpenSpec command inventory renderer in `internal/specdriven/openspec.go`
- [x] **T004** [P] Add OpenSpec slash commands to TUI catalog
- [x] **T005** [P] Update `/spec` help and OpenSpec mode summary
- [x] **T006** Add slash command tests for `/opsx` and `/openspec`

## US2 - Create and continue OpenSpec changes

- [x] **T007** Add OpenSpec artifact scaffolding helpers
- [x] **T008** Implement `openspec_propose`, `openspec_explore`, `openspec_new`, `openspec_continue`, and `openspec_ff`
- [x] **T009** Add tests for propose/new/continue artifact creation

## US3 - Apply, verify, sync, and archive OpenSpec changes

- [x] **T010** Implement `openspec_apply`, `openspec_verify`, `openspec_sync`, `openspec_archive`, `openspec_bulk_archive`, and `openspec_onboard`
- [x] **T011** Implement legacy `/openspec:*` wrappers
- [x] **T012** Register all OpenSpec tools and update skill membership/tool count tests
- [x] **T013** Document the critical tool-surface change
- [x] **T014** Run focused tests and fix failures

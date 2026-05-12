# Tasks: 001 Spec Kit Commands Implementation

Legend: `[ ]` pending · `[~]` in progress · `[x]` done · `[P]` parallelizable

## SETUP - Research and inventory

- [x] **T001** Verify upstream Spec Kit README command list and templates — upstream github/spec-kit
- [x] **T002** Write spec.md for command implementation — specs/001-spec-kit-commands-implementation/spec.md
- [x] **T003** Write plan.md with command mapping — specs/001-spec-kit-commands-implementation/plan.md

## US1 - Discover supported Spec Kit commands

- [x] **T004** [P] Add SpecKitCommand inventory and markdown render helper — internal/specdriven/speckit.go
- [x] **T005** Update `/spec` help and spec-kit summary with core `/speckit.*` commands — internal/chat/spec_repl.go
- [x] **T006** Update slash command catalog and picker filtering for `/speckit.*` commands — internal/chat/slash_commands.go
- [x] **T007** Add REPL guidance for direct `/speckit.*` input — internal/chat/repl.go

## US2 - Invoke Spec Kit-compatible agent tools

- [x] **T008** Add `speckit_constitution` tool — internal/tools/specdriven.go
- [x] **T009** Add `speckit_specify` tool using existing spec init/write behavior — internal/tools/specdriven.go
- [x] **T010** Add `speckit_clarify` tool to append clarifications to spec artifacts — internal/tools/specdriven.go
- [x] **T011** Add `speckit_plan` and `speckit_tasks` wrappers/delegates — internal/tools/specdriven.go
- [x] **T012** Add `speckit_analyze`, `speckit_implement`, and `speckit_checklist` tools — internal/tools/specdriven.go
- [x] **T013** Register new tools and add them to the `spec` skill — internal/tools/groups.go, internal/tools/load_skill.go

## US3 - Preserve compatibility

- [x] **T014** Keep legacy `spec_*` tools registered and tests passing — internal/tools/specdriven_test.go
- [x] **T015** Update tool count and skill activation tests — internal/tools/tools_test.go, internal/tools/specdriven_test.go

## VALIDATION

- [x] **T016** [P] Add/update chat slash command tests — internal/chat/slash_commands_test.go
- [x] **T017** [P] Add tool execution tests for new Spec Kit tools — internal/tools/specdriven_test.go
- [x] **T018** Run `go test ./internal/specdriven ./internal/tools ./internal/chat`
- [x] **T019** Document critical behavior change in docs/ — docs/spec-kit-command-implementation-critical-changes.md

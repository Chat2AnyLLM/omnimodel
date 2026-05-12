# Tasks: 003 SpecKit Clean Lifecycle

Legend: `[ ]` pending · `[~]` in progress · `[x]` done · `[P]` parallelizable

## SETUP - Define lifecycle approach

- [x] **T001** Create `spec.md` for the SpecKit clean lifecycle feature
- [x] **T002** Create `plan.md` describing lifecycle states, metadata, and archive flow
- [x] **T003** Inspect current spec tooling to choose lifecycle metadata format and insertion points

## US1 - Understand the current lifecycle state

- [x] **T004** [P] Define lifecycle state model and metadata helpers in `internal/specdriven`
- [x] **T005** [P] Initialize lifecycle state for newly created SpecKit specs
- [x] **T006** Add status-reporting tool and/or summary output for lifecycle guidance
- [x] **T007** Update `/spec` help text to explain the clean lifecycle

## US2 - Complete a spec without deleting its records

- [x] **T008** Implement completion operation that marks specs as completed while preserving artifacts
- [x] **T009** Support optional completion notes and follow-up references
- [x] **T010** Add tests for completion behavior and preserved files

## US3 - Archive completed specs cleanly when desired

- [x] **T011** Implement archive operation moving completed specs to `specs/archive/`
- [x] **T012** Add non-destructive unique destination handling for archive conflicts
- [x] **T013** Add tests for archive success, warnings, and conflict handling

## VALIDATION

- [x] **T014** [P] Add/update documentation describing the lifecycle: draft -> in progress -> completed -> archived
- [x] **T015** [P] Run focused tests for lifecycle helpers, tools, and help text
- [x] **T016** Manually verify the workflow on a sample spec folder

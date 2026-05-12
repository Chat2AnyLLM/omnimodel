# Plan: 003 SpecKit Clean Lifecycle

**Created**: 2026-05-12T13:17:46+08:00

## Technical Context

- **Language**: Go
- **Framework**: OmniCode internal spec tooling, chat help, and filesystem-based artifacts
- **Database**: N/A; lifecycle metadata stored in the repo filesystem
- **Deployment**: OmniCode CLI binary and local working tree
- **Constraints**:
  - Preserve existing `specs/<N>-<slug>/spec.md`, `plan.md`, and `tasks.md` structure.
  - Keep the workflow simple and understandable without requiring external tools.
  - Prefer additive metadata and non-destructive moves.

## Desired Lifecycle

Canonical SpecKit lifecycle:

1. **draft**
   - Spec exists, requirements are being written/refined.
2. **in_progress**
   - Plan/tasks are being executed or implementation has started.
3. **completed**
   - Implementation is done and validated; spec folder remains intact as a record.
4. **archived**
   - Completed spec has been moved to `specs/archive/` to reduce clutter.

## Design Decisions

### Lifecycle metadata

Use a small metadata file per spec folder, for example `specs/<N>-<slug>/status.md` or `.speckit-state.json`.

Preferred option: human-readable markdown, e.g. `status.md`, containing:

- current state
- created/completed/archived timestamps
- optional notes
- optional follow-up references

Reasoning:
- easy for users and agents to read/edit
- aligns with markdown-first artifact style
- reduces need for a separate parser-heavy format

### Completion behavior

Completing a spec should:
- leave `spec.md`, `plan.md`, and `tasks.md` in place
- mark lifecycle state as `completed`
- allow optional completion notes
- optionally summarize deferred work or deviations

### Archive behavior

Archiving a spec should:
- require or strongly prefer `completed` status first
- move the full folder to `specs/archive/<N>-<slug>/`
- preserve every artifact in the folder
- use a unique suffix if the archive destination already exists

### Tooling surface

Potential additions under the `spec` skill:
- `speckit_status` or `speckit_lifecycle_status`
- `speckit_complete`
- `speckit_archive`

Also update `/spec` help text with a short lifecycle guide:
- after implementation: validate -> mark completed -> optionally archive

## Phases

### Phase 0 - Research

- Inspect current spec artifact conventions in the repo.
- Decide whether to use markdown or JSON metadata.
- Confirm whether existing tools already imply status that can be reused.

### Phase 1 - Design

- Define lifecycle states and metadata schema.
- Define archive path convention.
- Define help text and user guidance updates.

### Phase 2 - Implementation

- Add lifecycle metadata helpers in `internal/specdriven`.
- Add or update tools for status, complete, and archive operations.
- Update help/docs to explain the workflow.
- Ensure spec creation initializes lifecycle state.

### Phase 3 - Validation

- Add tests for status initialization, completion, and archive moves.
- Validate non-destructive archive conflict handling.
- Validate help text/documentation output.

## Data Model

### SpecLifecycleRecord

Fields:
- `state`: `draft | in_progress | completed | archived`
- `created_at`: timestamp
- `updated_at`: timestamp
- `completed_at`: optional timestamp
- `archived_at`: optional timestamp
- `notes`: optional markdown text
- `follow_ups`: optional list of follow-up references

## API / Tool Contracts

### `speckit_lifecycle_status`

Inputs:
- `spec_dir?: string`

Outputs:
- current lifecycle state
- detected artifact summary
- recommended next step

### `speckit_complete`

Inputs:
- `spec_dir?: string`
- `notes?: string`
- `follow_ups?: string[]`

Outputs:
- status updated to completed
- preserved artifact summary
- completion note path/details

### `speckit_archive`

Inputs:
- `spec_dir?: string`
- `force?: bool`

Outputs:
- archive destination path
- archived status confirmation
- warning if force was needed for non-completed spec

## Validation

- Run focused Go tests for `internal/specdriven`, `internal/tools`, and `internal/chat`.
- Manually verify a sample spec can move draft -> in_progress -> completed -> archived.
- Confirm docs clearly answer: “What should I do with `spec.md`, `plan.md`, and `tasks.md` after implementation?”

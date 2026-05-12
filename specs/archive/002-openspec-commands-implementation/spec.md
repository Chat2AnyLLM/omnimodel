# Spec: OpenSpec Commands Implementation

**Created**: 2026-05-12T00:00:00Z

## Overview

Implement OpenSpec-compatible command support analogous to the existing Spec Kit command implementation. OmniCode should expose all documented OpenSpec slash-command equivalents as agent tools and TUI slash-picker entries, without requiring the external OpenSpec TypeScript CLI.

## User Stories

### US1 - Discover OpenSpec commands (P1)

As an OmniCode user in OpenSpec mode, I want to see all supported OpenSpec commands so that I can choose the right workflow step.

**Why this priority**: Discoverability is required before the workflow can be used.

**Acceptance Scenarios**:

#### Slash picker includes OpenSpec commands

- **GIVEN** I type `/opsx` or `/openspec` in the TUI slash picker
- **WHEN** choices are filtered
- **THEN** I see supported OpenSpec/OPSX commands with summaries

#### Spec help includes OpenSpec table

- **GIVEN** I run `/spec` or `/spec mode openspec`
- **WHEN** help is rendered
- **THEN** the output includes the OpenSpec command inventory and agent-tool equivalents

### US2 - Create and continue OpenSpec changes (P1)

As an agent, I want OpenSpec tools for proposing, exploring, scaffolding, and generating planning artifacts so that I can create `openspec/changes/<change>/` workflows.

**Why this priority**: The propose/new/continue/ff flow is the core OpenSpec planning path.

**Acceptance Scenarios**:

#### Propose creates all planning artifacts

- **GIVEN** I call the OpenSpec propose tool with a change description
- **WHEN** the tool executes
- **THEN** it creates `proposal.md`, `design.md`, `tasks.md`, delta specs, and metadata under `openspec/changes/<change>/`

#### Continue creates next missing artifact

- **GIVEN** a change exists with metadata but missing artifacts
- **WHEN** I call the OpenSpec continue tool
- **THEN** it creates the next ready artifact according to dependency order

### US3 - Apply, verify, sync, and archive OpenSpec changes (P1)

As an agent, I want lifecycle tools for implementation readiness, verification, syncing deltas, and archiving so that OpenSpec changes can be driven through completion.

**Why this priority**: Planning artifacts are not enough; users need lifecycle completion commands.

**Acceptance Scenarios**:

#### Apply reports pending tasks

- **GIVEN** a change has a `tasks.md`
- **WHEN** I call the OpenSpec apply tool
- **THEN** it reports pending tasks and implementation guidance

#### Archive moves completed change

- **GIVEN** a change exists under `openspec/changes/<change>`
- **WHEN** I call archive
- **THEN** the change moves to `openspec/changes/archive/<date>-<change>` and preserves artifacts

## Functional Requirements

- **FR-001** [US1]: The system SHALL list OpenSpec commands in the slash-command catalog.
- **FR-002** [US1]: The system SHALL render an OpenSpec command table analogous to the Spec Kit table.
- **FR-003** [US2]: The system SHALL provide agent tools for `/opsx:propose`, `/opsx:explore`, `/opsx:new`, `/opsx:continue`, and `/opsx:ff`.
- **FR-004** [US3]: The system SHALL provide agent tools for `/opsx:apply`, `/opsx:verify`, `/opsx:sync`, `/opsx:archive`, `/opsx:bulk-archive`, and `/opsx:onboard`.
- **FR-005** [US3]: The system SHALL provide legacy OpenSpec-compatible agent tools for `/openspec:proposal`, `/openspec:apply`, and `/openspec:archive`.
- **FR-006** [US2]: The system SHALL write OpenSpec artifacts to the canonical `openspec/changes/<change>/` tree.
- **FR-007** [US3]: The system SHALL avoid destructive overwrites unless explicitly requested or the operation is an archive move.

## Key Entities

### OpenSpecChange

A change directory under `openspec/changes/<change>/` containing metadata and workflow artifacts.

Fields: name, root_dir, change_dir, created_at, schema

### OpenSpecArtifact

A planning or lifecycle artifact in a change.

Fields: id, filename, dependencies, required_for_apply

## Edge Cases

- **EC-001**: Missing change name -> derive a safe kebab-case name from description or return a clear error.
- **EC-002**: Missing `tasks.md` during apply -> return guidance to run propose/ff/continue first.
- **EC-003**: Existing archive destination -> choose a unique suffixed archive folder instead of overwriting.

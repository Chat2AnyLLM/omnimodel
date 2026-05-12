# Spec: 003 SpecKit Clean Lifecycle

**Created**: 2026-05-12T13:17:46+08:00

## Overview

Define and implement a clean lifecycle for SpecKit artifacts so completed work has a predictable, low-clutter home and users know what to do with `spec.md`, `plan.md`, and `tasks.md` after implementation. OmniCode should support an explicit lifecycle of draft -> in progress -> completed -> archived for `specs/<N>-<slug>/` folders, with guidance and lightweight tooling that preserves project history.

## User Stories

### US1 - Understand the current lifecycle state (P1)

As a user working in a `specs/<N>-<slug>/` folder, I want to know whether a spec is draft, in progress, completed, or archived so I can manage work consistently.

**Why P1**: Users need a clear lifecycle before they can follow or automate it.

#### Acceptance Scenarios

- **Given** a spec folder exists under `specs/`, **when** I inspect its lifecycle metadata or summary, **then** I can determine its current state.
- **Given** a spec is newly created, **when** no implementation has started yet, **then** its initial lifecycle state is draft.
- **Given** implementation work has begun, **when** tasks are actively being executed, **then** the state can be marked in progress.

### US2 - Complete a spec without deleting its records (P1)

As a user who finished implementation, I want to mark the spec as completed while keeping `spec.md`, `plan.md`, and `tasks.md` in the repo so the work remains traceable.

**Why P1**: The main user need is knowing what to do with the files after implementation.

#### Acceptance Scenarios

- **Given** all tasks are done, **when** I complete the lifecycle step, **then** the spec folder remains intact and is marked completed.
- **Given** a completed spec has deviations or follow-ups, **when** I finalize it, **then** I can record completion notes or next steps.
- **Given** a completed spec remains under `specs/`, **when** another user reviews the repo, **then** they can still read the original `spec.md`, `plan.md`, and `tasks.md`.

### US3 - Archive completed specs cleanly when desired (P2)

As a user maintaining a growing repository, I want an archive convention for completed SpecKit folders so active work stays uncluttered while historical records remain accessible.

**Why P2**: Archiving is useful but secondary to defining the active workflow.

#### Acceptance Scenarios

- **Given** a completed spec is ready to be moved out of the active list, **when** I archive it, **then** it moves to a predictable archive location without losing artifacts.
- **Given** archived specs exist, **when** I browse the repository, **then** active specs and archived specs are clearly separated.
- **Given** an archive destination conflicts with an existing folder, **when** archiving occurs, **then** the system chooses a non-destructive unique destination.

## Functional Requirements

- **FR-001 [US1]** The system SHALL define the SpecKit lifecycle states as `draft`, `in_progress`, `completed`, and `archived`.
- **FR-002 [US1]** The system SHALL store lifecycle status for each spec in a predictable, repo-local format.
- **FR-003 [US1]** Newly created SpecKit specs SHALL default to `draft` unless explicitly set otherwise.
- **FR-004 [US2]** The system SHALL support marking a spec as `completed` without deleting `spec.md`, `plan.md`, or `tasks.md`.
- **FR-005 [US2]** The system SHALL support recording completion notes, follow-ups, or implementation deviations for a completed spec.
- **FR-006 [US3]** The system SHALL support archiving a completed spec into a dedicated archive location under `specs/archive/` or equivalent documented structure.
- **FR-007 [US3]** Archiving SHALL preserve all spec artifacts and avoid destructive overwrites.
- **FR-008 [US3]** The documented workflow SHALL explain what users should do at each lifecycle step, especially after implementation is finished.

## Key Entities

- **SpecLifecycleState**: one of `draft`, `in_progress`, `completed`, `archived`.
- **SpecLifecycleRecord**: metadata associated with a spec folder, including current status, timestamps, and optional notes.
- **SpecArchiveLocation**: destination path for completed spec folders moved out of the active `specs/` listing.

## Edge Cases

- **EC-001**: If a user attempts to archive a spec that is not completed, the system should warn or require explicit confirmation/promotion.
- **EC-002**: If a spec folder is missing `plan.md` or `tasks.md`, the lifecycle system should still handle status changes and report missing artifacts clearly.
- **EC-003**: If `specs/archive/<name>` already exists, the archive operation should create a unique suffixed path instead of overwriting history.

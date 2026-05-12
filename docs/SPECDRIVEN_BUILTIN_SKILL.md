# Spec-Driven Development: Built-in OmniCode Skill

## Context

Consolidated concepts from two external repositories:

- **spec-kit** (`~/repos/spec-kit`) — Python CLI implementing Spec-Driven Development
  (SDD): prioritised user stories, Given-When-Then acceptance scenarios, plan phases,
  atomic tasks. Introduced the `specify → plan → tasks → implement` pipeline.

- **OpenSpec** (`~/repos/OpenSpec`) — TypeScript framework with `ArtifactGraph`
  (topological dependency ordering), Change/Delta system (ADDED/MODIFIED/REMOVED
  requirement deltas), and a lightweight `propose → apply → archive` workflow.

The goal was to add spec-driven as a **built-in "spec" skill** in the OmniCode agent,
without requiring any external tooling.

---

## What Changed

### New Package: `internal/specdriven/`

Pure Go data model and rendering layer. No external dependencies.

| File | Purpose |
|---|---|
| `spec.go` | `Spec`, `UserStory`, `Scenario`, `Requirement`, `Entity`, `EdgeCase`, `SpecStore` |
| `plan.go` | `Plan`, `TechnicalContext`, `PlanPhase`, `APIContract`, `RenderSpec()`, `RenderPlan()` |
| `tasks.go` | `SpecTask`, `TaskGroup`, `ArtifactGraph`, `ScaffoldTaskGroups()`, `RenderTasks()`, `RenderStatus()` |

Key design decisions:
- `SpecStore` is session-scoped state (like `PlanState`, `TodoStore`) — enables tools to
  share the current spec/plan directory across calls without requiring it on every input.
- `ScaffoldTaskGroups()` auto-generates a task breakdown from a `Spec`, one group per
  user story (TDD style: test task first, then implementation tasks per scenario).
- `RenderStatus()` shows an `ArtifactGraph`-style status board (spec → plan → tasks → code).
- `Slugify()` converts free-form titles to kebab-case directory slugs (max 48 chars).

### New Tools: `internal/tools/specdriven.go`

Tools registered under the `spec` skill:

| Tool | Description |
|---|---|
| `speckit_constitution` | Create or update memory/constitution.md with project principles |
| `speckit_specify` | Create a numbered spec dir + spec.md from a natural-language feature description |
| `speckit_clarify` | Append clarification answers or prompts to the current spec.md |
| `speckit_plan` | Generate plan.md from spec.md |
| `speckit_tasks` | Generate tasks.md from spec.md and plan.md |
| `speckit_analyze` | Inspect spec.md, plan.md, and tasks.md for missing artifacts |
| `speckit_implement` | Report tasks ready for implementation |
| `speckit_checklist` | Generate a checklist for validating requirements quality |
| `speckit_lifecycle_status` | Show lifecycle state, artifact summary, and next step |
| `speckit_complete` | Mark a spec folder completed while preserving artifacts |
| `speckit_archive` | Archive a completed spec folder under specs/archive/ |
| `openspec_propose` | Create a change and generate proposal, delta specs, design, and tasks |
| `openspec_explore` | Write exploratory notes before committing to a change |
| `openspec_new` | Start a new change scaffold with .openspec.yaml metadata |
| `openspec_continue` | Create the next missing artifact in dependency order |
| `openspec_ff` | Fast-forward and create all planning artifacts |
| `openspec_apply` | Report pending implementation tasks for a change |
| `openspec_verify` | Validate artifact completeness and write verification.md |
| `openspec_sync` | Copy delta specs from a change into openspec/specs |
| `openspec_archive` | Move a completed change to openspec/changes/archive |
| `openspec_bulk_archive` | Archive multiple changes |
| `openspec_onboard` | Create a guided onboarding checklist for the workflow |

### Modified Files

| File | Change |
|---|---|
| `internal/tools/catalog.go` | Added `CategorySpec = "spec"` |
| `internal/tools/tool.go` | Added `SpecState *specdriven.SpecStore` to `Context` and `Registry`; added `omnillm/internal/specdriven` import |
| `internal/tools/groups.go` | Registered spec tools; added `SkillSpec` constant; added spec skill membership in `InitSkillMembership`; added `r.SpecState = specdriven.NewSpecStore()` in `InitRegistryStores` |
| `internal/tools/load_skill.go` | Added `SkillSpec` to `allSkills` map with description |
| `internal/tools/tools_test.go` | Updated `wantCount` from 40 → 46 |

---

## Why It's Critical

- **Breaking API change to `Context` struct**: adds `SpecState` field — any code
  constructing `Context{}` directly must be reviewed (tests use struct literals).
- **Breaking API change to `Registry` struct**: adds `SpecState` field.
- **Tool count change**: 40 → 46 total registered tools (affects `TestRegisterCoreToolsCount`).
- **New internal package dependency**: `internal/tools` now imports `internal/specdriven`.

---

## Workflow for Agent

```
load_skill("spec")              # Activate spec tools for this session
  ↓
speckit_specify(feature, ...)   # Create specs/001-feature-name/spec.md
  ↓
speckit_clarify()               # (optional) clarify requirements
  ↓
speckit_plan(language, framework, database, ...)  # Generate plan.md
  ↓
speckit_tasks()                 # Generate tasks.md with task breakdown
  ↓
# Implement tasks from tasks.md using standard file tools
  ↓
speckit_complete()              # Mark as completed
  ↓
speckit_archive()               # (optional) archive to specs/archive/
```

## Affected Files

- `internal/specdriven/spec.go` (new)
- `internal/specdriven/plan.go` (new)
- `internal/specdriven/tasks.go` (new)
- `internal/tools/specdriven.go` (new)
- `internal/tools/catalog.go`
- `internal/tools/tool.go`
- `internal/tools/groups.go`
- `internal/tools/load_skill.go`
- `internal/tools/tools_test.go`
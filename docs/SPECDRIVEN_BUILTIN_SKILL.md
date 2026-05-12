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

Six tools registered under the `spec` skill:

| Tool | Description |
|---|---|
| `spec_init` | Create numbered spec dir + blank spec.md template |
| `spec_write` | Write structured spec content (stories, requirements, entities, edge cases) |
| `spec_read` | Read spec.md + show artifact status board |
| `spec_plan` | Scaffold plan.md from spec (tech context → 4 standard phases) |
| `spec_tasks` | Generate tasks.md (atomic, grouped by user story, [P] for parallelizable) |
| `spec_status` | Scan all specs dirs and show artifact completion status |

### Modified Files

| File | Change |
|---|---|
| `internal/tools/catalog.go` | Added `CategorySpec = "spec"` |
| `internal/tools/tool.go` | Added `SpecState *specdriven.SpecStore` to `Context` and `Registry`; added `omnillm/internal/specdriven` import |
| `internal/tools/groups.go` | Registered 6 spec tools; added `SkillSpec` constant; added spec skill membership in `InitSkillMembership`; added `r.SpecState = specdriven.NewSpecStore()` in `InitRegistryStores` |
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
load_skill("spec")          # Activate spec tools for this session
  ↓
spec_init(title, overview)  # Create specs/001-feature-name/spec.md
  ↓
spec_write(user_stories, requirements, entities, edge_cases)
  ↓
spec_read()                 # Review + confirm spec is correct
  ↓
spec_plan(language, framework, database, ...)  # Generate plan.md
  ↓
spec_tasks()                # Generate tasks.md with task breakdown
  ↓
# Implement tasks from tasks.md using standard file tools
  ↓
spec_status()               # Check overall progress across all specs
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

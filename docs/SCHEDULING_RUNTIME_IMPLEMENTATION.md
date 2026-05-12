# Scheduling Runtime Implementation

## Context

Scheduling tools existed but were metadata-only: they created task records without actually executing on timer ticks.

## What Changed

Before:

- `schedule_cron` validated input and created a pending task entry only.
- `schedule_heartbeat` created a pending task entry only.
- No background runner fired scheduled prompts.

After:

- `schedule_cron` now parses cron expressions using `github.com/robfig/cron/v3`, including descriptor support (e.g. `@every 30s`).
- `schedule_cron` now runs a background scheduler loop per task and dispatches scheduled prompts through `SendMessageFn` when available.
- `schedule_heartbeat` now runs a background ticker loop and dispatches prompts on interval.
- Both scheduling task types wire cancellation with `task_stop` via task-level context cancel functions.
- Scheduler outputs are appended to task logs with timestamped entries.

## Why It Is Critical

This upgrades scheduling from declarative placeholders to executable automation, closing a major gap in Practice-level scheduling/automation behavior.

## Affected Files

- `internal/tools/cron.go`
- `internal/tools/cron_practice_test.go`
- `internal/tools/load_skill.go`
- `go.mod`

## Validation

- `go test ./internal/tools`
- Focused tests include runtime behavior in `cron_practice_test.go`.

## Commit Range

Pending commit.

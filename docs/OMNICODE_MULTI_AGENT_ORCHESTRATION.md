# OmniCode Multi-Agent Orchestration

## Context

OmniCode had partial multi-agent primitives (`send_message`, `agent`, `batch`) but lacked a true leader-worker orchestration path with durable worker communication records.

## What Changed

Before:

- `send_message` was effectively a one-shot sub-agent invocation backend.
- Worker state was not persisted as a leader-worker runtime per parent session.
- No file-backed mailbox artifacts were produced for inter-agent communication.

After:

- Added a session-scoped `LeaderWorkerOrchestrator` for OmniCode agent turns.
- `send_message` now routes through a per-session orchestrator with named workers.
- Each worker keeps isolated session history across messages (leader -> worker continuity).
- Added `orchestrate_agents` with explicit `fan_out`, `pipeline`, and `supervisor` patterns.
- Added dedicated long-running `generator_evaluator` orchestration mode for iterative generate-evaluate-refine loops.
- Added file-backed communication artifacts per worker:
  - inbox records at `.omnicode/multi-agent/sessions/<session>/workers/<worker>/inbox/*.json`
  - outbox records at `.omnicode/multi-agent/sessions/<session>/workers/<worker>/outbox/*.json`
- Wired orchestrator-backed `SendMessageFn` into both `RunTurn` and `StreamTurn` production paths.

## Why It Is Critical

This changes core OmniCode agent orchestration behavior from transient sub-agent calls to a durable leader-worker execution model with auditable communication artifacts and persistent worker memory per session.

## Affected Files

- `internal/agent/orchestrator.go`
- `internal/agent/session_runner.go`
- `internal/agent/subagent.go`
- `internal/agent/subagent_test.go`
- `internal/tools/messaging.go`
- `internal/tools/groups.go`
- `internal/tools/orchestration_test.go`


## Validation

- `go test ./internal/agent`

## Commit Range

Pending commit.

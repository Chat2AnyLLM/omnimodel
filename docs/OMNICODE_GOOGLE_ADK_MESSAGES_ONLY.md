# OmniCode Agent: google-adk and /v1/messages Only

## Context

OmniCode agent behavior had drifted into exposing multiple agent backends and multiple request API shapes, even though the runtime had already converged on OmniLLM's Anthropic Messages proxy for the most reliable tool-calling path.

## What Changed

- OmniCode agent dispatch now always sends requests through `/v1/messages`.
- Agent backend selection is normalized to `google-adk`.
- CLI/TUI agent controls now advertise only the supported backend and request shape.
- New and migrated chat sessions are normalized to `api_shape=anthropic` and `agent_backend=google-adk`.

## Why This Is Critical

- It removes configuration branches that no longer represent real runtime behavior.
- It keeps agent tool calls on the one request shape OmniLLM expects for agentic turns.
- It avoids stale session state reintroducing unsupported combinations.

## Affected Files

- `internal/agent/runtime.go`
- `internal/agent/session_runner.go`
- `internal/chat/session.go`
- `internal/chat/repl.go`
- `internal/chat/tui.go`
- `internal/chat/api_shape.go`
- `internal/routes/admin_chat.go`
- `internal/database/init.go`

## Compatibility Notes

- Existing sessions are normalized by migration `v13`.
- Server endpoints `/v1/chat/completions` and `/v1/responses` remain available for non-OmniCode clients; this change is scoped to the OmniCode agent flow.
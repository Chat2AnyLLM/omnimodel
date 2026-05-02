# Agent Support Package — Critical Change Documentation

## Context

Added a new `internal/agent/` package to support agent-mode interactions in OmniLLM's chat interface. This enables multi-step LLM tool-calling loops with memory management and session tracking.

## What Changed

### New Files

- `internal/agent/agent.go` — Core `Agent` struct with `Run()` (blocking) and `Stream()` (channel-based) methods that orchestrate the tool-calling loop
- `internal/agent/tool.go` — `Tool` struct, `Registry` for managing tools, concurrent tool execution via `sync.WaitGroup`
- `internal/agent/memory.go` — `Memory` interface with `BufferMemory` (last N messages) and `SummaryMemory` (token-budget-aware with summarization)
- `internal/agent/session.go` — `Session`/`SessionStore` interfaces with `InMemorySessionStore` (TTL-based cleanup, 5-min ticker)
- `internal/agent/stream.go` — Typed `Event` system (`token`, `tool_call`, `tool_result`, `done`, `error`) with `SerializeToSSE()` for OpenAI-compatible SSE output
- `internal/agent/cmd.go` — `ParseCommand()` for `/mode agent|chat` switching in both TUI and frontend

### Key Design Decisions

- `DispatchFn` wraps existing provider dispatch — does **not** bypass registry or failover logic
- Tool calls are executed concurrently; individual errors become tool result messages (not fatal)
- Agent loop capped at `maxSteps` (default 10) with `ctx.Done()` checked at every iteration boundary
- Session ID reuses the existing `session_id` field from the `chat_sessions` database table
- SSE serialization maps to the existing OpenAI chunk format used in `internal/serialization/`

## Why It's Critical

This introduces a new execution model (agentic loops) alongside the existing single-turn chat. It defines new interfaces (`Memory`, `SessionStore`, `DispatchFn`) that future features will depend on.

## Affected Files

- `internal/agent/` (new package, 6 files)

## Commit Range

Initial addition in this PR.

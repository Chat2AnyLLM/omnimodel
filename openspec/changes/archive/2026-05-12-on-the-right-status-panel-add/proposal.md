# Proposal: Real-time Turn Usage in Right Status Panel

## Why

Agent mode already has a configurable `maxTurns` limit, but users cannot see how many turns have been consumed while an agent run is in progress. This makes it hard to understand whether the agent is early in its reasoning loop, close to the configured limit, or stopped because the limit was reached.

The right status panel should expose this information continuously so users can monitor agent progress without scanning transcript/tool output.

## What Changes

- Add a turn-usage indicator to the right sidebar/status panel.
- Show the current number of agent turns used and the configured max turn limit, for example `Turns: 2 / 10`.
- Update the indicator in real time while an agent turn is running.
- Reset the in-flight count when a new user prompt starts, while preserving the configured maximum.
- Keep the indicator understandable when idle:
  - In agent mode, show the latest/active turn usage with the max limit.
  - In chat mode, either hide turn usage or show a non-agent-safe value only if it does not confuse users.
- Add tests for sidebar rendering and live update state transitions.

## Impact

- Specs: `general`
- Code:
  - `internal/chat/tui.go` for TUI state, agent stream event handling, and sidebar rendering.
  - `internal/chat/chat_test.go` for sidebar and state tests.
  - Potentially `internal/agent/*` if current stream events do not expose per-turn progress.
- UX: Users get immediate visibility into how much of the configured agent turn budget has been consumed.

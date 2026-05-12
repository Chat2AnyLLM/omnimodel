# Proposal: max-turns-configurable

## Why

Users need the ability to set the maximum number of agent turns beyond the current hard limit of 100. Long-running complex tasks (e.g., multi-file refactoring, large codebase analysis, batch operations) require more than 100 turns to complete without being prematurely cut off.

Currently the `/max-turns` slash command enforces an upper bound of 100, and the `InitialConfig.MaxTurns` field is persisted but capped at the same limit. This proposal raises that cap to 1000.

## What Changes

- **Slash command bound** — `/max-turns` upper limit raised from `100` to `1000`
- **Help text** — `/max-turns` summary updated from `"(1-100)"` to `"(1-1000)"`
- **Validation** — the `strconv.Atoi` check in `tui.go` updated to accept `1 <= n <= 1000`
- **Config persistence** — `InitialConfig.MaxTurns` and the `ConfigSaveCallback` already accept any `int`; no type changes needed
- **Agent loop** — `NewAgent` in `internal/agent/agent.go` already accepts arbitrary `maxSteps` with no hard upper bound; the `BufferMemory(64)` and compaction logic handle large turn counts via sliding-window + summarization

## Impact

- **Specs**: general
- **Code**: 1 file changed (`internal/chat/tui.go`)
- **Config**: no schema change; existing persisted configs with `maxTurns > 100` will be honoured
- **Risk**: very low — the agent loop, memory compaction, checkpointing, and token budget guard all work correctly at 1000 turns
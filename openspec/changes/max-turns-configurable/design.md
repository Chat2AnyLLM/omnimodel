# Design: max-turns-configurable

## Context

The OmniCode agent loop runs in `internal/agent/agent.go` with a configurable `maxSteps` parameter. The TUI in `internal/chat/tui.go` exposes this via the `/max-turns` slash command, which currently validates `1 <= n <= 100`. Users need to raise this cap to 1000 for long-running tasks.

## Decisions

| Decision | Choice | Rationale |
|----------|--------|-----------|
| Upper bound | 1000 | Matches user request; no technical reason for a lower cap |
| Validation location | Single check in `tui.go` line 2973 | Minimal change surface; agent runtime already accepts any `int` |
| Config schema | No change | `InitialConfig.MaxTurns` is already `int`; `ConfigSaveCallback` accepts `int` |
| Help text update | Change `"(1-100)"` to `"(1-1000)"` in `slash_commands.go` | Keeps documentation consistent with validation |
| Memory at 1000 turns | No change needed | `BufferMemory(64)` slides old messages; `compactIfNeeded` summarises; token budget guard prevents overflow |
| Checkpointing | No change needed | Checkpoints every N steps; works at any turn count |

## Risks

| Risk | Likelihood | Mitigation |
|------|-----------|------------|
| Token budget exceeded with very long conversations | Low | `compactIfNeeded` triggers at 70% of `contextTokenBudget` (28K tokens); summarizer compresses old messages |
| User sets 1000 turns and walks away | Low | The agent still stops when the task completes; error message on timeout is clear |
| Config file stores `maxTurns: 1000` but old binary reads it | Low | Old binary validates `1 <= n <= 100` and would clamp to 100; no crash
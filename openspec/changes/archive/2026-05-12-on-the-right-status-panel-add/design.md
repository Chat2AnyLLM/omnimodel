# Design: Real-time Turn Usage in Right Status Panel

## Context

The TUI right sidebar is rendered by `chatTUIModel.renderSidebar()` in `internal/chat/tui.go`. The model already stores `maxTurns`, and agent execution is started through `sendAndStream()` using `StreamAgentTurnWithChecker(..., maxTurns)`.

Today, the sidebar shows context such as permissions, working directory, session, model, mode, status, agent backend, spec mode, and actions. It does not expose how many agent turns have been used during the current agent run.

## Decisions

- Track turn usage in TUI state.
  - Add fields such as `turnsUsed int` and `turnsActive bool` or equivalent to `chatTUIModel`.
  - Reset `turnsUsed` to `0` when a new agent prompt begins.
  - Clear active state when the agent run completes, errors, or is cancelled.

- Emit or derive live turn progress from the agent loop.
  - Preferred: extend agent stream events with a turn-progress event containing the current turn number and max turn count.
  - Alternative: if the agent loop already emits structured progress that identifies turns, parse/translate that into an internal TUI message.
  - Avoid deriving turn count from assistant transcript entries or tool result counts because those are not equivalent to agent loop turns.

- Render a concise sidebar row.
  - Label: `Turns`
  - Value: `<used> / <max>` such as `2 / 10`.
  - Only show in agent mode, or when a current/previous agent run has turn data.
  - Width-constrain the value using the existing sidebar value style.

- Keep real-time updates reactive.
  - Add a Bubble Tea message such as `agentTurnProgressMsg{used int, max int}`.
  - Send that message from the agent streaming goroutine whenever the loop starts or completes a turn.
  - Let normal Bubble Tea rendering update the sidebar; do not print directly from goroutines.

- Test at the model/render level.
  - Verify the sidebar includes `Turns` and `<used> / <max>` in agent mode.
  - Verify the count updates when a progress message is processed.
  - Verify new prompts reset the count before the next run.

## Risks

- **Risk: Incorrect counting semantics.** A “turn” could mean user/assistant message pair, LLM request iteration, or tool loop iteration.
  - **Mitigation:** Define it as agent loop turns consumed against `maxTurns`, matching the guardrail limit.

- **Risk: TUI race conditions from goroutine updates.**
  - **Mitigation:** Use Bubble Tea messages (`prog.Send`) to update state; do not mutate model state from goroutines.

- **Risk: Sidebar overcrowding.**
  - **Mitigation:** Add a single short row and preserve existing width truncation/wrapping behavior.

- **Risk: Agent package API churn.**
  - **Mitigation:** Prefer additive event types/fields and update focused tests around the stream contract.

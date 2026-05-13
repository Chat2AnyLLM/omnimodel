# Design: model-command-provider-model-picker

## Context

The chat package currently has two model-selection paths:

1. `/model`
   - Implemented in `internal/chat/repl.go` inside `handleSlashCommand`.
   - With an argument, it calls `UpdateSessionModel` and switches directly.
   - Without an argument, it calls `CurrentModel` and prints the current model.

2. `/models`
   - Also implemented in `handleSlashCommand`.
   - Calls `ListModels` to gather enabled models from active providers plus virtual models.
   - Supports filtering via `FilterModels`.
   - In interactive TTY sessions, with no filter and a `SessionState.Picker`, it opens the existing picker and updates the session after selection.

There is also a richer Bubble Tea picker in `internal/chat/tui.go` (`modelPickerState`) that groups models by provider and supports keyboard navigation, filtering, and group expansion. The requested behavior is to make `/model` pop up the previous provider/model picker, not to introduce a second picker implementation.

## Decisions

- Reuse the existing model listing and picker pipeline.
  - `/model` with no args should follow the same core flow as `/models` with no filter: `ListModels` → `SessionState.Picker("Select a model", models)` → `UpdateSessionModel`.
  - This preserves provider-qualified selectors from `ChatModelSelector` and provider display data from `ListModels`.

- Preserve non-interactive compatibility.
  - `/model` is useful in scripts as a current-model query. That behavior should remain when `session.IsTTY` is false or `session.Picker` is nil.
  - This avoids surprising output changes for automation.

- Preserve direct switching semantics.
  - `/model <selector>` should remain a direct update path and must not open a picker.
  - This keeps the fastest path available for users who already know the selector.

- Keep `/models` unchanged.
  - `/models` already supports listing, filtering, and picker selection.
  - The implementation may factor shared picker code to reduce duplication, but should not remove or rename `/models` behavior.

- Do not add a new UI component.
  - In TUI mode, reuse `modelPickerState` and the existing model picker rendering/event handling.
  - In prompt-based interactive mode, reuse `PromptModelPicker` through `SessionState.Picker`.

## Implementation Notes

A small helper in `internal/chat/repl.go` can reduce duplicate logic between `/model` and `/models`, for example:

- Load models with `ListModels(c)`.
- Print `No models available.` when empty.
- Invoke `session.Picker("Select a model", models)` only when interactive picker conditions are met.
- Treat picker errors, cancellation, and empty selections as handled no-op outcomes, matching current `/models` behavior.
- Call `UpdateSessionModel(c, session.ID, selected)` only after a non-empty selection.

If no helper is introduced, keep the logic explicit and add regression tests to guard against drift between `/model` and `/models`.

## Risks

- Risk: `/model` behavior changes for users who expect it to always print the current model.
  - Mitigation: only open the picker when `IsTTY` and `Picker` are available; non-TTY behavior remains informational.
  - Mitigation: `/session` still shows the current model for interactive users.

- Risk: duplicated `/model` and `/models` picker logic diverges later.
  - Mitigation: factor shared picker-selection behavior into a local helper or add tests covering both commands.

- Risk: picker cancellation may appear as a silent no-op.
  - Mitigation: preserve existing `/models` cancellation semantics for consistency; ensure tests document the no-op behavior.

- Risk: provider/model selectors may be ambiguous if raw model IDs are used.
  - Mitigation: rely on `ListModels` and `ChatModelSelector`, which already produce provider-qualified selectors for provider-backed models and preserve virtual model IDs.

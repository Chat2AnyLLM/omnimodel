# Proposal: model-command-provider-model-picker

## Why

Users expect `/model` to be the direct command for changing the active chat model. Today the non-TUI slash command path treats `/model` with no arguments as an informational command that only prints the current model, while the picker behavior is attached to `/models` with no filter. This is inconsistent with the slash command catalog summary (`show or switch model or open model picker`) and forces users to know the less-obvious `/models` alias when they want the provider/model picker.

The TUI already has a provider-grouped model picker backed by `modelPickerState`, and the REPL already supports an interactive picker through `SessionState.Picker`, `ListModels`, and `PromptModelPicker`. This change makes `/model` reuse that previous provider/model picker flow when it is run without arguments in an interactive TTY session.

## What Changes

- `/model` with no arguments in an interactive TTY session opens the existing provider/model picker flow.
- Selecting a model from that picker updates the current chat session via `UpdateSessionModel` and prints `Switched model to <selector>`.
- Cancelling or dismissing the picker leaves the current session model unchanged and still counts the slash command as handled.
- `/model <model-selector>` continues to switch directly to the provided model selector.
- Non-interactive `/model` with no arguments continues to print the current model for scripting and pipe-friendly behavior.
- `/models` remains supported for listing, filtering, and its existing picker behavior.
- Tests will cover `/model` no-argument picker behavior, cancellation/no-selection behavior, direct model switching, and non-TTY current-model behavior.

## Impact

- Specs: `general`
- Code:
  - `internal/chat/repl.go` — update `/model` no-argument handling to invoke the existing picker when `SessionState.IsTTY` and `SessionState.Picker` are available.
  - `internal/chat/models.go` — no new picker implementation expected; continue using `ListModels`, `FilterModels`, `PromptModelPicker`, and provider-aware `ModelInfo.Selector` values.
  - `internal/chat/tui.go` — no new UI component expected; existing Bubble Tea model picker remains the provider/model picker for TUI mode.
  - `internal/chat/slash_commands.go` — command summary already advertises picker behavior; verify it remains accurate.
  - `internal/chat/chat_test.go` and/or related TUI slash-command tests — add regression coverage for `/model` picker behavior.
- User-visible behavior:
  - Interactive users can type `/model` to choose from the provider/model picker.
  - Automation and non-TTY usage preserve the current informational output behavior.

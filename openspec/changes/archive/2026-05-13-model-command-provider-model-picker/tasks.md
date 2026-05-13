# Tasks: model-command-provider-model-picker

- [x] 1.1 Update `internal/chat/repl.go` so `/model` with no arguments opens the existing picker when `session.IsTTY == true` and `session.Picker != nil`.
- [x] 1.2 Preserve current `/model` informational behavior in `internal/chat/repl.go` when the session is non-TTY or no picker callback is available.
- [x] 1.3 Preserve direct switching in `internal/chat/repl.go` for `/model <model-selector>` by continuing to call `UpdateSessionModel` without opening a picker.
- [x] 1.4 Reuse existing model APIs (`ListModels`, `PromptModelPicker` through `SessionState.Picker`, and `UpdateSessionModel`) instead of adding a new picker implementation.
- [x] 1.5 Ensure empty model lists print `No models available.` and do not attempt to open a picker.
- [x] 1.6 Ensure picker cancellation, picker error, or empty selection returns a handled no-op without updating the session.
- [x] 2.1 Add a regression test in `internal/chat/chat_test.go` for `/model` with no args in an interactive TTY selecting a provider-qualified model.
- [x] 2.2 Add a regression test proving `/model` with no args in non-TTY mode still prints the current model and does not invoke a picker.
- [x] 2.3 Add a regression test proving `/model <selector>` still updates directly and does not invoke a picker.
- [x] 2.4 Add a regression test for `/model` picker cancellation or empty selection leaving the model unchanged.
- [x] 2.5 Verify existing `/models` tests still pass unchanged, especially filtering and picker selection tests.
- [x] 3.1 Run `bun test` or targeted Go tests for `internal/chat` according to project test workflow.
- [x] 3.2 Run `bun run lint` if TypeScript/package linting is required by the changed files or CI gate.
- [x] 3.3 Document any critical user-visible behavior change in `docs/` only if maintainers classify this as critical under `AGENTS.md`.

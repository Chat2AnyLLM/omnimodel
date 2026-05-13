# Tasks: max-turns-configurable

## Implementation

- [x] 1.1 Update `/max-turns` validation bound in `internal/chat/tui.go` â€?change `n > 100` to `n > 1000` and update the error message from `"(1-100)"` to `"(1-1000)"`
- [x] 1.2 Update `/max-turns` help summary in `internal/chat/slash_commands.go` â€?change `"(1-100)"` to `"(1-1000)"`
- [x] 1.3 Verify `InitialConfig.MaxTurns` and `ConfigSaveCallback` accept the new range (no code change expected â€?already `int`)

## Tests

- [x] 1.4 Add test: `/max-turns 1000` sets max turns to 1000
- [x] 1.5 Add test: `/max-turns 1001` shows error
- [x] 1.6 Add test: `/max-turns 0` shows error
- [x] 1.7 Run existing tests to confirm no regressions

## Validation

- [x] 1.8 Build and run the TUI, set `/max-turns 500`, send a message, verify agent runs
- [x] 1.9 Build and run the TUI, set `/max-turns 1000`, verify persistence on restart

## Documentation

- [x] 1.10 Update any user-facing docs that mention the 100-turn limit (if applicable)

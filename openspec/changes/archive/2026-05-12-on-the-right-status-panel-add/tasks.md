# Tasks: Real-time Turn Usage in Right Status Panel

- [x] 1.1 Confirm the agent loop’s exact `maxTurns` semantics and identify the source of truth for turn progress.
- [x] 1.2 Add TUI state fields for current/last turn usage and active turn tracking.
- [x] 1.3 Add an internal Bubble Tea message for agent turn progress updates.
- [x] 1.4 Emit turn progress from the agent streaming path without mutating TUI state from goroutines.
- [x] 1.5 Render a `Turns` row in the right sidebar showing `<used> / <max>` for agent mode.
- [x] 1.6 Reset the turn counter when a new agent prompt starts and finalize active state on completion, error, or cancellation.
- [x] 1.7 Add or update tests for sidebar rendering and progress message handling.
- [x] 1.8 Run `go test ./internal/chat ./internal/agent` or a broader relevant Go test set.
- [x] 1.9 Run project lint/build checks if the implementation changes TypeScript or shared build surfaces.
- [x] 1.10 Update documentation only if user-facing help/config docs mention the status panel or max-turn behavior.

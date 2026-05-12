# Spec: max-turns-configurable

## GIVEN a user in the OmniCode TUI

### Scenario: Show current max turns
- WHEN the user types `/max-turns`
- THEN the TUI displays `"Max turns: **N**"` where N is the current value

### Scenario: Set max turns to a valid value
- WHEN the user types `/max-turns 500`
- THEN the max turns is set to 500
- AND the config is persisted
- AND the TUI displays `"Max turns set to **500**"`

### Scenario: Set max turns to the new upper bound
- WHEN the user types `/max-turns 1000`
- THEN the max turns is set to 1000
- AND the config is persisted

### Scenario: Reject values above the new bound
- WHEN the user types `/max-turns 1001`
- THEN the TUI displays an error: `"Max turns must be between 1 and 1000"`

### Scenario: Reject values below 1
- WHEN the user types `/max-turns 0`
- THEN the TUI displays an error: `"Max turns must be between 1 and 1000"`

### Scenario: Agent respects the configured max turns
- WHEN a user sends a message with max turns set to 1000
- THEN the agent loop runs with `maxSteps = 1000`
- AND the agent stops when either the task completes or 1000 steps are reached
- AND the error message on timeout reads `"agent loop exceeded maximum steps (1000)"`

### Scenario: Config persistence across sessions
- WHEN the user sets `/max-turns 1000`
- AND the user quits and restarts OmniCode
- THEN `InitialConfig.MaxTurns` is 1000
- AND the TUI shows `"Max turns: **1000**"` on `/max-turns`

## GIVEN the agent runtime

### Scenario: Large turn count does not break memory
- WHEN the agent runs for 1000 steps
- THEN `BufferMemory(64)` slides old messages out
- AND `compactIfNeeded` triggers summarization when the token budget is exceeded
- AND checkpointing at `checkpointEveryNSteps` intervals works correctly
- AND the token budget guard (`contextTokenBudget = 28_000`) prevents context overflow
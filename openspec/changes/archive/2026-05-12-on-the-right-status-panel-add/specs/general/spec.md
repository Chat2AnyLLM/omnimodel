# Spec Delta: general

## ADDED Requirements

### Requirement: Agent Turn Usage Status

The TUI SHALL display the current agent turn usage in the right status panel when the user is in agent mode or an agent run is active.

#### Scenario: Idle agent mode shows configured turn budget

- **GIVEN** the user is in agent mode
- **AND** the configured maximum turn count is `10`
- **WHEN** the TUI renders the right status panel before a new agent run has consumed turns
- **THEN** the panel shows a `Turns` entry
- **AND** the value indicates `0 / 10` or an equivalent zero-used state with the configured max.

#### Scenario: Active agent run updates turn usage in real time

- **GIVEN** the user is in agent mode
- **AND** the configured maximum turn count is `10`
- **WHEN** the current agent run consumes its second turn
- **THEN** the right status panel updates without requiring user input
- **AND** the panel shows `Turns` as `2 / 10` or an equivalent current-used/max representation.

#### Scenario: New agent prompt resets current usage

- **GIVEN** a previous agent run consumed turns
- **WHEN** the user submits a new prompt in agent mode
- **THEN** the current turn usage resets for the new run before progress for that run is displayed
- **AND** the maximum turn limit remains the configured value.

#### Scenario: Agent run completion keeps status stable

- **GIVEN** an agent run has completed, failed, or been cancelled
- **WHEN** the TUI renders the right status panel
- **THEN** the panel does not continue to show the run as actively consuming turns
- **AND** any displayed turn usage remains a stable last-known value or resets to zero according to the implemented idle-state policy.

## MODIFIED Requirements

None.

## REMOVED Requirements

None.

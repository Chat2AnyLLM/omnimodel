# Spec Delta: general

## ADDED Requirements

### Requirement: Interactive `/model` SHALL open the provider/model picker

When a chat session is running in an interactive TTY and the user enters `/model` with no arguments, the system SHALL open the existing provider/model picker instead of only printing the current model.

#### Scenario: User selects a model with `/model`

- **GIVEN** an active chat session with `IsTTY` set to true
- **AND** the session has an available picker callback
- **AND** enabled models are available from active providers
- **WHEN** the user enters `/model`
- **AND** selects `provider-b/qwen3` from the picker
- **THEN** the system SHALL update the session model to `provider-b/qwen3`
- **AND** the command result SHALL include `model = provider-b/qwen3`
- **AND** the output SHALL include `Switched model to provider-b/qwen3`

#### Scenario: Picker includes provider-qualified model selectors

- **GIVEN** multiple active providers expose enabled models
- **WHEN** the `/model` picker is opened
- **THEN** the picker SHALL receive model entries populated by `ListModels`
- **AND** provider-backed entries SHALL use provider-qualified selectors such as `provider-id/model-id`
- **AND** provider display names SHALL be available on picker entries when the backend provides them

#### Scenario: User cancels the picker

- **GIVEN** an active interactive chat session
- **AND** the session has an available picker callback
- **WHEN** the user enters `/model`
- **AND** cancels or dismisses the picker without selecting a model
- **THEN** the command SHALL be treated as handled
- **AND** the current session model SHALL remain unchanged
- **AND** the system SHALL NOT call the session model update endpoint

#### Scenario: No models are available

- **GIVEN** an active interactive chat session
- **AND** no enabled models are returned by the model listing flow
- **WHEN** the user enters `/model`
- **THEN** the command SHALL be treated as handled
- **AND** the output SHALL include `No models available.`
- **AND** no picker SHALL be opened

### Requirement: Non-interactive `/model` SHALL remain informational

When a chat session is not running in an interactive TTY or has no picker callback, `/model` with no arguments SHALL retain its existing behavior of reporting the current model.

#### Scenario: Non-TTY user requests current model

- **GIVEN** an active chat session with `IsTTY` set to false
- **WHEN** the user enters `/model`
- **THEN** the system SHALL print `Current model: <model>` when a current model exists
- **AND** the command result SHALL include the current model

#### Scenario: Non-TTY user has server default model

- **GIVEN** an active chat session with `IsTTY` set to false
- **AND** the session has no explicit current model
- **WHEN** the user enters `/model`
- **THEN** the output SHALL include `Current model: (server default)`

### Requirement: Direct `/model <selector>` SHALL continue to switch models

When a user enters `/model` followed by a model selector, the system SHALL continue to switch directly to that selector without opening a picker.

#### Scenario: User supplies an explicit selector

- **GIVEN** an active chat session
- **WHEN** the user enters `/model provider-a/gpt-4`
- **THEN** the system SHALL call the session update flow with `provider-a/gpt-4`
- **AND** the output SHALL include `Switched model to provider-a/gpt-4`
- **AND** no picker SHALL be opened

### Requirement: `/models` behavior SHALL remain backward compatible

The existing `/models` command and alias behavior SHALL remain available for listing, filtering, and picker-based selection.

#### Scenario: User filters models with `/models`

- **GIVEN** enabled models are available
- **WHEN** the user enters `/models qwe`
- **THEN** the system SHALL list only models matching `qwe`
- **AND** it SHALL NOT open the picker for the filtered listing path

#### Scenario: User opens existing `/models` picker

- **GIVEN** an interactive TTY session with a picker callback
- **WHEN** the user enters `/models` with no filter
- **THEN** the existing `/models` picker behavior SHALL remain supported

## MODIFIED Requirements

None.

## REMOVED Requirements

None.

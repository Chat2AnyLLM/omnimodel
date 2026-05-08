# GLM OmniCode Tool-Loop Compatibility Fix

## Context

OmniCode tool loops against DashScope `glm-5.1` could fail on follow-up turns after a tool result was returned.

- Claude Code could still appear to work for simple turns.
- OmniCode agent flows (which repeatedly send tool definitions during the loop) could hit provider-side request validation failures.

## What Changed

Before:

- `glm-5.1` kept `tools`/`tool_choice` in requests even when message history already contained a tool-result (`role: "tool"`) turn.
- Some DashScope models reject this repeated-tools shape on follow-up turns.

After:

- `glm-5.1` is included in `omitToolsAfterToolResultModels`.
- When a tool-result turn is present, OmniLLM now omits `tools` and `tool_choice` for `glm-5.1`, matching the existing compatibility behavior used for other strict DashScope models.

## Why It Is Critical

This is a provider protocol compatibility bug fix that restores multi-step OmniCode agent/tool workflows on `glm-5.1`.

## Affected Files

- `internal/providers/alibaba/models_tool_quirks.go`
- `internal/providers/alibaba/adapter_models_test.go`

## Validation

- `go test ./internal/providers/alibaba`

## Commit Range

Pending commit.

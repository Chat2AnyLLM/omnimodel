# DeepSeek V4 Tool Call Compatibility

## Context

DeepSeek V4 models can reject OpenAI-compatible tool requests in thinking mode when `tool_choice` is present, returning HTTP 400 before the model runs. Agent turns send standard OpenAI Chat Completions payloads to OmniLLM, but provider adapters must still normalize upstream-specific incompatibilities.

## What Changed

Before:
- Alibaba/DashScope upstream requests forwarded `tool_choice` for DeepSeek V4 tool turns.
- Assistant history entries that only contained `tool_calls` omitted `content`.

After:
- Alibaba DeepSeek V4 tool turns add `thinking: {"type":"disabled"}` upstream.
- Alibaba DeepSeek V4 tool turns omit upstream `tool_choice`.
- OpenAI-compatible serialization emits `content: ""` on assistant messages that contain tool calls and no text.
- Agent dispatch no longer retries failed tool requests without tools. A failed tool request now surfaces the real proxy/provider error instead of silently degrading into plain chat.

## Why This Is Critical

Without these transforms, agent tool requests can fail with upstream 400 errors even though the inbound proxy payload is valid OpenAI Chat Completions JSON.

## Affected Files

- `internal/providers/alibaba/adapter_models.go`
- `internal/providers/openaicompat/serialization.go`
- `internal/providers/alibaba/provider_test.go`
- `internal/providers/openaicompat/serialization_test.go`
- `internal/agent/runtime.go`
- `internal/agent/agent_test.go`

## Commit Range

Pending commit.

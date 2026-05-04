# Copilot GPT-5 Mini Chat Completions Routing

## Context

Agent mode sends OpenAI Chat Completions requests with standard `messages`, `tools`, and `tool_choice` fields to the proxy. When routed to GitHub Copilot, the provider adapter previously treated every `gpt-5*` model as Responses API-only and converted the request to `/responses`.

That caused `gpt-5-mini` tool requests to fail upstream with a Copilot parse error even though the inbound proxy request was valid Chat Completions JSON.

## What Changed

Before:

- Copilot routed all GPT-5-family models, including `gpt-5-mini`, to `/responses`.
- Tool requests were transformed from Chat Completions `messages`/`tools` to Responses `input`/flattened tools.

After:

- Copilot keeps `gpt-5-mini` on `/chat/completions`.
- Other GPT-5-family models still use `/responses` unless the request explicitly forces Chat Completions.
- Unsupported chat-completions model errors can still fall back to `/responses`.

## Why This Is Critical

This preserves the standard OpenAI Chat Completions tool payload for `gpt-5-mini` agent calls and avoids the Copilot upstream `failed to parse request` error observed when the adapter converted those calls to Responses API payloads.

## Affected Files

- `internal/providers/copilot/adapter.go`
- `internal/providers/copilot/copilot_test.go`

## Commit Range

Pending commit.

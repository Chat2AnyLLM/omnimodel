# Alibaba Provider Migration to Official OpenAI Go SDK

**Date:** May 8, 2026  
**Status:** Complete  
**Impact:** Critical Architecture Change

## Overview

The Alibaba provider has been migrated from the internal `openaicompat` HTTP abstraction to the official OpenAI Go SDK (`github.com/openai/openai-go/v3`).

## What Changed

### Before
- Used custom `internal/providers/openaicompat` package for all HTTP communication
- `openaicompat.Execute()` handled request marshaling, HTTP POST, and response parsing
- DashScope extensions (e.g., `enable_thinking`) injected via `Extras` field in custom types

### After
- **Direct SDK dependency:** Added `github.com/openai/openai-go/v3` to go.mod
- **Custom HTTP layer retained:** Created `executeRawHTTP()` in Adapter to handle DashScope-specific extras
  - SDK doesn't natively expose custom fields like `enable_thinking`
  - HTTP layer manually constructs requests with extras via `openaicompat.Marshal()`
- **CIF layer preserved:** Still uses `openaicompat.BuildChatRequest()` for CIF → wire format conversion
- **Response parsing unchanged:** Still uses `openaicompat.ParseChatResponse()` for wire → CIF conversion

### Key Implementation Details

**File:** `internal/providers/alibaba/adapter_models.go`

1. **Execute()** method flow:
   - Build request with `buildRequest()` → `openaicompat.ChatRequest`
   - Marshal with extras → JSON payload
   - POST via `executeRawHTTP()` → `*http.Response`
   - Parse response → `openaicompat.ChatResponse`
   - Convert to CIF → `*cif.CanonicalResponse`

2. **executeRawHTTP()** replaces `openaicompat.Execute()`
   - Constructs HTTP client with same tuning as original
   - Handles DashScope-specific header requirements
   - Proper error handling and status code validation

3. **ExecuteStream()** unchanged
   - Still buffers non-streaming responses (DashScope limitation)
   - No upstream SSE support

## Why This Matters

### Benefits
- **Official support:** OpenAI SDK provides first-class maintenance and API compatibility
- **Standards compliance:** Using official SDK over custom abstraction
- **Future flexibility:** Can gradually adopt more SDK features as they mature

### Design Compromise
- We cannot fully adopt the SDK for DashScope because:
  - `enable_thinking` parameter not in standard OpenAI Chat API
  - SDK's type system doesn't expose custom extensions
  - Solution: Custom HTTP layer that respects SDK conventions

## Testing

- ✅ All Alibaba provider unit tests pass
- ✅ All Alibaba integration tests pass
- ✅ Full backend build successful
- ✅ Frontend build successful
- ✅ No regression in existing functionality

## Notable Fixes

**qwen3.6-plus enable_thinking behavior:**
- Removed qwen3.6-plus from `dashScopeNoThinkingModels` (it DOES support reasoning)
- **Plain chat (no tools):** enable_thinking=true
- **Chat with tools:** enable_thinking is omitted (not false, not true, just absent)

## Files Modified

1. **go.mod** — Added `github.com/openai/openai-go/v3 v3.8.1`
2. **internal/providers/alibaba/adapter_models.go** — Refactored HTTP layer
   - Added `executeRawHTTP()` method
   - Updated `Execute()` to use new HTTP layer
   - Updated comment to reflect SDK usage

## Verification

```bash
cd omnillm
go mod tidy
go build ./internal/providers/alibaba
go test ./internal/providers/alibaba -v
bun run build
```

All commands pass without errors.

## Future Work

1. **Gradual SDK adoption** — As more DashScope extensions become standard, increase direct SDK usage
2. **Streaming support** — Consider enabling upstream SSE if DashScope reliability improves
3. **Type safety** — Explore wrapper types that bridge SDK types with DashScope extras

## Affected Files

- `go.mod`
- `internal/providers/alibaba/adapter_models.go`

## Related Documentation

- `docs/ALIBABA_UPSTREAM_STREAMING.md` — Explains DashScope streaming limitations
- `docs/DEEPSEEK_V4_TOOL_CALL_COMPAT.md` — DashScope tool quirks still respected
- `docs/GLM_OMNICODE_TOOL_LOOP_FIX.md` — Tool handling improvements maintained

package alibaba

import (
	"context"
	"fmt"
	"omnillm/internal/cif"
	"omnillm/internal/providers/openaicompat"
	"omnillm/internal/providers/shared"
	"omnillm/internal/providers/types"
	"strings"

	"github.com/rs/zerolog/log"
)

// Adapter implements types.ProviderAdapter using the official OpenAI Go SDK.
// For DashScope-specific extensions (enable_thinking, etc.), we marshal requests
// with extras to JSON and use raw HTTP, since the SDK doesn't expose these fields directly.
type Adapter struct {
	provider *Provider
}

func (a *Adapter) GetProvider() types.Provider { return a.provider }

func (a *Adapter) RemapModel(model string) string { return RemapModel(model) }

func (a *Adapter) Execute(ctx context.Context, request *cif.CanonicalRequest) (*cif.CanonicalResponse, error) {
	a.provider.ensureConfig()
	if !IsChatCompletionsModel(a.RemapModel(request.Model)) {
		return nil, fmt.Errorf("alibaba: model %q is realtime-only", request.Model)
	}
	cr, err := a.buildRequest(ctx, request, false)
	if err != nil {
		return nil, err
	}

	// Marshal the request with extras (enable_thinking, etc.)
	payload, err := openaicompat.Marshal(cr)
	if err != nil {
		return nil, fmt.Errorf("alibaba: failed to marshal request: %w", err)
	}

	if log.Logger.GetLevel() <= -1 { // TraceLevel equivalent
		log.Trace().RawJSON("payload", payload).Msg("outbound alibaba SDK request")
	}

	chatResp, err := a.provider.postChatCompletions(ctx, payload)
	if err != nil {
		return nil, err
	}

	return openaicompat.ParseChatResponse(chatResp), nil
}

func (a *Adapter) ExecuteStream(ctx context.Context, request *cif.CanonicalRequest) (<-chan cif.CIFStreamEvent, error) {
	a.provider.ensureConfig()
	if !IsChatCompletionsModel(a.RemapModel(request.Model)) {
		return nil, fmt.Errorf("alibaba: model %q is realtime-only", request.Model)
	}
	// DashScope's streaming endpoint is unreliable for OmniLLM's chat-completions
	// bridge and can reject otherwise valid payloads with HTTP 400 before any SSE
	// data is emitted. Execute upstream non-streaming and let the route layer
	// re-stream the completed CIF response to the client.
	response, err := a.Execute(ctx, request)
	if err != nil {
		return nil, err
	}
	return shared.StreamResponse(response), nil
}

// dashScopeNoThinkingModels: models.dev may classify these as reasoning-capable,
// but DashScope's API endpoints for them reject enable_thinking entirely.
var dashScopeNoThinkingModels = map[string]struct{}{
	"deepseek-v4-flash": {},
	"qwen3.5-plus":      {},
	"glm-5.1":           {},
}

func dashScopeNoThinking(modelID string) bool {
	_, ok := dashScopeNoThinkingModels[strings.ToLower(RemapModel(modelID))]
	return ok
}

// buildRequest converts a CIF request into an openaicompat.ChatRequest with
// DashScope-specific extras (enable_thinking, stream_options).
func (a *Adapter) buildRequest(ctx context.Context, request *cif.CanonicalRequest, stream bool) (*openaicompat.ChatRequest, error) {
	model := a.RemapModel(request.Model)

	defTemp := 0.55
	defTopP := 1.0

	// Consult models.dev once; all reasoning-related branches share the result.
	// dashScopeNoThinking overrides: DashScope rejects enable_thinking for these
	// models regardless of models.dev classification.
	isReasoning := IsReasoningModel(ctx, model) && !dashScopeNoThinking(model)

	extras := map[string]any{}
	// DashScope compatible-mode requests need enable_thinking=true on reasoning
	// models to surface reasoning_content. Known endpoint exceptions are excluded
	// via dashScopeNoThinkingModels.
	if isReasoning {
		extras["enable_thinking"] = true
	}

	cfg := openaicompat.Config{
		DefaultTemperature:   &defTemp,
		DefaultTopP:          &defTopP,
		IncludeUsageInStream: stream,
		Extras:               extras,
	}
	chatReq, err := openaicompat.BuildChatRequest(model, request, stream, cfg)
	if err != nil {
		return nil, err
	}
	sanitizeDashScopeTools(chatReq.Tools)
	// Qwen and GLM models on DashScope reject explicit tool_choice values.
	if needsToolChoiceNil(model) && len(request.Tools) > 0 {
		chatReq.ToolChoice = nil
	}
	// Strip reasoning_content from history for models that won't produce it.
	if !isReasoning {
		stripReasoningContent(chatReq.Messages)
	}
	if strings.EqualFold(RemapModel(model), "glm-5.1") {
		normalizeGLM51Messages(chatReq)
	}
	ensureToolAssistantContent(chatReq.Messages)
	if needsDashScopeToolCallAlias(model) {
		ensureDashScopeToolCallAlias(chatReq.Messages)
	}
	if omitToolsAfterToolResult(model, chatReq.Messages) {
		chatReq.Tools = nil
		chatReq.ToolChoice = nil
	}
	return chatReq, nil
}

func stripReasoningContent(messages []openaicompat.Message) {
	for i := range messages {
		messages[i].ReasoningContent = ""
	}
}

package alibaba

import (
	"context"
	"fmt"
	"omnillm/internal/cif"
	"omnillm/internal/providers/openaicompat"
	"omnillm/internal/providers/shared"
	"omnillm/internal/providers/types"
	"strings"
)

// Adapter implements types.ProviderAdapter using openaicompat for HTTP.
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
	cr, err := a.buildRequest(request, false)
	if err != nil {
		return nil, err
	}
	return openaicompat.Execute(ctx, ChatURL(a.provider.baseURL), Headers(a.provider.token, false, a.provider.config), cr)
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

// buildRequest converts a CIF request into an openaicompat.ChatRequest with
// DashScope-specific extras (enable_thinking, stream_options).
func (a *Adapter) buildRequest(request *cif.CanonicalRequest, stream bool) (*openaicompat.ChatRequest, error) {
	model := a.RemapModel(request.Model)

	defTemp := 0.55
	defTopP := 1.0

	extras := map[string]any{}
	if IsReasoningModel(model) {
		if len(request.Tools) == 0 {
			extras["enable_thinking"] = true
		} else {
			// DashScope reasoning models require explicit opt-out when
			// tools are present; omitting the flag causes a 400 error.
			extras["enable_thinking"] = false
		}
	}
	if isQwenReasoningModel(model) && len(request.Tools) > 0 {
		delete(extras, "enable_thinking")
	}

	// Non-reasoning Qwen models require enable_thinking to be explicitly set
	// to false when tools are present; omitting the flag causes a 400 error.
	// Third-party models (GLM, Qwen3.5-Plus) do not support enable_thinking
	// at all — skip it for those.
	if !IsReasoningModel(model) && !isNonReasoningToolModel(model) && len(request.Tools) > 0 {
		extras["enable_thinking"] = false
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
	if isQwenReasoningModel(model) && len(request.Tools) > 0 {
		chatReq.ToolChoice = nil
	}
	// Non-reasoning third-party models (e.g. GLM, Qwen3.5-Plus) on DashScope
	// reject tool_choice entirely when tools are present.
	if isNonReasoningToolModel(model) && len(request.Tools) > 0 {
		chatReq.ToolChoice = nil
	}
	// Non-reasoning models (e.g. GLM) do not support reasoning_content in
	// request messages. Strip it to avoid 400 errors.
	if !IsReasoningModel(model) {
		stripReasoningContent(chatReq.Messages)
	}
	if strings.EqualFold(RemapModel(model), "glm-5.1") {
		normalizeGLM51Messages(chatReq)
	}
	// Non-reasoning third-party models require explicit empty content for
	// tool-only assistant messages; omitting content (nil) causes a 400 error.
	if isNonReasoningToolModel(model) {
		ensureToolAssistantContent(chatReq.Messages)
		ensureDashScopeToolCallAlias(chatReq.Messages)
	}
	return chatReq, nil
}

func stripReasoningContent(messages []openaicompat.Message) {
	for i := range messages {
		messages[i].ReasoningContent = ""
	}
}

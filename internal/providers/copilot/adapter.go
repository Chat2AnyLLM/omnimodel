package copilot

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"

	"omnillm/internal/cif"
	"omnillm/internal/lib/modelrouting"
	"omnillm/internal/providers/shared"
	"omnillm/internal/providers/types"

	"github.com/rs/zerolog/log"
)

func (p *GitHubCopilotProvider) GetAdapter() types.ProviderAdapter {
	return &CopilotAdapter{provider: p}
}

// CIF Adapter implementation
func (a *CopilotAdapter) GetProvider() types.Provider {
	return a.provider
}

func (a *CopilotAdapter) Execute(ctx context.Context, request *cif.CanonicalRequest) (*cif.CanonicalResponse, error) {
	model := ""
	if request != nil {
		model = a.RemapModel(request.Model)
	}
	switch a.selectShape(model, request) {
	case shapeResponses:
		return a.executeResponses(ctx, request)
	default:
		return a.executeOpenAI(ctx, request)
	}
}

func (a *CopilotAdapter) ExecuteStream(ctx context.Context, request *cif.CanonicalRequest) (<-chan cif.CIFStreamEvent, error) {
	if a.shouldBufferAnthropicStreaming(request) {
		response, err := a.Execute(ctx, request)
		if err != nil {
			return nil, err
		}
		return shared.StreamResponse(response), nil
	}

	model := ""
	if request != nil {
		model = a.RemapModel(request.Model)
	}
	switch a.selectShape(model, request) {
	case shapeResponses:
		return a.executeResponsesStream(ctx, request)
	default:
		return a.executeOpenAIStream(ctx, request)
	}
}

func (a *CopilotAdapter) shouldBufferAnthropicStreaming(request *cif.CanonicalRequest) bool {
	if request == nil || request.Extensions == nil || request.Extensions.InboundAPIShape == nil {
		return false
	}

	if !strings.EqualFold(strings.TrimSpace(*request.Extensions.InboundAPIShape), "anthropic") {
		return false
	}

	// Copilot's Claude chat-completions stream carries valid incremental tool_calls,
	// while the buffered non-streaming path can collapse into stop_reason=tool_use
	// without an actual tool block. Preserve native streaming for Claude models.
	model := a.RemapModel(request.Model)
	return !strings.Contains(strings.ToLower(model), "claude")
}

func (a *CopilotAdapter) forceChatCompletions(request *cif.CanonicalRequest) bool {
	return request != nil &&
		request.Extensions != nil &&
		request.Extensions.ForceChatCompletions != nil &&
		*request.Extensions.ForceChatCompletions
}

// isUnsupportedChatCompletionsModel detects Copilot's
// `unsupported_api_for_model` 400 so we can fall back to /responses.
func (a *CopilotAdapter) isUnsupportedChatCompletionsModel(apiErr *copilotAPIError) bool {
	if apiErr == nil || apiErr.statusCode != http.StatusBadRequest {
		return false
	}
	var payload copilotErrorEnvelope
	if err := json.Unmarshal(apiErr.body, &payload); err == nil {
		if payload.Error.Code == "unsupported_api_for_model" {
			return true
		}
		if strings.Contains(strings.ToLower(payload.Error.Message), "/chat/completions") {
			return true
		}
	}
	return strings.Contains(strings.ToLower(string(apiErr.body)), "unsupported_api_for_model")
}

func (a *CopilotAdapter) RemapModel(canonicalModel string) string {
	return modelrouting.NormalizeModelName(canonicalModel)
}

func (a *CopilotAdapter) executeOpenAI(ctx context.Context, request *cif.CanonicalRequest) (*cif.CanonicalResponse, error) {
	return a.executeOpenAIWithRetry(ctx, request, true)
}

func (a *CopilotAdapter) executeOpenAIWithRetry(ctx context.Context, request *cif.CanonicalRequest, allowAuthRetry bool) (*cif.CanonicalResponse, error) {
	toolNameMapper := newCopilotToolNameMapper(request)
	openaiPayload := a.convertCIFToOpenAI(request, toolNameMapper)
	openaiPayload["stream"] = false

	reqBody, err := json.Marshal(openaiPayload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/chat/completions", a.provider.GetBaseURL())
	log.Trace().Str("url", url).RawJSON("payload", reqBody).Msg("outbound proxy request payload")
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	for k, v := range a.requestHeaders(request) {
		req.Header.Set(k, v)
	}

	resp, err := copilotHTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		apiErr := &copilotAPIError{statusCode: resp.StatusCode, body: body}
		if allowAuthRetry && a.shouldRetryAfterAuthError(request, apiErr) && a.refreshTokenForRetry("chat.completions") {
			return a.executeOpenAIWithRetry(ctx, request, false)
		}
		if !a.forceChatCompletions(request) && a.isUnsupportedChatCompletionsModel(apiErr) {
			log.Info().
				Str("model", request.Model).
				Str("provider", a.provider.GetInstanceID()).
				Msg("Copilot model requires responses API, retrying request")
			return a.executeResponses(ctx, request)
		}
		return nil, apiErr
	}

	var openaiResp map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&openaiResp); err != nil {
		return nil, fmt.Errorf("failed to decode response: %w", err)
	}

	return a.convertOpenAIToCIF(openaiResp, toolNameMapper), nil
}

func (a *CopilotAdapter) executeOpenAIStream(ctx context.Context, request *cif.CanonicalRequest) (<-chan cif.CIFStreamEvent, error) {
	return a.executeOpenAIStreamWithRetry(ctx, request, true)
}

func (a *CopilotAdapter) executeOpenAIStreamWithRetry(ctx context.Context, request *cif.CanonicalRequest, allowAuthRetry bool) (<-chan cif.CIFStreamEvent, error) {
	toolNameMapper := newCopilotToolNameMapper(request)
	openaiPayload := a.convertCIFToOpenAI(request, toolNameMapper)
	openaiPayload["stream"] = true

	reqBody, err := json.Marshal(openaiPayload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal request: %w", err)
	}

	url := fmt.Sprintf("%s/chat/completions", a.provider.GetBaseURL())
	log.Trace().Str("url", url).RawJSON("payload", reqBody).Msg("outbound proxy request payload")
	req, err := http.NewRequestWithContext(ctx, "POST", url, bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	for k, v := range a.requestHeaders(request) {
		req.Header.Set(k, v)
	}
	req.Header.Set("Accept", "text/event-stream")

	// Streaming requests must not use a fixed client timeout; stream length is model dependent.
	resp, err := copilotStreamClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("streaming request failed: %w", err)
	}

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		apiErr := &copilotAPIError{statusCode: resp.StatusCode, body: body}
		if allowAuthRetry && a.shouldRetryAfterAuthError(request, apiErr) && a.refreshTokenForRetry("chat.completions-stream") {
			return a.executeOpenAIStreamWithRetry(ctx, request, false)
		}
		if !a.forceChatCompletions(request) && a.isUnsupportedChatCompletionsModel(apiErr) {
			log.Info().
				Str("model", request.Model).
				Str("provider", a.provider.GetInstanceID()).
				Msg("Copilot model requires responses API for streaming, retrying request")
			return a.executeResponsesStream(ctx, request)
		}
		return nil, apiErr
	}

	eventCh := make(chan cif.CIFStreamEvent, 64)
	go a.parseOpenAISSE(resp.Body, eventCh, toolNameMapper)
	return eventCh, nil
}

func (a *CopilotAdapter) shouldRetryAfterAuthError(request *cif.CanonicalRequest, apiErr *copilotAPIError) bool {
	if request != nil &&
		request.Extensions != nil &&
		request.Extensions.DisableAuthRetry != nil &&
		*request.Extensions.DisableAuthRetry {
		return false
	}

	return apiErr != nil && apiErr.IsAuthenticationError() && a.provider.githubToken != ""
}

func (a *CopilotAdapter) requestHeaders(request *cif.CanonicalRequest) map[string]string {
	headers := a.provider.GetHeaders(a.requestUsesVision(request))
	headers["X-Initiator"] = a.requestInitiator(request)
	return headers
}

func (a *CopilotAdapter) requestUsesVision(request *cif.CanonicalRequest) bool {
	if request == nil {
		return false
	}

	for _, message := range request.Messages {
		for _, part := range messageContentParts(message) {
			if _, ok := part.(cif.CIFImagePart); ok {
				return true
			}
		}
	}

	return false
}

func (a *CopilotAdapter) requestInitiator(request *cif.CanonicalRequest) string {
	if request == nil {
		return "user"
	}

	for _, message := range request.Messages {
		switch msg := message.(type) {
		case cif.CIFAssistantMessage:
			if len(msg.Content) > 0 {
				return "agent"
			}
		case cif.CIFUserMessage:
			for _, part := range msg.Content {
				if _, ok := part.(cif.CIFToolResultPart); ok {
					return "agent"
				}
			}
		}
	}

	return "user"
}

func messageContentParts(message cif.CIFMessage) []cif.CIFContentPart {
	switch msg := message.(type) {
	case cif.CIFUserMessage:
		return msg.Content
	case cif.CIFAssistantMessage:
		return msg.Content
	default:
		return nil
	}
}

func (a *CopilotAdapter) refreshTokenForRetry(endpoint string) bool {
	if err := a.provider.RefreshToken(); err != nil {
		log.Warn().
			Err(err).
			Str("provider", a.provider.GetInstanceID()).
			Str("endpoint", endpoint).
			Msg("Failed to refresh Copilot token after upstream auth error")
		return false
	}

	log.Info().
		Str("provider", a.provider.GetInstanceID()).
		Str("endpoint", endpoint).
		Msg("Refreshed Copilot token after upstream auth error, retrying request")
	return true
}

package alibaba

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"omnillm/internal/cif"
	"omnillm/internal/providers/openaicompat"
	"omnillm/internal/providers/shared"
	"omnillm/internal/providers/types"
	"strings"

	"github.com/rs/zerolog/log"
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
	if IsReasoningModel(model) && len(request.Tools) == 0 {
		extras["enable_thinking"] = true
	}
	if isDeepSeekV4Model(model) && len(request.Tools) > 0 {
		delete(extras, "enable_thinking")
		extras["thinking"] = map[string]any{"type": "disabled"}
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
	if isDeepSeekV4Model(model) && len(request.Tools) > 0 {
		chatReq.ToolChoice = nil
	}
	return chatReq, nil
}

func isDeepSeekV4Model(modelID string) bool {
	lower := strings.ToLower(strings.TrimSpace(modelID))
	return strings.Contains(lower, "deepseek-v4")
}

// ErrHardcodedFallback is returned alongside a hardcoded model list when the
// live DashScope /models API is unavailable. Callers that cache model lists
// should treat this as a soft error and skip caching so a future request can
// retry the live API.
var ErrHardcodedFallback = fmt.Errorf("alibaba: models fetch failed, using hardcoded fallback")

// GetModels returns the available models for this Alibaba instance.
// If the live API is unreachable it returns the hardcoded Qwen-only catalog
// together with ErrHardcodedFallback so callers can decide whether to cache.
func GetModels(instanceID, token, baseURL string, config map[string]any) (*types.ModelsResponse, error) {
	if token == "" {
		return nil, fmt.Errorf("alibaba: not authenticated")
	}
	resp, err := FetchModelsFromAPI(instanceID, token, baseURL, config)
	if err == nil && len(resp.Data) > 0 {
		return resp, nil
	}
	log.Warn().Err(err).Str("provider", instanceID).Msg("alibaba: falling back to hardcoded model list")
	return GetModelsHardcoded(instanceID), ErrHardcodedFallback
}

// GetModelsHardcoded returns the fallback model catalog (Qwen models only).
// DeepSeek and other third-party models available on DashScope are only
// surfaced when FetchModelsFromAPI succeeds, because DashScope account plans
// vary and not every key has access to every model.
func GetModelsHardcoded(instanceID string) *types.ModelsResponse {
	var result []types.Model
	for _, m := range Models {
		if strings.Contains(strings.ToLower(m.ID), "deepseek") {
			continue // only include from live API — plan access varies
		}
		entry := m
		entry.Provider = instanceID
		result = append(result, entry)
	}
	return &types.ModelsResponse{Data: result, Object: "list"}
}

// FetchModelsFromAPI fetches available models from the DashScope API.
func FetchModelsFromAPI(instanceID, token, baseURL string, _ map[string]any) (*types.ModelsResponse, error) {
	url := strings.TrimRight(baseURL, "/") + "/models"
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("alibaba: failed to create models request: %w", err)
	}
	for k, v := range Headers(token, false, nil) {
		req.Header.Set(k, v)
	}

	resp, err := alibabaHTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("alibaba: models request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("alibaba: models fetch failed (%d)", resp.StatusCode)
	}

	b, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("alibaba: failed to read models response: %w", err)
	}

	var payload struct {
		Data []struct {
			ID string `json:"id"`
		} `json:"data"`
	}
	if err := json.NewDecoder(bytes.NewReader(b)).Decode(&payload); err != nil {
		return nil, fmt.Errorf("alibaba: failed to decode models response: %w", err)
	}

	models := make([]types.Model, 0, len(payload.Data))
	for _, item := range payload.Data {
		if item.ID == "" || !IsChatCompletionsModel(item.ID) {
			continue
		}
		m := types.Model{ID: item.ID, Name: item.ID, Provider: instanceID}
		if meta, ok := ModelMetadata(item.ID); ok {
			if meta.Name != "" {
				m.Name = meta.Name
			}
			m.Description = meta.Description
			m.Capabilities = meta.Capabilities
			m.MaxTokens = meta.MaxTokens
		}
		models = append(models, m)
	}
	return &types.ModelsResponse{Data: models, Object: "list"}, nil
}

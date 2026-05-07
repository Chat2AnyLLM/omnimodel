package alibaba

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"omnillm/internal/providers/types"
	"omnillm/internal/services/modelsmeta"
	"strings"

	"github.com/rs/zerolog/log"
)

// ErrHardcodedFallback is returned alongside a hardcoded model list when the
// live DashScope /models API is unavailable. Callers that cache model lists
// should treat this as a soft error and skip caching so a future request can
// retry the live API.
var ErrHardcodedFallback = fmt.Errorf("alibaba: models fetch failed, using hardcoded fallback")

// GetModels returns the available models for this Alibaba instance.
// If the live API is unreachable it returns the Qwen-only catalog from
// models.dev together with ErrHardcodedFallback so callers can decide whether
// to cache.
func GetModels(instanceID, token, baseURL string, config map[string]any) (*types.ModelsResponse, error) {
	if token == "" {
		return nil, fmt.Errorf("alibaba: not authenticated")
	}
	resp, err := FetchModelsFromAPI(instanceID, token, baseURL, config)
	if err == nil && len(resp.Data) > 0 {
		return resp, nil
	}
	log.Warn().Err(err).Str("provider", instanceID).Msg("alibaba: falling back to models.dev catalog")
	return GetModelsHardcoded(instanceID), ErrHardcodedFallback
}

// GetModelsHardcoded returns a fallback catalog of Qwen models sourced from
// models.dev. DeepSeek and other third-party models hosted on DashScope are
// intentionally excluded; they are only surfaced when FetchModelsFromAPI
// succeeds, because DashScope account plans vary.
func GetModelsHardcoded(instanceID string) *types.ModelsResponse {
	result, err := modelsmeta.DefaultService.Get(context.Background(), false)
	if err != nil || len(result.Models) == 0 {
		return &types.ModelsResponse{Data: nil, Object: "list"}
	}
	var models []types.Model
	for _, m := range result.Models {
		if !strings.HasPrefix(strings.ToLower(m.ID), "qwen") {
			continue
		}
		model := types.Model{
			ID:       m.ID,
			Name:     m.ID,
			Provider: instanceID,
		}
		if m.Name != "" {
			model.Name = m.Name
		}
		if m.OutputLimitTokens != nil {
			model.MaxTokens = *m.OutputLimitTokens
		} else if m.ContextLimitTokens != nil {
			model.MaxTokens = *m.ContextLimitTokens
		}
		models = append(models, model)
	}
	return &types.ModelsResponse{Data: models, Object: "list"}
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
		if meta := modelsmeta.DefaultService.LookupModel(context.Background(), item.ID); meta != nil {
			if meta.Name != "" {
				m.Name = meta.Name
			}
			if meta.OutputLimitTokens != nil {
				m.MaxTokens = *meta.OutputLimitTokens
			} else if meta.ContextLimitTokens != nil {
				m.MaxTokens = *meta.ContextLimitTokens
			}
		}
		models = append(models, m)
	}
	return &types.ModelsResponse{Data: models, Object: "list"}, nil
}

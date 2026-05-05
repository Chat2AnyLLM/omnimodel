package kimi

import (
	"context"
	"encoding/base64"
	"encoding/json"
	"fmt"
	"net/http"
	"omnillm/internal/cif"
	"omnillm/internal/database"
	"omnillm/internal/providers/shared"
	"omnillm/internal/providers/types"
	"strings"
	"time"

	"github.com/rs/zerolog/log"
)

const UserAgent = "KimiClient/1.0"

// Base URL for Kimi API
const BaseURL = "https://api.moonshot.cn/v1"

// Shared HTTP clients.
var (
	kimiHTTPClient   = &http.Client{Timeout: 120 * time.Second}
	kimiStreamClient = &http.Client{}
)

// Model catalog for Kimi.
var Models = []types.Model{
	{ID: "kimi-k2-thinking", Name: "Kimi K2 Thinking", MaxTokens: 131072, Provider: "kimi"},
	{ID: "kimi-k2.5", Name: "Kimi K2.5", MaxTokens: 131072, Provider: "kimi"},
	{ID: "kimi-k2.6", Name: "Kimi K2.6", MaxTokens: 131072, Provider: "kimi"},
	{ID: "moonshot-v1-8k", Name: "Moonshot v1 8K", MaxTokens: 8192, Provider: "kimi"},
	{ID: "moonshot-v1-32k", Name: "Moonshot v1 32K", MaxTokens: 32768, Provider: "kimi"},
	{ID: "moonshot-v1-128k", Name: "Moonshot v1 128K", MaxTokens: 131072, Provider: "kimi"},
}

// IsThinkingModel reports whether the given model ID has thinking always
// enabled. These models require reasoning_content to be present in every
// assistant message that contains tool_calls (even when empty), otherwise
// the API returns a 400 error.
// All kimi-k2* models (kimi-k2.5, kimi-k2.6, kimi-k2-thinking, …) are
// thinking models.
func IsThinkingModel(model string) bool {
	return strings.HasPrefix(model, "kimi-k2")
}

// EnsureReasoningContentInMessages post-processes serialized OpenAI-compatible
// messages for thinking models. Kimi's API requires that every assistant
// message containing tool_calls also carries a reasoning_content field (even
// an empty string). Without it the API rejects multi-turn tool-use requests.
func EnsureReasoningContentInMessages(messages []map[string]interface{}) {
	for _, msg := range messages {
		role, _ := msg["role"].(string)
		if role != "assistant" {
			continue
		}
		toolCalls, hasToolCalls := msg["tool_calls"]
		if !hasToolCalls || toolCalls == nil {
			continue
		}
		if _, ok := msg["reasoning_content"]; !ok {
			msg["reasoning_content"] = ""
		}
	}
}

// OAuthSupportedModels lists model IDs available to OAuth-authenticated users.
var OAuthSupportedModels = map[string]bool{}

// TokenData represents the token data structure for Kimi
type TokenData struct {
	AccessToken  string `json:"access_token"`
	RefreshToken string `json:"refresh_token,omitempty"`
	TokenType    string `json:"token_type"`
	ExpiresIn    int64  `json:"expires_in"`
	CreatedAt    int64  `json:"created_at"`
	AuthType     string `json:"auth_type"`
	BaseURL      string `json:"base_url,omitempty"`
	ResourceURL  string `json:"resource_url,omitempty"`
}

// IsExpiringSoon checks if the token expires within the next 5 minutes
func IsExpiringSoon(td *TokenData) bool {
	if td.CreatedAt == 0 || td.ExpiresIn == 0 {
		return false
	}
	expirationTime := time.Unix(td.CreatedAt, 0).Add(time.Duration(td.ExpiresIn) * time.Second)
	return time.Until(expirationTime) < 5*time.Minute
}

// SetupAPIKeyAuth handles the api-key auth method for Kimi.
func SetupAPIKeyAuth(instanceID string, options *types.AuthOptions) (token, baseURL, name string, config map[string]interface{}, err error) {
	if options.APIKey == "" {
		return "", "", "", nil, fmt.Errorf("kimi: API key is required")
	}
	region := strings.TrimSpace(options.Region)
	if region == "" {
		region = "global"
	}

	tokenStore := database.NewTokenStore()
	tokenData := map[string]interface{}{"access_token": options.APIKey}
	if err := tokenStore.Save(instanceID, tokenData); err != nil {
		return "", "", "", nil, fmt.Errorf("failed to save kimi token: %w", err)
	}

	cfg := map[string]interface{}{
		"auth_type": "api-key",
		"region":    region,
	}
	if endpoint := strings.TrimSpace(options.Endpoint); endpoint != "" {
		cfg["base_url"] = endpoint
	}

	configStore := database.NewProviderConfigStore()
	if err := configStore.Save(instanceID, cfg); err != nil {
		return "", "", "", nil, fmt.Errorf("failed to save kimi config: %w", err)
	}

	resolvedURL := NormalizeBaseURL(cfg)
	resolvedName := APIKeyProviderName(cfg)

	log.Info().
		Str("provider", instanceID).
		Str("region", region).
		Msg("Kimi authenticated via API key")

	return options.APIKey, resolvedURL, resolvedName, cfg, nil
}

// LoadTokenFromDB reads the persisted Kimi token from the database.
func LoadTokenFromDB(instanceID string) (token, baseURL string, config map[string]interface{}, err error) {
	tokenStore := database.NewTokenStore()
	record, err := tokenStore.Get(instanceID)
	if err != nil {
		return "", "", nil, fmt.Errorf("kimi: failed to load token from DB: %w", err)
	}
	if record == nil {
		return "", "", nil, nil
	}

	var td TokenData
	if err := json.Unmarshal([]byte(record.TokenData), &td); err != nil {
		return "", "", nil, fmt.Errorf("kimi: failed to parse token data: %w", err)
	}

	cfg := map[string]interface{}{
		"auth_type":    td.AuthType,
		"base_url":     td.BaseURL,
		"resource_url": td.ResourceURL,
	}
	resolvedURL := NormalizeBaseURL(cfg)
	return td.AccessToken, resolvedURL, cfg, nil
}

// SaveOAuthToken persists a completed OAuth token for the given provider instance.
func SaveOAuthToken(instanceID string, td *TokenData) (newInstanceID, name, baseURL string, err error) {
	cfg := map[string]interface{}{
		"auth_type":    td.AuthType,
		"base_url":     td.BaseURL,
		"resource_url": td.ResourceURL,
	}
	resolvedURL := NormalizeBaseURL(cfg)

	region := "china"
	if td.ResourceURL != "" && strings.Contains(strings.ToLower(td.ResourceURL), "intl") {
		region = "global"
	} else if resolvedURL != "" && strings.Contains(strings.ToLower(resolvedURL), "intl") {
		region = "global"
	}

	email := ExtractEmailFromJWT(td.AccessToken)
	if email != "" {
		safe := strings.ReplaceAll(email, "@", "-")
		safe = strings.ReplaceAll(safe, ".", "-")
		newInstanceID = "kimi-oauth-" + region + "-" + safe
		name = "Kimi (" + email + ")"
	} else {
		suffix := ShortTokenSuffix(td.AccessToken)
		newInstanceID = "kimi-oauth-" + region + "-" + suffix
		name = "Kimi OAuth (" + suffix + ")"
	}

	tokenStore := database.NewTokenStore()
	if err := tokenStore.Save(newInstanceID, td); err != nil {
		return "", "", "", fmt.Errorf("kimi: failed to save OAuth token: %w", err)
	}

	if instanceID != newInstanceID {
		_ = tokenStore.Delete(instanceID)
	}

	log.Info().
		Str("instanceID", newInstanceID).
		Str("name", name).
		Str("resource_url", td.ResourceURL).
		Msg("Kimi OAuth token saved")

	return newInstanceID, name, resolvedURL, nil
}

// EnsureFreshToken refreshes the OAuth token if it is about to expire.
func EnsureFreshToken(instanceID, currentToken string) string {
	tokenStore := database.NewTokenStore()
	record, err := tokenStore.Get(instanceID)
	if err != nil || record == nil {
		return currentToken
	}

	var td TokenData
	if err := json.Unmarshal([]byte(record.TokenData), &td); err != nil {
		return currentToken
	}

	if !IsExpiringSoon(&td) {
		return td.AccessToken
	}

	if td.RefreshToken == "" {
		log.Warn().Str("provider", instanceID).Msg("Kimi OAuth token expiring but no refresh token available")
		return currentToken
	}

	refreshed, err := RefreshToken(context.Background(), td.RefreshToken)
	if err != nil {
		log.Warn().Err(err).Str("provider", instanceID).Msg("Kimi OAuth token refresh failed")
		return currentToken
	}

	_, _, _, saveErr := SaveOAuthToken(instanceID, refreshed)
	if saveErr != nil {
		log.Warn().Err(saveErr).Str("provider", instanceID).Msg("Failed to persist refreshed Kimi token")
	}

	return refreshed.AccessToken
}

// RefreshToken attempts to refresh an expired OAuth token.
func RefreshToken(ctx context.Context, refreshToken string) (*TokenData, error) {
	return nil, fmt.Errorf("refresh not implemented for Kimi")
}

// Headers returns the HTTP headers required for Kimi API requests.
func Headers(token string, stream bool, config map[string]interface{}) map[string]string {
	headers := map[string]string{
		"Authorization": "Bearer " + token,
		"Content-Type":  "application/json",
		"Accept":        "application/json",
	}
	if stream {
		headers["Accept"] = "text/event-stream"
	}

	if authType, _ := shared.FirstString(config, "auth_type", "authType"); authType == "oauth" {
		headers["User-Agent"] = UserAgent
	}
	return headers
}

// ChatURL returns the chat completions URL for the given base URL.
func ChatURL(baseURL string) string {
	base := baseURL
	if base == "" {
		base = BaseURL
	}
	return base + "/chat/completions"
}

// ModelsURL returns the models list URL for the given base URL.
func ModelsURL(baseURL string) string {
	base := baseURL
	if base == "" {
		base = BaseURL
	}
	return base + "/models"
}

// GetModels returns the available models for this Kimi instance.
func GetModels(instanceID, token, baseURL string, config map[string]interface{}) (*types.ModelsResponse, error) {
	if token == "" {
		return nil, fmt.Errorf("kimi: not authenticated (set access_token via admin UI)")
	}

	authType, _ := shared.FirstString(config, "auth_type", "authType")
	if authType == "oauth" {
		return GetModelsHardcoded(instanceID, config), nil
	}

	resp, err := FetchModelsFromAPI(instanceID, token, baseURL)
	if err == nil && len(resp.Data) > 0 {
		return resp, nil
	}

	log.Warn().Err(err).Str("provider", instanceID).Msg("Failed to fetch models from API, using hardcoded list")
	return GetModelsHardcoded(instanceID, config), nil
}

// GetModelsHardcoded returns the hardcoded model catalog for this Kimi instance.
func GetModelsHardcoded(instanceID string, config map[string]interface{}) *types.ModelsResponse {
	models := Models
	authType, _ := shared.FirstString(config, "auth_type", "authType")
	if authType == "oauth" {
		filtered := make([]types.Model, 0, len(models))
		for _, model := range models {
			if OAuthSupportedModels[model.ID] {
				filtered = append(filtered, model)
			}
		}
		models = filtered
	}
	if models == nil {
		models = []types.Model{}
	}
	result := make([]types.Model, len(models))
	for i, m := range models {
		result[i] = m
		result[i].Provider = instanceID
	}
	return &types.ModelsResponse{Data: result, Object: "list"}
}

// FetchModelsFromAPI fetches available models from the Kimi API.
func FetchModelsFromAPI(instanceID, token, baseURL string) (*types.ModelsResponse, error) {
	url := ModelsURL(baseURL)
	req, err := http.NewRequest(http.MethodGet, url, nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create kimi models request: %w", err)
	}

	for key, value := range Headers(token, false, map[string]interface{}{}) {
		req.Header.Set(key, value)
	}

	resp, err := kimiHTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("kimi models request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode < http.StatusOK || resp.StatusCode >= http.StatusMultipleChoices {
		return nil, fmt.Errorf("kimi models fetch failed (%d)", resp.StatusCode)
	}

	var payload struct {
		Data []struct {
			ID        string `json:"id"`
			Name      string `json:"name"`
			MaxTokens int    `json:"max_tokens,omitempty"`
		} `json:"data"`
	}
	if err := json.NewDecoder(resp.Body).Decode(&payload); err != nil {
		return nil, fmt.Errorf("failed to decode kimi models response: %w", err)
	}

	models := make([]types.Model, 0, len(payload.Data))
	for _, model := range payload.Data {
		if model.ID == "" {
			continue
		}
		if !IsChatCompletionsModel(model.ID) {
			continue
		}
		result := types.Model{
			ID:        model.ID,
			Name:      model.Name,
			Provider:  instanceID,
			MaxTokens: model.MaxTokens,
		}
		if metadata, ok := ModelMetadata(model.ID); ok {
			if metadata.Name != "" {
				result.Name = metadata.Name
			}
			result.Description = metadata.Description
			result.Capabilities = metadata.Capabilities
			result.MaxTokens = metadata.MaxTokens
		}
		models = append(models, result)
	}
	return &types.ModelsResponse{Data: models, Object: "list"}, nil
}

// IsChatCompletionsModel returns true if the model supports chat completions.
func IsChatCompletionsModel(modelID string) bool {
	return !strings.Contains(strings.ToLower(modelID), "realtime")
}

// ModelMetadata returns the hardcoded metadata for the given model ID if known.
func ModelMetadata(modelID string) (types.Model, bool) {
	for _, m := range Models {
		if m.ID == modelID {
			return m, true
		}
	}
	return types.Model{}, false
}

// NormalizeBaseURL derives the base URL from a provider config map.
func NormalizeBaseURL(config map[string]interface{}) string {
	if baseURL, ok := shared.FirstString(config, "base_url", "baseUrl"); ok {
		return EnsureBaseURL(baseURL)
	}
	if resourceURL, ok := shared.FirstString(config, "resource_url", "resourceUrl"); ok {
		return EnsureBaseURL(resourceURL)
	}
	return EnsureBaseURL(BaseURL)
}

// APIKeyProviderName returns the display name for an API-key authenticated Kimi provider.
func APIKeyProviderName(config map[string]interface{}) string {
	region, _ := shared.FirstString(config, "region")
	if region == "" {
		region = "global"
	}
	return "Kimi (" + region + ")"
}

// EnsureBaseURL normalizes a base URL to have https scheme and /v1 suffix.
func EnsureBaseURL(raw string) string {
	baseURL := strings.TrimSpace(raw)
	if baseURL == "" {
		baseURL = BaseURL
	}
	if !strings.HasPrefix(baseURL, "http://") && !strings.HasPrefix(baseURL, "https://") {
		baseURL = "https://" + baseURL
	}
	baseURL = strings.TrimRight(baseURL, "/")
	if !strings.HasSuffix(baseURL, "/v1") {
		baseURL += "/v1"
	}
	return baseURL
}

// IsJWT checks if a token has the format of a JWT (xxx.yyy.zzz).
func IsJWT(token string) bool {
	parts := strings.Split(token, ".")
	return len(parts) == 3 && len(parts[0]) > 0 && len(parts[1]) > 0 && len(parts[2]) > 0
}

// ShortTokenSuffix returns the last 5 characters of a token for display purposes.
func ShortTokenSuffix(token string) string {
	trimmed := strings.TrimSpace(token)
	if len(trimmed) >= 5 {
		return trimmed[len(trimmed)-5:]
	}
	if trimmed == "" {
		return "oauth"
	}
	return trimmed
}

// ExtractEmailFromJWT decodes the payload of a JWT and extracts the email claim.
func ExtractEmailFromJWT(token string) string {
	parts := strings.Split(token, ".")
	if len(parts) != 3 {
		return ""
	}
	payload := parts[1]
	switch len(payload) % 4 {
	case 2:
		payload += "=="
	case 3:
		payload += "="
	}
	decoded, err := base64.URLEncoding.DecodeString(payload)
	if err != nil {
		return ""
	}
	var claims struct {
		Email string `json:"email"`
	}
	if err := json.Unmarshal(decoded, &claims); err != nil {
		return ""
	}
	return claims.Email
}

// BuildOpenAIPayload builds the OpenAI-compatible request payload for Kimi.
func BuildOpenAIPayload(model string, messages []map[string]interface{}, request *cif.CanonicalRequest, isOAuth bool) map[string]interface{} {
	payload := map[string]interface{}{
		"model":    model,
		"messages": messages,
	}

	if request.Temperature != nil {
		payload["temperature"] = *request.Temperature
	}
	if request.TopP != nil {
		payload["top_p"] = *request.TopP
	}
	if request.MaxTokens != nil {
		payload["max_tokens"] = *request.MaxTokens
	}
	if len(request.Stop) > 0 {
		payload["stop"] = request.Stop
	}
	if request.UserID != nil {
		payload["user"] = *request.UserID
	}

	if len(request.Tools) > 0 {
		tools := make([]map[string]interface{}, 0, len(request.Tools))
		for _, tool := range request.Tools {
			t := map[string]interface{}{
				"type": "function",
				"function": map[string]interface{}{
					"name":       tool.Name,
					"parameters": shared.NormalizeToolParameters(tool.ParametersSchema),
				},
			}
			if tool.Description != nil {
				t["function"].(map[string]interface{})["description"] = *tool.Description
			}
			tools = append(tools, t)
		}
		payload["tools"] = tools
	}

	if request.ToolChoice != nil {
		if toolChoice := shared.ConvertCanonicalToolChoiceToOpenAI(request.ToolChoice); toolChoice != nil {
			if IsThinkingModel(model) && len(request.Tools) > 0 {
				payload["thinking"] = map[string]any{"type": "disabled"}
			} else {
				payload["tool_choice"] = toolChoice
			}
		}
	}

	return payload
}

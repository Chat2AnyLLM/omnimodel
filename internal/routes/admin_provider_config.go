package routes

import (
	"encoding/json"

	"github.com/rs/zerolog/log"

	"omnillm/internal/database"
	"omnillm/internal/providers/shared"
	"omnillm/internal/providers/types"
)

type providerModelView struct {
	ID           string                 `json:"id"`
	Name         string                 `json:"name,omitempty"`
	Description  string                 `json:"description,omitempty"`
	MaxTokens    int                    `json:"max_tokens,omitempty"`
	OutputTokens int                    `json:"output_tokens,omitempty"`
	Enabled      bool                   `json:"enabled"`
	Capabilities map[string]interface{} `json:"capabilities,omitempty"`
}

func loadProviderConfig(instanceID string) (map[string]interface{}, error) {
	configStore := database.NewProviderConfigStore()
	record, err := configStore.Get(instanceID)
	if err != nil || record == nil {
		return nil, err
	}

	var config map[string]interface{}
	if err := json.Unmarshal([]byte(record.ConfigData), &config); err != nil {
		return nil, err
	}

	return config, nil
}

func normalizeProviderConfigForFrontend(providerType string, config map[string]interface{}) map[string]interface{} {
	if len(config) == 0 {
		return nil
	}

	switch providerType {
	case "azure-openai":
		normalized := map[string]interface{}{}
		if endpoint, ok := firstStringValue(config, "endpoint"); ok {
			normalized["endpoint"] = endpoint
		}
		if apiVersion, ok := firstStringValue(config, "apiVersion", "api_version"); ok {
			normalized["apiVersion"] = apiVersion
		}
		if deployments, ok := config["deployments"]; ok {
			switch typed := deployments.(type) {
			case []string:
				if len(typed) > 0 {
					normalized["deployments"] = typed
				}
			case []interface{}:
				if len(typed) > 0 {
					normalized["deployments"] = typed
				}
			}
		}
		if len(normalized) == 0 {
			return nil
		}
		return normalized
	case "alibaba":
		normalized := map[string]interface{}{}
		if baseURL, ok := firstStringValue(config, "baseUrl", "base_url"); ok {
			normalized["baseUrl"] = baseURL
		}
		if region, ok := firstStringValue(config, "region"); ok {
			normalized["region"] = region
		}
		if plan, ok := firstStringValue(config, "plan"); ok {
			normalized["plan"] = plan
		}
		if apiFormat, ok := firstStringValue(config, "apiFormat", "api_format"); ok {
			normalized["apiFormat"] = apiFormat
		}
		if len(normalized) == 0 {
			return nil
		}
		return normalized
	case "openai-compatible":
		normalized := map[string]interface{}{}
		if endpoint, ok := firstStringValue(config, "base_url", "endpoint"); ok {
			normalized["endpoint"] = endpoint
		}
		if apiFormat, ok := firstStringValue(config, "apiFormat", "api_format"); ok {
			if normalizedFormat := normalizeOpenAICompatibleAPIFormatConfig(apiFormat); normalizedFormat != "" {
				normalized["apiFormat"] = normalizedFormat
			}
		}
		if models := stringSliceValue(config["models"]); len(models) > 0 {
			normalized["models"] = models
		}
		if len(normalized) == 0 {
			return nil
		}
		return normalized
	default:
		return config
	}
}

func normalizeProviderConfigForStorage(providerType string, config map[string]interface{}) map[string]interface{} {
	switch providerType {
	case "azure-openai":
		endpoint, _ := firstStringValue(config, "endpoint")
		apiVersion, _ := firstStringValue(config, "apiVersion", "api_version")

		normalized := map[string]interface{}{}
		if endpoint != "" {
			normalized["endpoint"] = endpoint
		}
		if apiVersion != "" {
			normalized["api_version"] = apiVersion
		}
		if deployments, ok := config["deployments"]; ok {
			switch typed := deployments.(type) {
			case []string:
				if len(typed) > 0 {
					normalized["deployments"] = typed
				}
			case []interface{}:
				if len(typed) > 0 {
					normalized["deployments"] = typed
				}
			}
		}
		return normalized
	case "alibaba":
		baseURL, _ := firstStringValue(config, "baseUrl", "base_url")
		region, _ := firstStringValue(config, "region")
		plan, _ := firstStringValue(config, "plan")
		apiFormat, _ := firstStringValue(config, "apiFormat", "api_format")

		normalized := map[string]interface{}{}
		if baseURL != "" {
			normalized["base_url"] = baseURL
		}
		if region != "" {
			normalized["region"] = region
		}
		if plan != "" {
			normalized["plan"] = plan
		}
		if apiFormat != "" {
			normalized["api_format"] = apiFormat
		}
		return normalized
	case "openai-compatible":
		normalized := map[string]interface{}{}
		if baseURL, _ := firstStringValue(config, "base_url", "endpoint"); baseURL != "" {
			normalized["base_url"] = baseURL
		}
		if _, ok := config["models"]; ok {
			normalized["models"] = stringSliceValue(config["models"])
		}
		if apiFormat, ok := firstStringValue(config, "apiFormat", "api_format"); ok {
			if normalizedFormat := normalizeOpenAICompatibleAPIFormatConfig(apiFormat); normalizedFormat != "" {
				normalized["api_format"] = normalizedFormat
			}
		}
		return normalized
	default:
		return config
	}
}

// loadProviderModels returns the model list for a provider, using a database
// cache with a 24h TTL. When forceRefresh is true, it bypasses the cache and
// always calls the provider's external API.
func loadProviderModels(provider types.Provider, forceRefresh bool) ([]providerModelView, error) {
	instanceID := provider.GetInstanceID()
	cacheStore := database.NewProviderModelsCacheStore()
	stateStore := database.NewModelStateStore()

	states, err := stateStore.GetAllByInstance(instanceID)
	if err != nil {
		return nil, err
	}

	stateByID := make(map[string]database.ProviderModelStateRecord, len(states))
	for _, state := range states {
		stateByID[state.ModelID] = state
	}

	// Check cache first (unless force refresh)
	if !forceRefresh {
		if cached, err := cacheStore.Get(instanceID, provider.GetID(), database.DefaultCacheTTL); err == nil && cached != nil {
			var models []providerModelView
			if err := json.Unmarshal([]byte(cached.ModelsData), &models); err == nil {
				// Re-apply enabled states from DB (may have changed since cache)
				for i := range models {
					if state, ok := stateByID[models[i].ID]; ok {
						models[i].Enabled = state.Enabled
					}
				}
				return models, nil
			}
		}
	}

	// Cache miss or force refresh — call external API
	modelsResp, err := provider.GetModels()
	if err != nil {
		if len(states) == 0 {
			return nil, err
		}

		models := make([]providerModelView, 0, len(states))
		for _, state := range states {
			models = append(models, providerModelView{
				ID:      state.ModelID,
				Name:    state.ModelID,
				Enabled: state.Enabled,
			})
		}
		return models, nil
	}

	models := make([]providerModelView, 0, len(modelsResp.Data)+len(states))
	seen := make(map[string]struct{}, len(modelsResp.Data))

	for _, model := range modelsResp.Data {
		if _, exists := seen[model.ID]; exists {
			continue
		}
		enabled := true
		if state, ok := stateByID[model.ID]; ok {
			enabled = state.Enabled
		}

		models = append(models, providerModelView{
			ID:           model.ID,
			Name:         model.Name,
			Description:  model.Description,
			MaxTokens:    model.MaxTokens,
			OutputTokens: model.OutputTokens,
			Enabled:      enabled,
			Capabilities: model.Capabilities,
		})
		seen[model.ID] = struct{}{}
	}

	// Merge user-defined models from provider config (e.g. openai-compatible).
	if cfg, err := loadProviderConfig(instanceID); err == nil && cfg != nil {
		for _, modelID := range stringSliceValue(cfg["models"]) {
			if modelID == "" {
				continue
			}
			if _, exists := seen[modelID]; exists {
				continue
			}
			enabled := true
			if state, ok := stateByID[modelID]; ok {
				enabled = state.Enabled
			}
			models = append(models, providerModelView{
				ID:      modelID,
				Name:    modelID,
				Enabled: enabled,
			})
			seen[modelID] = struct{}{}
		}
	}

	for _, state := range states {
		if _, exists := seen[state.ModelID]; exists {
			continue
		}

		models = append(models, providerModelView{
			ID:      state.ModelID,
			Name:    state.ModelID,
			Enabled: state.Enabled,
		})
	}

	// Save to cache
	if modelsJSON, err := json.Marshal(models); err == nil {
		if err := cacheStore.Save(instanceID, provider.GetID(), string(modelsJSON)); err != nil {
			log.Warn().Err(err).Str("provider", instanceID).Msg("Failed to cache provider models")
		}
	}

	return models, nil
}

func countEnabledModels(models []providerModelView) int {
	enabled := 0
	for _, model := range models {
		if model.Enabled {
			enabled++
		}
	}

	return enabled
}

func firstStringValue(values map[string]interface{}, keys ...string) (string, bool) {
	for _, key := range keys {
		if value, ok := values[key].(string); ok && value != "" {
			return value, true
		}
	}

	return "", false
}

func stringSliceValue(raw interface{}) []string {
	switch value := raw.(type) {
	case []string:
		return value
	case []interface{}:
		result := make([]string, 0, len(value))
		for _, item := range value {
			if text, ok := item.(string); ok && text != "" {
				result = append(result, text)
			}
		}
		return result
	default:
		return nil
	}
}

func normalizeOpenAICompatibleAPIFormatConfig(raw string) string {
	return shared.NormalizeOpenAICompatibleAPIFormat(raw)
}

func mergeOpenAICompatibleAPIFormat(merged, incomingConfig map[string]interface{}) {
	_, hasCamel := incomingConfig["apiFormat"]
	_, hasSnake := incomingConfig["api_format"]
	if !hasCamel && !hasSnake {
		return
	}

	if apiFormat, ok := firstStringValue(incomingConfig, "apiFormat", "api_format"); ok {
		if normalizedFormat := normalizeOpenAICompatibleAPIFormatConfig(apiFormat); normalizedFormat != "" {
			merged["api_format"] = normalizedFormat
			return
		}
	}

	delete(merged, "api_format")
}

func cloneConfigMap(config map[string]interface{}) map[string]interface{} {
	if len(config) == 0 {
		return map[string]interface{}{}
	}
	cloned := make(map[string]interface{}, len(config))
	for key, value := range config {
		cloned[key] = value
	}
	return cloned
}

func mergeOpenAICompatibleConfig(previousConfig, incomingConfig, normalizedConfig map[string]interface{}) map[string]interface{} {
	merged := cloneConfigMap(previousConfig)
	for key, value := range normalizedConfig {
		merged[key] = value
	}

	if _, ok := incomingConfig["models"]; ok {
		models := stringSliceValue(incomingConfig["models"])
		if len(models) == 0 {
			delete(merged, "models")
		} else {
			merged["models"] = models
		}
	}

	mergeOpenAICompatibleAPIFormat(merged, incomingConfig)

	return merged
}

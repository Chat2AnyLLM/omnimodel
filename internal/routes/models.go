package routes

import (
	"omnillm/internal/database"
	"omnillm/internal/registry"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"
)

func SetupModelRoutes(router *gin.RouterGroup) {
	router.GET("/models", handleModels)
}

func handleModels(c *gin.Context) {
	providerRegistry := registry.GetProviderRegistry()

	activeProviders := providerRegistry.GetActiveProviders()
	allModels := make([]map[string]interface{}, 0)

	if len(activeProviders) == 0 {
		log.Debug().Msg("No active providers available for models list")
		c.JSON(200, map[string]interface{}{
			"object":   "list",
			"data":     allModels,
			"has_more": false,
		})
		return
	}

	seen := make(map[string]struct{})

	for _, provider := range activeProviders {
		modelsResponse, err := loadProviderModels(provider, false)
		if err != nil {
			log.Warn().
				Str("provider", provider.GetInstanceID()).
				Err(err).
				Msg("Failed to get models from provider")
			continue
		}

		// Convert to OpenAI models format
		for _, model := range modelsResponse {
			if !model.Enabled {
				continue
			}

			// Deduplicate: skip model IDs already seen from another provider
			if _, exists := seen[model.ID]; exists {
				continue
			}
			seen[model.ID] = struct{}{}

			openaiModel := map[string]interface{}{
				"id":        model.ID,
				"object":    "model",
				"created":   time.Now().Unix(),
				"owned_by":  provider.GetInstanceID(),
				"api_shape": "openai",
			}

			// Add optional fields if available
			if model.MaxTokens > 0 {
				openaiModel["max_tokens"] = model.MaxTokens
			}
			if model.Description != "" {
				openaiModel["description"] = model.Description
			}
			if model.Capabilities != nil {
				openaiModel["capabilities"] = model.Capabilities
			}
			if model.Name != "" {
				openaiModel["display_name"] = model.Name
			}

			allModels = append(allModels, openaiModel)
		}
	}

	response := map[string]interface{}{
		"object":   "list",
		"data":     allModels,
		"has_more": false,
	}

	// Append enabled virtual models
	virtualmodelStore := database.NewVirtualModelStore()
	if virtualmodels, err := virtualmodelStore.GetAll(); err == nil {
		for _, vm := range virtualmodels {
			if !vm.Enabled {
				continue
			}
			entry := map[string]interface{}{
				"id":           vm.VirtualModelID,
				"object":       "model",
				"created":      time.Now().Unix(),
				"owned_by":     "virtual",
				"display_name": vm.Name,
				"api_shape":    vm.APIShape,
			}
			if vm.Description != "" {
				entry["description"] = vm.Description
			}
			allModels = append(allModels, entry)
		}
		response["data"] = allModels
	}

	log.Debug().Int("model_count", len(allModels)).Msg("Returning models list")
	c.JSON(200, response)
}

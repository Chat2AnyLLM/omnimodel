package routes

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"strings"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"

	"omnillm/internal/database"
	alibabapkg "omnillm/internal/providers/alibaba"
	azurepkg "omnillm/internal/providers/azure"
	codexpkg "omnillm/internal/providers/codex"
	copilot "omnillm/internal/providers/copilot"
	kimipkg "omnillm/internal/providers/kimi"
	modelscopepkg "omnillm/internal/providers/modelscope"
	openaicompatprovider "omnillm/internal/providers/openaicompatprovider"
	"omnillm/internal/providers/types"
	"omnillm/internal/registry"
	ghservice "omnillm/internal/services/github"
)

// Provider endpoints

func handleGetProviders(c *gin.Context) {
	providerRegistry := registry.GetProviderRegistry()
	providers := providerRegistry.ListProviders()

	providerList := make([]map[string]interface{}, 0, len(providers))
	instanceStore := database.NewProviderInstanceStore()
	for _, provider := range providers {
		config, configErr := loadProviderConfig(provider.GetInstanceID())
		if configErr != nil {
			log.Warn().Err(configErr).Str("provider", provider.GetInstanceID()).Msg("Failed to load provider config")
		}

		authStatus := "unauthenticated"
		if provider.GetToken() != "" {
			authStatus = "authenticated"
		}

		name := provider.GetName()
		subtitle := ""
		if dbRecord, dbErr := instanceStore.Get(provider.GetInstanceID()); dbErr == nil && dbRecord != nil {
			if dbRecord.Name != "" {
				name = dbRecord.Name
			}
			subtitle = dbRecord.Subtitle
		}

		providerInfo := map[string]interface{}{
			"id":         provider.GetInstanceID(),
			"type":       provider.GetID(),
			"name":       name,
			"subtitle":   subtitle,
			"isActive":   providerRegistry.IsActiveProvider(provider.GetInstanceID()),
			"authStatus": authStatus,
		}

		if normalizedConfig := normalizeProviderConfigForFrontend(provider.GetID(), config); normalizedConfig != nil {
			providerInfo["config"] = normalizedConfig
		}

		if authStatus == "authenticated" {
			models, err := loadProviderModels(provider, false)
			if err != nil {
				log.Warn().Err(err).Str("provider", provider.GetInstanceID()).Msg("Failed to load provider models")
				providerInfo["totalModelCount"] = 0
				providerInfo["enabledModelCount"] = 0
			} else {
				providerInfo["totalModelCount"] = len(models)
				providerInfo["enabledModelCount"] = countEnabledModels(models)
			}
		} else {
			providerInfo["totalModelCount"] = 0
			providerInfo["enabledModelCount"] = 0
		}

		providerList = append(providerList, providerInfo)
	}

	c.JSON(http.StatusOK, providerList)
}

func handleSwitchProvider(c *gin.Context) {
	var req map[string]interface{}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	providerID, _ := firstStringValue(req, "providerId", "provider_id")
	if providerID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "providerId is required"})
		return
	}

	providerRegistry := registry.GetProviderRegistry()
	provider, err := providerRegistry.SetActive(providerID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"message": fmt.Sprintf("Provider '%s' not found", providerID),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":  true,
		"message":  fmt.Sprintf("Switched to provider %s", provider.GetInstanceID()),
		"provider": provider.GetInstanceID(),
	})
}

// handleAuthAndCreateProvider authenticates a new provider instance *before*
// persisting it to the database.  The provider is only saved when auth succeeds,
// eliminating the temporary placeholder record and the post-auth rename.
//
// POST /api/admin/providers/auth-and-create/:type
// Body: same as handleProviderAuth (method, apiKey, region, token, …)
func handleAuthAndCreateProvider(c *gin.Context) {
	providerType := c.Param("type")

	var req types.AuthOptions
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	providerRegistry := registry.GetProviderRegistry()

	// Cancel any existing active auth flow before starting a new one to prevent
	// concurrent goroutines from corrupting the singleton activeAuthFlow state.
	activeAuthFlowMu.Lock()
	if activeAuthFlow != nil {
		if activeAuthFlow.cancelFn != nil {
			activeAuthFlow.cancelFn()
		}
		log.Info().Str("provider", activeAuthFlow.ProviderID).Msg("Auth-and-create: cancelled previous auth flow")
		activeAuthFlow = nil
	}
	activeAuthFlowMu.Unlock()

	switch providerType {

	// ——————————————————————————————————————————————————————————————
	case "alibaba":
		if req.Method == "oauth" {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"message": "OAuth authentication is not supported for Alibaba DashScope — please use API key authentication",
			})
			return
		}

		// API-key path — build provider with a canonical ID derived from the
		// API key fields before persisting it.
		suffix := req.APIKey
		if len(suffix) > 6 {
			suffix = suffix[len(suffix)-6:]
		}

		plan := strings.ToLower(strings.TrimSpace(req.Plan))
		switch plan {
		case "", "standard":
			plan = "standard"
		case "coding", "coding_plan", "coding-plan":
			plan = "coding-plan"
		default:
			plan = "standard"
		}
		region := req.Region
		if region == "" {
			region = "global"
		}
		planSlug := strings.ReplaceAll(plan, "-plan", "")
		canonicalID := "alibaba-" + planSlug + "-" + region + "-" + suffix

		prov := alibabapkg.NewProvider(canonicalID, "")
		if err := prov.SetupAuth(&req); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"message": fmt.Sprintf("Authentication failed: %v", err),
			})
			return
		}

		if err := providerRegistry.Register(prov, true); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"message": fmt.Sprintf("Failed to register provider: %v", err),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"provider": gin.H{
				"id":         prov.GetInstanceID(),
				"type":       prov.GetID(),
				"name":       prov.GetName(),
				"isActive":   false,
				"authStatus": "authenticated",
			},
		})

	// ——————————————————————————————————————————————————————————————
	case "alibaba-modelscope":
		if req.APIKey == "" {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"message": "API key is required for ModelScope authentication",
			})
			return
		}

		suffix := req.APIKey
		if len(suffix) > 6 {
			suffix = suffix[len(suffix)-6:]
		}
		canonicalID := "modelscope-" + suffix

		prov := modelscopepkg.NewProvider(canonicalID, "")
		if err := prov.SetupAuth(&req); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"message": fmt.Sprintf("Authentication failed: %v", err),
			})
			return
		}

		if err := providerRegistry.Register(prov, true); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"message": fmt.Sprintf("Failed to register provider: %v", err),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"provider": gin.H{
				"id":         prov.GetInstanceID(),
				"type":       prov.GetID(),
				"name":       prov.GetName(),
				"isActive":   false,
				"authStatus": "authenticated",
			},
		})

	// ——————————————————————————————————————————————————————————————
	case "github-copilot":
		cop := copilot.NewGitHubCopilotProvider("copilot-tmp", "")

		if req.Method == "oauth" || (req.GithubToken == "" && req.Token == "") {
			deviceCode, err := cop.InitiateDeviceCodeFlow()
			if err != nil {
				c.JSON(http.StatusInternalServerError, gin.H{
					"success": false,
					"message": fmt.Sprintf("Failed to initiate OAuth: %v", err),
				})
				return
			}

			const pendingID = "copilot-pending"
			copilotCtx, copilotCancel := context.WithCancel(context.Background())
			activeAuthFlowMu.Lock()
			activeAuthFlow = &authFlowState{
				ProviderID:     pendingID,
				Status:         "awaiting_user",
				InstructionURL: deviceCode.VerificationURI,
				UserCode:       deviceCode.UserCode,
				deviceCode:     deviceCode.DeviceCode,
				cancelFn:       copilotCancel,
			}
			activeAuthFlowMu.Unlock()

			dc := deviceCode
			go func() {
				defer copilotCancel()
				if err := cop.PollAndCompleteDeviceCodeFlow(dc); err != nil {
					if copilotCtx.Err() != nil {
						return
					}
					activeAuthFlowMu.Lock()
					if activeAuthFlow != nil && activeAuthFlow.ProviderID == pendingID {
						activeAuthFlow.Status = "error"
						activeAuthFlow.Error = err.Error()
					}
					activeAuthFlowMu.Unlock()
					log.Error().Err(err).Str("type", "github-copilot").Msg("Auth-and-create: OAuth failed")
					return
				}

				// Derive canonical ID from the provider name which now contains the username
				// e.g., "GitHub Copilot (octocat)" → "github-copilot-octocat"
				canonicalID := deriveGitHubCopilotID(cop.GetName())
				cop.SetInstanceID(canonicalID)

				if err := cop.SaveToDB(); err != nil {
					log.Error().Err(err).Str("provider", canonicalID).Msg("Auth-and-create: failed to save GitHub Copilot token")
					activeAuthFlowMu.Lock()
					if activeAuthFlow != nil && activeAuthFlow.ProviderID == pendingID {
						activeAuthFlow.Status = "error"
						activeAuthFlow.Error = "Failed to save token"
					}
					activeAuthFlowMu.Unlock()
					return
				}

				if err := providerRegistry.Register(cop, true); err != nil {
					log.Warn().Err(err).Str("provider", canonicalID).Msg("Auth-and-create: failed to register GitHub Copilot provider")
				}

				// Update status and provider ID atomically.
				activeAuthFlowMu.Lock()
				if activeAuthFlow != nil && activeAuthFlow.ProviderID == pendingID {
					activeAuthFlow.ProviderID = canonicalID
					activeAuthFlow.Status = "complete"
				}
				activeAuthFlowMu.Unlock()

				log.Info().Str("provider", canonicalID).Msg("Auth-and-create: GitHub Copilot OAuth completed")
			}()

			c.JSON(http.StatusOK, gin.H{
				"success":          false,
				"requiresAuth":     true,
				"pending_id":       pendingID,
				"user_code":        deviceCode.UserCode,
				"verification_uri": deviceCode.VerificationURI,
				"message":          fmt.Sprintf("Visit %s and enter code: %s", deviceCode.VerificationURI, deviceCode.UserCode),
			})
			return
		}

		// Direct token path.
		token := req.Token
		if token == "" {
			token = req.GithubToken
		}
		req.GithubToken = token
		if err := cop.SetupAuth(&req); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"message": fmt.Sprintf("Authentication failed: %v", err),
			})
			return
		}

		// Derive canonical ID from the provider name which now contains the username
		// e.g., "GitHub Copilot (octocat)" → "github-copilot-octocat"
		canonicalID := deriveGitHubCopilotID(cop.GetName())
		cop.SetInstanceID(canonicalID)

		if err := cop.SaveToDB(); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"message": fmt.Sprintf("Failed to save provider credentials: %v", err),
			})
			return
		}

		if err := providerRegistry.Register(cop, true); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"message": fmt.Sprintf("Failed to register provider: %v", err),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"provider": gin.H{
				"id":         cop.GetInstanceID(),
				"type":       cop.GetID(),
				"name":       cop.GetName(),
				"isActive":   false,
				"authStatus": "authenticated",
			},
		})

	// ——————————————————————————————————————————————————————————————
	case "codex":
		if strings.TrimSpace(req.APIKey) == "" {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"message": "apiKey is required for Codex authentication",
			})
			return
		}

		canonicalID := providerRegistry.NextInstanceID("codex")
		cdx := codexpkg.NewCodexProvider(canonicalID)
		if err := cdx.SetupAuth(&req); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"message": fmt.Sprintf("Codex authentication failed: %v", err),
			})
			return
		}

		if err := cdx.SaveToDB(); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"message": fmt.Sprintf("Failed to save Codex credentials: %v", err),
			})
			return
		}

		if err := providerRegistry.Register(cdx, true); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"message": fmt.Sprintf("Failed to register Codex provider: %v", err),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"provider": gin.H{
				"id":         cdx.GetInstanceID(),
				"type":       cdx.GetID(),
				"name":       cdx.GetName(),
				"isActive":   false,
				"authStatus": "authenticated",
			},
		})

	// ——————————————————————————————————————————————————————————————
	case "antigravity":
		// The auth-and-create path is no longer used for Antigravity;
		// the frontend calls POST /providers/antigravity/start-oauth directly.
		// Return a clear error so any stale callers know which endpoint to use.
		c.JSON(http.StatusBadRequest, gin.H{
			"success": false,
			"message": "Use POST /api/admin/providers/antigravity/start-oauth to begin Google OAuth",
		})

	// ——————————————————————————————————————————————————————————————
	case "azure-openai":
		instanceID := providerRegistry.NextInstanceID(providerType)
		prov := azurepkg.NewProvider(instanceID, "")

		if err := prov.SetupAuth(&req); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"message": fmt.Sprintf("Authentication failed: %v", err),
			})
			return
		}

		if rawCfg, cfgErr := loadProviderConfig(prov.GetInstanceID()); cfgErr == nil && rawCfg != nil {
			normCfg := normalizeProviderConfigForStorage(providerType, rawCfg)
			if len(normCfg) > 0 {
				_ = database.NewProviderConfigStore().Save(prov.GetInstanceID(), normCfg)
			}
		}

		if err := providerRegistry.Register(prov, true); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"message": fmt.Sprintf("Failed to register provider: %v", err),
			})
			return
		}

		if req.Deployments != "" {
			var rawDeployments []interface{}
			if jsonErr := json.Unmarshal([]byte(req.Deployments), &rawDeployments); jsonErr == nil {
				modelStateStore := database.NewModelStateStore()
				for _, item := range rawDeployments {
					switch typed := item.(type) {
					case string:
						deployment := strings.TrimSpace(typed)
						if deployment != "" {
							_ = modelStateStore.SetEnabled(prov.GetInstanceID(), deployment, true)
						}
					case map[string]interface{}:
						model, _ := typed["model"].(string)
						deployment, _ := typed["deployment"].(string)
						model = strings.TrimSpace(model)
						deployment = strings.TrimSpace(deployment)
						if model == "" {
							model = deployment
						}
						if deployment == "" {
							deployment = model
						}
						if model != "" {
							_ = modelStateStore.SetEnabled(prov.GetInstanceID(), model, true)
						}
						if deployment != "" && deployment != model {
							_ = modelStateStore.SetEnabled(prov.GetInstanceID(), deployment, true)
						}
					}
				}
			}
		}

		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"provider": gin.H{
				"id":         prov.GetInstanceID(),
				"type":       prov.GetID(),
				"name":       prov.GetName(),
				"isActive":   false,
				"authStatus": "authenticated",
			},
		})

	// ——————————————————————————————————————————————————————————————
	case "google":
		instanceID := providerRegistry.NextInstanceID(providerType)
		prov := kimipkg.NewProvider(instanceID, "")

		if err := prov.SetupAuth(&req); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"message": fmt.Sprintf("Authentication failed: %v", err),
			})
			return
		}

		if err := providerRegistry.Register(prov, true); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"message": fmt.Sprintf("Failed to register provider: %v", err),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"provider": gin.H{
				"id":         prov.GetInstanceID(),
				"type":       prov.GetID(),
				"name":       prov.GetName(),
				"isActive":   false,
				"authStatus": "authenticated",
			},
		})

	// ——————————————————————————————————————————————————————————————
	case "openai-compatible":
		instanceID := openaicompatprovider.CanonicalInstanceID(req.Endpoint, req.APIKey)
		prov := openaicompatprovider.NewProvider(instanceID, "")

		if err := prov.SetupAuth(&req); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"message": fmt.Sprintf("Authentication failed: %v", err),
			})
			return
		}

		if err := providerRegistry.Register(prov, true); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"message": fmt.Sprintf("Failed to register provider: %v", err),
			})
			return
		}

		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"provider": gin.H{
				"id":         prov.GetInstanceID(),
				"type":       prov.GetID(),
				"name":       prov.GetName(),
				"isActive":   false,
				"authStatus": "authenticated",
			},
		})

	default:
		c.JSON(http.StatusBadRequest, gin.H{
			"error": fmt.Sprintf("Unknown provider type '%s'", providerType),
		})
	}
}

// deriveGitHubCopilotID extracts the GitHub username from the provider name
// and generates a consistent instance ID.
// Example: "GitHub Copilot (octocat)" → "github-copilot-octocat"
func deriveGitHubCopilotID(name string) string {
	// Look for pattern "GitHub Copilot (username)"
	start := strings.Index(name, "(")
	end := strings.Index(name, ")")
	if start == -1 || end == -1 || end <= start+1 {
		// Fallback if we can't extract username
		providerRegistry := registry.GetProviderRegistry()
		return providerRegistry.NextInstanceID("github-copilot")
	}

	username := strings.TrimSpace(name[start+1 : end])
	if username == "" {
		providerRegistry := registry.GetProviderRegistry()
		return providerRegistry.NextInstanceID("github-copilot")
	}

	// Sanitize username: lowercase and replace non-alphanumeric with dashes
	sanitized := strings.Map(func(r rune) rune {
		if (r >= 'a' && r <= 'z') || (r >= '0' && r <= '9') || r == '-' || r == '_' {
			return r
		}
		return '-'
	}, strings.ToLower(username))

	// Remove leading/trailing dashes
	sanitized = strings.Trim(sanitized, "-_")
	if sanitized == "" {
		providerRegistry := registry.GetProviderRegistry()
		return providerRegistry.NextInstanceID("github-copilot")
	}

	return "github-copilot-" + sanitized
}

func handleDeleteProvider(c *gin.Context) {
	providerID := c.Param("id")

	providerRegistry := registry.GetProviderRegistry()
	if err := providerRegistry.Remove(providerID); err != nil {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"message": err.Error(),
		})
		return
	}

	modelStateStore := database.NewModelStateStore()
	if states, err := modelStateStore.GetAllByInstance(providerID); err == nil {
		for _, state := range states {
			_ = modelStateStore.Delete(providerID, state.ModelID)
		}
	}
	modelConfigStore := database.NewModelConfigStore()
	if configs, err := modelConfigStore.GetAllByInstance(providerID); err == nil {
		for _, config := range configs {
			_ = modelConfigStore.Delete(providerID, config.ModelID)
		}
	}
	_ = database.NewProviderConfigStore().Delete(providerID)
	_ = database.NewProviderInstanceStore().Delete(providerID)

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": fmt.Sprintf("Provider '%s' deleted", providerID),
	})
}

func handleGetProviderPriorities(c *gin.Context) {
	instanceStore := database.NewProviderInstanceStore()
	instances, err := instanceStore.GetAll()
	if err != nil {
		log.Error().Err(err).Msg("Failed to get provider priorities from database")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve provider priorities"})
		return
	}

	priorities := make(map[string]int)
	for _, instance := range instances {
		priorities[instance.InstanceID] = instance.Priority
	}

	c.JSON(http.StatusOK, gin.H{"priorities": priorities})
}

func handleSetProviderPriorities(c *gin.Context) {
	var req struct {
		Priorities map[string]int `json:"priorities"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	instanceStore := database.NewProviderInstanceStore()
	for id, priority := range req.Priorities {
		record, err := instanceStore.Get(id)
		if err != nil || record == nil {
			continue
		}
		record.Priority = priority
		instanceStore.Save(record)
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Provider priorities updated",
	})
}

func handleListProviderModels(c *gin.Context) {
	providerID := c.Param("id")

	providerRegistry := registry.GetProviderRegistry()
	provider, err := providerRegistry.GetProvider(providerID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	models, err := loadProviderModels(provider, false)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"models": models,
		"total":  len(models),
	})
}

func handleRefreshProviderModels(c *gin.Context) {
	providerID := c.Param("id")

	providerRegistry := registry.GetProviderRegistry()
	provider, err := providerRegistry.GetProvider(providerID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	// Force refresh: bypass cache and call external API
	models, err := loadProviderModels(provider, true)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": err.Error()})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"models": models,
		"total":  len(models),
		"cached": false,
	})
}

func handleToggleProviderModel(c *gin.Context) {
	providerID := c.Param("id")
	var req map[string]interface{}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	modelID, _ := firstStringValue(req, "modelId", "model_id")
	if modelID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "modelId is required"})
		return
	}

	enabled, ok := req["enabled"].(bool)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{"error": "enabled must be a boolean"})
		return
	}

	modelStateStore := database.NewModelStateStore()
	if err := modelStateStore.SetEnabled(providerID, modelID, enabled); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to persist model state"})
		return
	}

	log.Info().
		Str("provider", providerID).
		Str("model", modelID).
		Bool("enabled", enabled).
		Msg("Model toggle requested")

	c.JSON(http.StatusOK, gin.H{
		"success":  true,
		"modelId":  modelID,
		"model_id": modelID,
		"enabled":  enabled,
	})
}

func handleGetProviderUsage(c *gin.Context) {
	providerID := c.Param("id")

	providerRegistry := registry.GetProviderRegistry()
	provider, err := providerRegistry.GetProvider(providerID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	usage, err := provider.GetUsage()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get usage data"})
		return
	}

	c.JSON(http.StatusOK, usage)
}

func handleProviderAuth(c *gin.Context) {
	providerID := c.Param("id")

	var req types.AuthOptions
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	providerRegistry := registry.GetProviderRegistry()
	provider, err := providerRegistry.GetProvider(providerID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	cop, isCopilot := provider.(*copilot.GitHubCopilotProvider)

	// Codex: API-key auth only.
	if _, isCodex := provider.(*codexpkg.CodexProvider); isCodex {
		if strings.TrimSpace(req.APIKey) == "" {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"message": "apiKey is required for Codex authentication",
			})
			return
		}
	}

	// Alibaba: API-key only — OAuth is not supported.
	if aliProv, ok := provider.(*alibabapkg.Provider); ok {
		if req.Method == "oauth" {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"message": "OAuth authentication is not supported for Alibaba DashScope — please use API key authentication",
			})
			return
		}
		if err := aliProv.SetupAuth(&req); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": fmt.Sprintf("Authentication failed: %v", err)})
			return
		}
		if err := providerRegistry.Register(aliProv, true); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": fmt.Sprintf("Failed to update provider: %v", err)})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"provider": gin.H{
				"id":         aliProv.GetInstanceID(),
				"type":       aliProv.GetID(),
				"name":       aliProv.GetName(),
				"authStatus": "authenticated",
			},
		})
		return
	}

	// ModelScope: API-key only.
	if msProv, ok := provider.(*modelscopepkg.Provider); ok {
		if req.APIKey == "" {
			c.JSON(http.StatusBadRequest, gin.H{
				"success": false,
				"message": "API key is required for ModelScope authentication",
			})
			return
		}
		if err := msProv.SetupAuth(&req); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": fmt.Sprintf("Authentication failed: %v", err)})
			return
		}
		if err := providerRegistry.Register(msProv, true); err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{"success": false, "message": fmt.Sprintf("Failed to update provider: %v", err)})
			return
		}
		c.JSON(http.StatusOK, gin.H{
			"success": true,
			"provider": gin.H{
				"id":         msProv.GetInstanceID(),
				"type":       msProv.GetID(),
				"name":       msProv.GetName(),
				"authStatus": "authenticated",
			},
		})
		return
	}

	// Handle OAuth device code flow for GitHub Copilot
	if isCopilot && (req.Method == "oauth" || (req.GithubToken == "" && req.Token == "")) {
		// Start device code flow in a goroutine, return immediately with requiresAuth
		deviceCode, err := cop.InitiateDeviceCodeFlow()
		if err != nil {
			c.JSON(http.StatusInternalServerError, gin.H{
				"success": false,
				"message": fmt.Sprintf("Failed to initiate OAuth: %v", err),
			})
			return
		}

		// Set auth flow state to awaiting_user
		copilotCtx, copilotCancel := context.WithCancel(context.Background())
		activeAuthFlowMu.Lock()
		activeAuthFlow = &authFlowState{
			ProviderID:     providerID,
			Status:         "awaiting_user",
			InstructionURL: deviceCode.VerificationURI,
			UserCode:       deviceCode.UserCode,
			deviceCode:     deviceCode.DeviceCode,
			cancelFn:       copilotCancel,
		}
		activeAuthFlowMu.Unlock()

		// Start polling for the access token in a goroutine
		dc := deviceCode
		prov := cop
		reg := providerRegistry
		go func() {
			defer copilotCancel()
			if err := prov.PollAndCompleteDeviceCodeFlow(dc); err != nil {
				// Ignore if cancelled
				if copilotCtx.Err() != nil {
					return
				}
				activeAuthFlowMu.Lock()
				if activeAuthFlow != nil && activeAuthFlow.ProviderID == providerID {
					activeAuthFlow.Status = "error"
					activeAuthFlow.Error = err.Error()
				}
				activeAuthFlowMu.Unlock()
				log.Error().Err(err).Str("provider", providerID).Msg("OAuth device code flow failed")
				return
			}

			// Save token to database
			if err := prov.SaveToDB(); err != nil {
				log.Error().Err(err).Str("provider", providerID).Msg("Failed to save token")
			}

			// Persist updated provider name to DB
			if err := reg.Register(prov, true); err != nil {
				log.Warn().Err(err).Str("provider", providerID).Msg("Failed to update provider in registry")
			}

			// Mark complete
			activeAuthFlowMu.Lock()
			if activeAuthFlow != nil && activeAuthFlow.ProviderID == providerID {
				activeAuthFlow.Status = "complete"
			}
			activeAuthFlowMu.Unlock()

			log.Info().Str("provider", providerID).Msg("GitHub Copilot OAuth completed")
		}()

		c.JSON(http.StatusOK, gin.H{
			"success":          false,
			"requiresAuth":     true,
			"user_code":        deviceCode.UserCode,
			"verification_uri": deviceCode.VerificationURI,
			"message":          fmt.Sprintf("Visit %s and enter code: %s", deviceCode.VerificationURI, deviceCode.UserCode),
		})
		return
	}

	// Direct token auth for GitHub Copilot
	if isCopilot && (req.Token != "" || req.GithubToken != "") {
		token := req.Token
		if token == "" {
			token = req.GithubToken
		}
		req.GithubToken = token
	}

	if err := provider.SetupAuth(&req); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": fmt.Sprintf("Authentication failed: %v", err),
		})
		return
	}

	// Save token to database if provider supports it
	if isCopilot {
		if err := cop.SaveToDB(); err != nil {
			log.Error().Err(err).Str("provider", providerID).Msg("Failed to save token to database")
		}
	}

	if provider.GetID() == "azure-openai" {
		if rawCfg, cfgErr := loadProviderConfig(providerID); cfgErr == nil && rawCfg != nil {
			normCfg := normalizeProviderConfigForStorage(provider.GetID(), rawCfg)
			if len(normCfg) > 0 {
				_ = database.NewProviderConfigStore().Save(providerID, normCfg)
			}
		}
	}

	// Invalidate model cache on re-auth
	cacheStore := database.NewProviderModelsCacheStore()
	if err := cacheStore.Delete(providerID); err != nil {
		log.Warn().Err(err).Str("provider", providerID).Msg("Failed to invalidate model cache")
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Provider authenticated successfully",
	})
}

func handleInitiateDeviceCode(c *gin.Context) {
	providerID := c.Param("id")

	providerRegistry := registry.GetProviderRegistry()
	provider, err := providerRegistry.GetProvider(providerID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	// ——————————————————————————————————————————————————————————————
	cop, ok := provider.(*copilot.GitHubCopilotProvider)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Provider does not support device code OAuth flow",
		})
		return
	}

	deviceCode, err := cop.InitiateDeviceCodeFlow()
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   fmt.Sprintf("Failed to initiate device code flow: %v", err),
		})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":          true,
		"user_code":        deviceCode.UserCode,
		"device_code":      deviceCode.DeviceCode,
		"verification_uri": deviceCode.VerificationURI,
		"expires_in":       deviceCode.ExpiresIn,
		"interval":         deviceCode.Interval,
		"message":          fmt.Sprintf("Please visit %s and enter code: %s", deviceCode.VerificationURI, deviceCode.UserCode),
	})
}

func handleCompleteDeviceCode(c *gin.Context) {
	providerID := c.Param("id")

	var req struct {
		DeviceCode   string `json:"device_code"`
		UserCode     string `json:"user_code"`
		CodeVerifier string `json:"code_verifier"` // Alibaba PKCE; optional for GitHub
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body - requires device_code and user_code"})
		return
	}

	providerRegistry := registry.GetProviderRegistry()
	provider, err := providerRegistry.GetProvider(providerID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	// ——————————————————————————————————————————————————————————————
	cop, ok := provider.(*copilot.GitHubCopilotProvider)
	if !ok {
		c.JSON(http.StatusBadRequest, gin.H{
			"error": "Provider does not support device code OAuth flow",
		})
		return
	}

	// Reconstruct the device code response for polling
	deviceCodeResp := &ghservice.DeviceCodeResponse{
		DeviceCode:      req.DeviceCode,
		UserCode:        req.UserCode,
		VerificationURI: "",  // Not needed for polling
		ExpiresIn:       600, // Default 10 minutes
		Interval:        5,   // Default 5 second poll interval
	}

	// Poll for access token
	if err := cop.PollAndCompleteDeviceCodeFlow(deviceCodeResp); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"error":   fmt.Sprintf("Failed to complete OAuth: %v", err),
		})
		return
	}

	// Save tokens to database
	if err := cop.SaveToDB(); err != nil {
		log.Error().Err(err).Str("provider", providerID).Msg("Failed to save tokens to database")
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "GitHub Copilot authenticated successfully",
		"provider": gin.H{
			"id":   cop.GetInstanceID(),
			"name": cop.GetName(),
			"type": "github-copilot",
		},
	})
}

func handleUpdateProviderConfig(c *gin.Context) {
	providerID := c.Param("id")

	providerRegistry := registry.GetProviderRegistry()
	provider, err := providerRegistry.GetProvider(providerID)
	if err != nil {
		c.JSON(http.StatusNotFound, gin.H{"error": err.Error()})
		return
	}

	var config map[string]interface{}
	if err := c.ShouldBindJSON(&config); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	previousConfig, _ := loadProviderConfig(providerID)
	normalizedConfig := normalizeProviderConfigForStorage(provider.GetID(), config)
	if len(normalizedConfig) == 0 {
		c.JSON(http.StatusBadRequest, gin.H{"error": "No valid configuration fields supplied"})
		return
	}
	if provider.GetID() == "openai-compatible" {
		normalizedConfig = mergeOpenAICompatibleConfig(previousConfig, config, normalizedConfig)
	}

	configStore := database.NewProviderConfigStore()
	if err := configStore.Save(providerID, normalizedConfig); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to persist provider config"})
		return
	}

	if provider.GetID() == "azure-openai" {
		oldModels := azurepkg.ModelIDs(previousConfig)
		newModels := azurepkg.ModelIDs(normalizedConfig)
		if len(oldModels) > 0 {
			newSet := make(map[string]struct{}, len(newModels))
			for _, model := range newModels {
				newSet[model] = struct{}{}
			}

			modelStateStore := database.NewModelStateStore()
			modelConfigStore := database.NewModelConfigStore()
			for _, model := range oldModels {
				if _, keep := newSet[model]; keep {
					continue
				}
				_ = modelStateStore.Delete(providerID, model)
				_ = modelConfigStore.Delete(providerID, model)
			}
		}
	}

	log.Info().
		Str("provider", providerID).
		Msg("Provider config update requested")

	// Invalidate model cache when config changes (may affect available models)
	modelsCacheStore := database.NewProviderModelsCacheStore()
	if err := modelsCacheStore.Delete(providerID); err != nil {
		log.Warn().Err(err).Str("provider", providerID).Msg("Failed to invalidate model cache")
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": fmt.Sprintf("Configuration updated for %s", providerID),
		"config":  normalizeProviderConfigForFrontend(provider.GetID(), normalizedConfig),
	})
}

func handleRenameProvider(c *gin.Context) {
	providerID := c.Param("id")

	var req struct {
		Name     *string `json:"name"`
		Subtitle *string `json:"subtitle"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}
	if req.Name == nil && req.Subtitle == nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "At least one of 'name' or 'subtitle' must be provided"})
		return
	}

	instanceStore := database.NewProviderInstanceStore()
	record, err := instanceStore.Get(providerID)
	if err != nil || record == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Provider instance not found in database"})
		return
	}

	var newName *string
	if req.Name != nil {
		trimmed := strings.TrimSpace(*req.Name)
		if trimmed == "" {
			c.JSON(http.StatusBadRequest, gin.H{"error": "name cannot be empty"})
			return
		}
		newName = &trimmed
		record.Name = trimmed
		providerRegistry := registry.GetProviderRegistry()
		if provider, provErr := providerRegistry.GetProvider(providerID); provErr == nil {
			provider.SetName(trimmed)
		}
		log.Info().Str("provider", providerID).Str("name", trimmed).Msg("Provider renamed")
	}

	var newSubtitle *string
	if req.Subtitle != nil {
		trimmed := strings.TrimSpace(*req.Subtitle)
		newSubtitle = &trimmed
		record.Subtitle = trimmed
		log.Info().Str("provider", providerID).Str("subtitle", trimmed).Msg("Provider subtitle updated")
	}

	if err := instanceStore.UpdateMetadata(providerID, newName, newSubtitle); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to persist provider metadata"})
		return
	}

	c.JSON(http.StatusOK, gin.H{"success": true, "name": record.Name, "subtitle": record.Subtitle})
}

func handleActivateProvider(c *gin.Context) {
	providerID := c.Param("id")

	providerRegistry := registry.GetProviderRegistry()
	if !providerRegistry.IsRegistered(providerID) {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"message": "Provider " + providerID + " not found",
		})
		return
	}

	provider, err := providerRegistry.AddActive(providerID)
	if err != nil {
		log.Error().Err(err).Str("provider", providerID).Msg("Failed to activate provider")
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to activate provider: " + err.Error(),
		})
		return
	}

	log.Info().Str("provider", providerID).Msg("Provider activated")
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Provider " + providerID + " activated",
		"provider": gin.H{
			"id":   provider.GetInstanceID(),
			"name": provider.GetName(),
		},
	})
}

func handleDeactivateProvider(c *gin.Context) {
	providerID := c.Param("id")

	providerRegistry := registry.GetProviderRegistry()
	if !providerRegistry.IsRegistered(providerID) {
		c.JSON(http.StatusNotFound, gin.H{
			"success": false,
			"message": "Provider " + providerID + " not found",
		})
		return
	}

	if err := providerRegistry.RemoveActive(providerID); err != nil {
		log.Error().Err(err).Str("provider", providerID).Msg("Failed to deactivate provider")
		c.JSON(http.StatusInternalServerError, gin.H{
			"success": false,
			"message": "Failed to deactivate provider: " + err.Error(),
		})
		return
	}

	log.Info().Str("provider", providerID).Msg("Provider deactivated")
	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Provider " + providerID + " deactivated",
	})
}

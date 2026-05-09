package copilot

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"
	"bytes"
	"context"
	"strings"

	"omnillm/internal/ingestion"
	"omnillm/internal/serialization"
	"omnillm/internal/providers/types"
	ghservice "omnillm/internal/services/github"

	"github.com/rs/zerolog/log"
)

func (p *GitHubCopilotProvider) GetHeaders(forVision bool) map[string]string {
	token := p.GetToken()
	headers := map[string]string{
		"Authorization":                       fmt.Sprintf("Bearer %s", token),
		"Content-Type":                        "application/json",
		"Accept":                              "application/json",
		"copilot-integration-id":              "vscode-chat",
		"Editor-Version":                      ghservice.EditorVersion,
		"Editor-Plugin-Version":               ghservice.PluginVersion,
		"User-Agent":                          ghservice.UserAgent,
		"OpenAI-Intent":                       "conversation-panel",
		"X-Github-Api-Version":                ghservice.APIVersion,
		"X-Request-Id":                        generateCopilotRequestID(),
		"X-Vscode-User-Agent-Library-Version": "electron-fetch",
	}

	if forVision {
		headers["copilot-vision-request"] = "true"
	}

	return headers
}

func generateCopilotRequestID() string {
	buf := make([]byte, 16)
	if _, err := rand.Read(buf); err == nil {
		return hex.EncodeToString(buf)
	}

	return fmt.Sprintf("%d", time.Now().UnixNano())
}

func (p *GitHubCopilotProvider) GetModels() (*types.ModelsResponse, error) {
	token := p.GetToken()
	if token == "" {
		return nil, fmt.Errorf("provider not authenticated")
	}

	req, err := http.NewRequest("GET", fmt.Sprintf("%s/models", p.baseURL), nil)
	if err == nil {
		for k, v := range p.GetHeaders(false) {
			req.Header.Set(k, v)
		}

		client := copilotHTTPClient
		resp, err := client.Do(req)
		if err == nil {
			defer resp.Body.Close()

			if resp.StatusCode == http.StatusOK {
				var payload struct {
					Data []struct {
						ID                 string   `json:"id"`
						Name               string   `json:"name"`
						Capabilities       struct {
							Tokenizer string `json:"tokenizer"`
							Limits    struct {
								MaxContextWindowTokens int `json:"max_context_window_tokens"`
								MaxOutputTokens        int `json:"max_output_tokens"`
							} `json:"limits"`
							Supports struct {
								ToolCalls         bool `json:"tool_calls"`
								ParallelToolCalls bool `json:"parallel_tool_calls"`
								Dimensions        bool `json:"dimensions"`
							} `json:"supports"`
						} `json:"capabilities"`
						SupportedEndpoints []string `json:"supported_endpoints"`
					} `json:"data"`
					Object string `json:"object"`
				}

				decodeErr := json.NewDecoder(resp.Body).Decode(&payload)
				if decodeErr == nil && len(payload.Data) > 0 {
					// Build shape cache from supported_endpoints in the Copilot /models response
					cache := make(modelShapeCache, len(payload.Data))
					for _, m := range payload.Data {
						cache[m.ID] = shapeFromEndpoints(m.SupportedEndpoints)
					}
					p.shapeCache = cache
					models := make([]types.Model, 0, len(payload.Data))
					for _, model := range payload.Data {
						capabilities := map[string]interface{}{}
						if model.Capabilities.Tokenizer != "" {
							capabilities["tokenizer"] = model.Capabilities.Tokenizer
						}
						if model.Capabilities.Supports.ToolCalls {
							capabilities["function_calling"] = true
						}
						if model.Capabilities.Supports.ParallelToolCalls {
							capabilities["parallel_tool_calls"] = true
						}
						if model.Capabilities.Supports.Dimensions {
							capabilities["embeddings"] = true
						}

						maxTokens := model.Capabilities.Limits.MaxContextWindowTokens
						if maxTokens == 0 {
							maxTokens = model.Capabilities.Limits.MaxOutputTokens
						}

						models = append(models, types.Model{
							ID:           model.ID,
							Name:         firstNonEmpty(model.Name, model.ID),
							MaxTokens:    maxTokens,
							Provider:     p.instanceID,
							Capabilities: capabilities,
						})
					}

					return &types.ModelsResponse{
						Data:   models,
						Object: firstNonEmpty(payload.Object, "list"),
					}, nil
				}

				log.Warn().Err(decodeErr).Str("provider", p.instanceID).Msg("Failed to decode Copilot models response, falling back to built-in model list")
			} else {
				body, _ := io.ReadAll(resp.Body)
				log.Warn().
					Str("provider", p.instanceID).
					Int("status", resp.StatusCode).
					Str("body", string(body)).
					Msg("Copilot models request failed, falling back to built-in model list")
			}
		} else {
			log.Warn().Err(err).Str("provider", p.instanceID).Msg("Failed to fetch Copilot models, falling back to built-in model list")
		}
	}

	models := []types.Model{
		{
			ID:          "gpt-4o",
			Name:        "GPT-4o",
			Description: "Most capable GPT-4o model with vision",
			MaxTokens:   128000,
			Provider:    p.instanceID,
			Capabilities: map[string]interface{}{
				"tokenizer":        "o200k_base",
				"vision":           true,
				"function_calling": true,
				"streaming":        true,
			},
		},
		// (other built-in models omitted for brevity in this file)
	}

	return &types.ModelsResponse{
		Data:   models,
		Object: "list",
	}, nil
}

func firstNonEmpty(values ...string) string {
	for _, value := range values {
		if value != "" {
			return value
		}
	}

	return ""
}

// Legacy interface methods
func (p *GitHubCopilotProvider) CreateChatCompletions(payload map[string]interface{}) (map[string]interface{}, error) {
	raw, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal payload: %w", err)
	}
	canonicalReq, err := ingestion.ParseOpenAIChatCompletions(raw)
	if err != nil {
		return nil, fmt.Errorf("failed to parse request: %w", err)
	}

	adapter := p.GetAdapter().(*CopilotAdapter)
	response, err := adapter.Execute(context.Background(), canonicalReq)
	if err != nil {
		return nil, err
	}

	openaiResp, err := serialization.SerializeToOpenAI(response)
	if err != nil {
		return nil, err
	}

	respBytes, _ := json.Marshal(openaiResp)
	var result map[string]interface{}
	json.Unmarshal(respBytes, &result)

	return result, nil
}

func (p *GitHubCopilotProvider) CreateEmbeddings(payload map[string]interface{}) (map[string]interface{}, error) {
	if p.token == "" {
		return nil, fmt.Errorf("provider not authenticated")
	}

	// Forward to Copilot embeddings endpoint
	reqBody, err := json.Marshal(payload)
	if err != nil {
		return nil, fmt.Errorf("failed to marshal payload: %w", err)
	}

	url := fmt.Sprintf("%s/embeddings", p.baseURL)
	req, err := http.NewRequest("POST", url, bytes.NewBuffer(reqBody))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	for k, v := range p.GetHeaders(false) {
		req.Header.Set(k, v)
	}

	resp, err := copilotHTTPClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("embeddings request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("embeddings API failed with status %d: %s", resp.StatusCode, string(body))
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode embeddings response: %w", err)
	}

	return result, nil
}

func (p *GitHubCopilotProvider) GetUsage() (map[string]interface{}, error) {
	if p.githubToken == "" {
		return nil, fmt.Errorf("GitHub token not available")
	}
	return ghservice.GetCopilotUsage(p.githubToken)
}

// shapeFromEndpoints returns the preferred copilotAPIShape for a model given
// its supported_endpoints list from the Copilot /models response.
// A model that lists /responses but NOT /chat/completions is responses-only.
// All other models default to chat/completions.
func shapeFromEndpoints(endpoints []string) copilotAPIShape {
	hasResponses := false
	hasChat := false
	for _, ep := range endpoints {
		if strings.HasSuffix(ep, "/responses") {
			hasResponses = true
		}
		if strings.HasSuffix(ep, "/chat/completions") {
			hasChat = true
		}
	}
	if hasResponses && !hasChat {
		return shapeResponses
	}
	return shapeChat
}

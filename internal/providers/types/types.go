// Package types defines interfaces and types for providers
package types

import (
	"context"
	"encoding/json"
	"omnillm/internal/cif"
)

type ProviderID string

// Supported provider identifiers.
const (
	ProviderGitHubCopilot    ProviderID = "github-copilot"
	ProviderAntigravity      ProviderID = "antigravity"
	ProviderAlibaba          ProviderID = "alibaba"
	ProviderAzureOpenAI      ProviderID = "azure-openai"
	ProviderGoogle           ProviderID = "google"
	ProviderKimi             ProviderID = "kimi"
	ProviderOpenAICompatible ProviderID = "openai-compatible"
	ProviderModelScope      ProviderID = "alibaba-modelscope"
)

// AuthOptions is decoded from JSON request bodies.  Fields accept both
// camelCase (frontend convention) and snake_case (legacy) names via a custom
// UnmarshalJSON that normalises keys before decoding.
type AuthOptions struct {
	Method              string `json:"method,omitempty"`
	Force               bool   `json:"force,omitempty"`
	ClientID            string `json:"client_id,omitempty"`
	ClientSecret        string `json:"client_secret,omitempty"`
	GithubToken         string `json:"github_token,omitempty"`
	Token               string `json:"token,omitempty"` // alias for GithubToken from frontend
	APIKey              string `json:"apiKey,omitempty"`
	Region              string `json:"region,omitempty"`
	Plan                string `json:"plan,omitempty"`
	Endpoint            string `json:"endpoint,omitempty"`
	APIFormat           string `json:"apiFormat,omitempty"` // e.g. "anthropic" for Alibaba Anthropic-compatible endpoint
	Models              string `json:"models,omitempty"`    // JSON-encoded []string, used by openai-compatible
	Deployments         string `json:"deployments,omitempty"` // JSON-encoded []string, used by azure-openai
	APIVersion          string `json:"apiVersion,omitempty"`  // e.g. "2024-02-01", used by azure-openai
	AllowLocalEndpoints bool   `json:"allowLocalEndpoints,omitempty"`
	OAuthProvider       string `json:"oauthProvider,omitempty"` // "github" (default) or "google" for Codex
}

// UnmarshalJSON normalises camelCase aliases sent by the frontend before
// decoding into AuthOptions.
func (a *AuthOptions) UnmarshalJSON(data []byte) error {
	// Decode into a raw map first so we can rename keys.
	var raw map[string]interface{}
	if err := json.Unmarshal(data, &raw); err != nil {
		return err
	}
	// camelCase → snake_case / canonical aliases.
	aliases := map[string]string{
		"clientId":     "client_id",
		"clientSecret": "client_secret",
		"githubToken":  "github_token",
	}
	for camel, canonical := range aliases {
		if v, ok := raw[camel]; ok {
			if _, exists := raw[canonical]; !exists {
				raw[canonical] = v
			}
			delete(raw, camel)
		}
	}
	// Re-encode the normalised map and decode into a plain alias type to
	// avoid infinite recursion.
	type plain AuthOptions
	normalised, err := json.Marshal(raw)
	if err != nil {
		return err
	}
	return json.Unmarshal(normalised, (*plain)(a))
}

type Model struct {
	ID           string                 `json:"id"`
	Name         string                 `json:"name"`
	Description  string                 `json:"description,omitempty"`
	Capabilities map[string]interface{} `json:"capabilities,omitempty"`
	MaxTokens    int                    `json:"max_tokens,omitempty"`
	OutputTokens int                    `json:"output_tokens,omitempty"`
	Provider     string                 `json:"provider,omitempty"`
}

type ModelsResponse struct {
	Data   []Model `json:"data"`
	Object string  `json:"object"`
}

type ProviderAdapter interface {
	GetProvider() Provider
	Execute(ctx context.Context, request *cif.CanonicalRequest) (*cif.CanonicalResponse, error)
	ExecuteStream(ctx context.Context, request *cif.CanonicalRequest) (<-chan cif.CIFStreamEvent, error)
	RemapModel(canonicalModel string) string
}

type Provider interface {
	// Identity
	GetID() string         // Provider type (e.g. "antigravity")
	GetInstanceID() string // Unique instance identifier (e.g. "antigravity-1")
	GetName() string
	SetName(name string)

	// Authentication
	SetupAuth(options *AuthOptions) error
	GetToken() string
	RefreshToken() error

	// API Configuration
	GetBaseURL() string
	GetHeaders(forVision bool) map[string]string

	// Models
	GetModels() (*ModelsResponse, error)

	// Legacy Request Methods (to be deprecated)
	CreateChatCompletions(payload map[string]interface{}) (map[string]interface{}, error)
	CreateEmbeddings(payload map[string]interface{}) (map[string]interface{}, error)
	GetUsage() (map[string]interface{}, error)

	// CIF Adapter (optional during migration)
	GetAdapter() ProviderAdapter
}

type ProviderConfig struct {
	Provider string                 `json:"provider"`
	Config   map[string]interface{} `json:"config,omitempty"`
}

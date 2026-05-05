package google

import (
	"context"
	"encoding/json"
	"fmt"
	"omnillm/internal/cif"
	"omnillm/internal/database"
	"omnillm/internal/providers/types"
	"strings"
	"sync"

	"github.com/rs/zerolog/log"
)

type Provider struct {
	instanceID string
	name       string
	token      string
	baseURL    string
	config     map[string]interface{}
	configOnce sync.Once
}

type Adapter struct {
	provider *Provider
}

func NewProvider(instanceID, name string) *Provider {
	displayName := strings.TrimSpace(name)
	if displayName == "" {
		displayName = "Google Gemini"
	}
	return &Provider{
		instanceID: instanceID,
		name:       displayName,
		baseURL:    defaultBaseURL,
	}
}

func (p *Provider) GetID() string         { return string(types.ProviderGoogle) }
func (p *Provider) GetInstanceID() string { return p.instanceID }
func (p *Provider) GetName() string       { return p.name }
func (p *Provider) SetName(name string)   { p.name = name }

func (p *Provider) SetupAuth(options *types.AuthOptions) error {
	token, baseURL, name, err := SetupAuth(p.instanceID, options)
	if err != nil {
		return err
	}
	p.token = token
	p.baseURL = baseURL
	p.name = name
	return nil
}

func (p *Provider) GetToken() string    { return p.token }
func (p *Provider) RefreshToken() error { return nil }

func (p *Provider) GetBaseURL() string {
	p.loadConfigFromDB()
	return p.baseURL
}

func (p *Provider) GetHeaders(forVision bool) map[string]string {
	p.loadConfigFromDB()
	return Headers(p.token)
}

func (p *Provider) GetModels() (*types.ModelsResponse, error) {
	p.loadConfigFromDB()
	return FetchModels(p.instanceID, p.token, p.baseURL)
}

func (p *Provider) CreateChatCompletions(payload map[string]interface{}) (map[string]interface{}, error) {
	return nil, fmt.Errorf("provider %s: use the adapter for chat completions", p.GetID())
}

func (p *Provider) CreateEmbeddings(payload map[string]interface{}) (map[string]interface{}, error) {
	return nil, fmt.Errorf("provider %s: embeddings not yet implemented in Go backend", p.GetID())
}

func (p *Provider) GetUsage() (map[string]interface{}, error) {
	return map[string]interface{}{}, nil
}

func (p *Provider) GetAdapter() types.ProviderAdapter {
	return &Adapter{provider: p}
}

func (p *Provider) loadConfigRecord() (map[string]interface{}, error) {
	configStore := database.NewProviderConfigStore()
	record, err := configStore.Get(p.instanceID)
	if err != nil || record == nil {
		return nil, err
	}
	var config map[string]interface{}
	if err := json.Unmarshal([]byte(record.ConfigData), &config); err != nil {
		return nil, err
	}
	return config, nil
}

func (p *Provider) loadConfigFromDB() {
	p.configOnce.Do(func() {
		config, err := p.loadConfigRecord()
		if err != nil {
			log.Warn().Err(err).Str("provider", p.instanceID).Msg("Failed to load provider config")
		}
		if config != nil {
			p.applyConfig(config)
		}
		tokenStore := database.NewTokenStore()
		record, err := tokenStore.Get(p.instanceID)
		if err != nil {
			log.Warn().Err(err).Str("provider", p.instanceID).Msg("Failed to load provider token")
			return
		}
		if record == nil {
			return
		}
		var tokenData map[string]interface{}
		if err := json.Unmarshal([]byte(record.TokenData), &tokenData); err != nil {
			p.token = record.TokenData
			return
		}
		for _, key := range []string{"token", "api_key", "apiKey", "access_token", "github_token", "copilot_token"} {
			if t, ok := tokenData[key].(string); ok && t != "" {
				p.token = t
				break
			}
		}
		p.applyConfig(sanitizeTokenConfig(tokenData))
		if p.baseURL == "" {
			p.baseURL = defaultBaseURL
		}
		if p.name == "" {
			p.name = "Google Gemini"
		}
	})
}

func (p *Provider) LoadFromDB() error {
	p.loadConfigFromDB()
	return nil
}

func (p *Provider) applyConfig(config map[string]interface{}) {
	if len(config) == 0 {
		return
	}
	if p.config == nil {
		p.config = make(map[string]interface{}, len(config))
	}
	for key, value := range config {
		p.config[key] = value
	}
	if baseURL, ok := firstString(config, "base_url", "baseUrl"); ok {
		p.baseURL = strings.TrimRight(strings.TrimSpace(baseURL), "/")
	}
	if p.baseURL == "" {
		p.baseURL = defaultBaseURL
	}
}

func (a *Adapter) GetProvider() types.Provider { return a.provider }
func (a *Adapter) RemapModel(model string) string {
	return strings.TrimSpace(model)
}
func (a *Adapter) Execute(ctx context.Context, request *cif.CanonicalRequest) (*cif.CanonicalResponse, error) {
	a.provider.loadConfigFromDB()
	return Execute(ctx, a.provider.token, a.provider.baseURL, request)
}
func (a *Adapter) ExecuteStream(ctx context.Context, request *cif.CanonicalRequest) (<-chan cif.CIFStreamEvent, error) {
	a.provider.loadConfigFromDB()
	return Stream(ctx, a.provider.token, a.provider.baseURL, request)
}

func sanitizeTokenConfig(tokenData map[string]interface{}) map[string]interface{} {
	if len(tokenData) == 0 {
		return nil
	}
	filtered := make(map[string]interface{}, len(tokenData))
	for key, value := range tokenData {
		switch key {
		case "token", "api_key", "apiKey", "access_token", "github_token", "copilot_token", "refresh_token", "refreshToken", "id_token", "idToken", "expires_at", "expiresAt", "expiry", "expiry_date":
			continue
		default:
			filtered[key] = value
		}
	}
	if len(filtered) == 0 {
		return nil
	}
	return filtered
}

func firstString(values map[string]interface{}, keys ...string) (string, bool) {
	for _, key := range keys {
		if value, ok := values[key].(string); ok && strings.TrimSpace(value) != "" {
			return value, true
		}
	}
	return "", false
}

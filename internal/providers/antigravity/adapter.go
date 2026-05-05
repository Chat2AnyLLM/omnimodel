package antigravity

import (
	"context"
	"encoding/json"
	"fmt"
	"omnillm/internal/cif"
	"omnillm/internal/database"
	"omnillm/internal/providers/types"
	"sync"
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
	displayName := name
	if displayName == "" {
		displayName = "Antigravity"
	}
	return &Provider{
		instanceID: instanceID,
		name:       displayName,
		baseURL:    defaultBaseURL,
	}
}

func (p *Provider) GetID() string         { return string(types.ProviderAntigravity) }
func (p *Provider) GetInstanceID() string { return p.instanceID }
func (p *Provider) GetName() string       { return p.name }
func (p *Provider) SetName(name string)   { p.name = name }

func (p *Provider) SetupAuth(options *types.AuthOptions) error {
	clientID := options.ClientID
	clientSecret := options.ClientSecret
	if clientID == "" || clientSecret == "" {
		return fmt.Errorf("antigravity: OAuth client_id and client_secret are required")
	}
	tokenStore := database.NewTokenStore()
	tokenData := map[string]interface{}{
		"client_id":     clientID,
		"client_secret": clientSecret,
	}
	if err := tokenStore.Save(p.instanceID, tokenData); err != nil {
		return fmt.Errorf("antigravity: failed to save client credentials: %w", err)
	}
	p.baseURL = defaultBaseURL
	p.name = "Antigravity"
	return nil
}

func (p *Provider) GetToken() string { return p.token }
func (p *Provider) RefreshToken() error { return nil }

func (p *Provider) GetBaseURL() string {
	p.LoadFromDB()
	return p.baseURL
}

func (p *Provider) GetHeaders(forVision bool) map[string]string {
	return map[string]string{
		"Authorization": "Bearer " + p.token,
		"Content-Type":  "application/json",
	}
}

func (p *Provider) GetModels() (*types.ModelsResponse, error) {
	return GetModels(p.instanceID), nil
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

func (p *Provider) LoadFromDB() error {
	tokenStore := database.NewTokenStore()
	record, err := tokenStore.Get(p.instanceID)
	if err != nil {
		return fmt.Errorf("failed to load token: %w", err)
	}
	if record == nil {
		return nil
	}
	var tokenData map[string]interface{}
	if err := json.Unmarshal([]byte(record.TokenData), &tokenData); err != nil {
		p.token = record.TokenData
		return nil
	}
	if t, ok := tokenData["access_token"].(string); ok && t != "" {
		p.token = t
	}
	if projectID, ok := tokenData["project_id"].(string); ok && projectID != "" {
		if p.config == nil {
			p.config = map[string]interface{}{}
		}
		p.config["project_id"] = projectID
	}
	if email, ok := tokenData["email"].(string); ok && email != "" && p.name == "" {
		p.name = "Antigravity (" + email + ")"
	}
	if p.baseURL == "" {
		p.baseURL = defaultBaseURL
	}
	return nil
}

func (p *Provider) ApplyTokenFromDB() {
	_ = p.LoadFromDB()
}

func (a *Adapter) GetProvider() types.Provider { return a.provider }
func (a *Adapter) RemapModel(model string) string { return RemapModel(model) }
func (a *Adapter) Execute(ctx context.Context, request *cif.CanonicalRequest) (*cif.CanonicalResponse, error) {
	a.provider.LoadFromDB()
	projectID := ""
	if a.provider.config != nil {
		if v, ok := a.provider.config["project_id"].(string); ok {
			projectID = v
		}
	}
	return Execute(ctx, a.provider.token, a.provider.baseURL, projectID, request)
}
func (a *Adapter) ExecuteStream(ctx context.Context, request *cif.CanonicalRequest) (<-chan cif.CIFStreamEvent, error) {
	a.provider.LoadFromDB()
	projectID := ""
	if a.provider.config != nil {
		if v, ok := a.provider.config["project_id"].(string); ok {
			projectID = v
		}
	}
	return Stream(ctx, a.provider.token, a.provider.baseURL, projectID, request)
}

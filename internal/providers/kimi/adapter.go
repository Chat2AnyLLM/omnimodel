package kimi

import (
	"context"
	"encoding/json"
	"fmt"
	"strings"
	"sync"

	"omnillm/internal/cif"
	"omnillm/internal/database"
	"omnillm/internal/providers/shared"
	"omnillm/internal/providers/types"
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
		displayName = "Kimi"
	}
	return &Provider{
		instanceID: instanceID,
		name:       displayName,
		baseURL:    BaseURL,
	}
}

func (p *Provider) GetID() string         { return string(types.ProviderKimi) }
func (p *Provider) GetInstanceID() string { return p.instanceID }
func (p *Provider) GetName() string       { return p.name }
func (p *Provider) SetName(name string)   { p.name = name }
func (p *Provider) SetInstanceID(id string) {
	p.instanceID = id
}

func (p *Provider) SetupAuth(options *types.AuthOptions) error {
	switch options.Method {
	case "", "api-key":
		token, baseURL, name, config, err := SetupAPIKeyAuth(p.instanceID, options)
		if err != nil {
			return err
		}
		p.token = token
		p.baseURL = baseURL
		p.name = name
		p.config = config
		return nil
	case "oauth":
		return p.LoadFromDB()
	default:
		return fmt.Errorf("kimi: unsupported auth method: %s", options.Method)
	}
}

func (p *Provider) GetToken() string { return p.token }

func (p *Provider) RefreshToken() error {
	p.token = EnsureFreshToken(p.instanceID, p.token)
	return nil
}

func (p *Provider) GetBaseURL() string {
	p.ensureConfig()
	return p.baseURL
}

func (p *Provider) GetConfig() map[string]interface{} {
	p.ensureConfig()
	return p.config
}

func (p *Provider) GetHeaders(forVision bool) map[string]string {
	p.ensureConfig()
	return Headers(p.token, false, p.config)
}

func (p *Provider) GetModels() (*types.ModelsResponse, error) {
	p.ensureConfig()
	return GetModels(p.instanceID, p.token, p.baseURL, p.config)
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
	token, baseURL, config, err := LoadTokenFromDB(p.instanceID)
	if err != nil {
		return err
	}
	if token != "" {
		p.token = token
	}
	if baseURL != "" {
		p.baseURL = baseURL
	}
	if config != nil {
		p.applyConfig(config)
	}
	return nil
}

func (p *Provider) ensureConfig() {
	p.configOnce.Do(func() {
		configStore := database.NewProviderConfigStore()
		record, err := configStore.Get(p.instanceID)
		if err != nil || record == nil {
			return
		}
		var config map[string]interface{}
		if err := json.Unmarshal([]byte(record.ConfigData), &config); err != nil {
			return
		}
		p.applyConfig(config)
	})
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
	p.baseURL = NormalizeBaseURL(p.config)
	if p.name == "" || p.name == p.instanceID || p.name == "Kimi" {
		if authType, _ := shared.FirstString(p.config, "auth_type", "authType"); authType == "api-key" {
			p.name = APIKeyProviderName(p.config)
		}
	}
}

func (a *Adapter) GetProvider() types.Provider { return a.provider }

func (a *Adapter) RemapModel(model string) string { return strings.TrimSpace(model) }

func (a *Adapter) Execute(ctx context.Context, request *cif.CanonicalRequest) (*cif.CanonicalResponse, error) {
	a.provider.ensureConfig()
	payload := a.buildOpenAIPayload(request)
	payload["stream"] = false
	return shared.ExecuteOpenAIWithPayload(ctx, ChatURL(a.provider.baseURL), Headers(a.provider.token, false, a.provider.config), payload)
}

func (a *Adapter) ExecuteStream(ctx context.Context, request *cif.CanonicalRequest) (<-chan cif.CIFStreamEvent, error) {
	a.provider.ensureConfig()
	payload := a.buildOpenAIPayload(request)
	payload["stream"] = true
	return shared.StreamOpenAIWithPayload(ctx, ChatURL(a.provider.baseURL), Headers(a.provider.token, true, a.provider.config), payload)
}

func (a *Adapter) buildOpenAIPayload(request *cif.CanonicalRequest) map[string]interface{} {
	model := a.RemapModel(request.Model)
	messages := shared.CIFMessagesToOpenAI(request.Messages)
	if request.SystemPrompt != nil && strings.TrimSpace(*request.SystemPrompt) != "" {
		messages = append([]map[string]interface{}{ {
			"role":    "system",
			"content": *request.SystemPrompt,
		}}, messages...)
	}
	if IsThinkingModel(model) {
		EnsureReasoningContentInMessages(messages)
	}
	return BuildOpenAIPayload(model, messages, request, false)
}

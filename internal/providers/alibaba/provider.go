package alibaba

import (
	"encoding/json"
	"fmt"
	"omnillm/internal/database"
	"omnillm/internal/providers/shared"
	"omnillm/internal/providers/types"
	"strings"
	"sync"

	"github.com/rs/zerolog/log"
)

const UserAgent = "OmniLLM/1.0"

// API mode constant for OpenAI-compatible DashScope endpoints.
const AlibabaAPIModeOpenAICompatible = "openai-compatible"

// Provider implements types.Provider for Alibaba DashScope.
type Provider struct {
	instanceID string
	name       string
	token      string
	baseURL    string
	config     map[string]interface{}
	// configOnce ensures config is loaded from the database exactly once,
	// even under concurrent requests.  Replaces the racy configLoaded bool.
	configOnce sync.Once
}

// NewProvider creates a new Alibaba Provider.
func NewProvider(instanceID, name string) *Provider {
	return &Provider{
		instanceID: instanceID,
		name:       name,
		baseURL:    BaseURLGlobal,
	}
}

// GetID returns the provider type identifier.
func (p *Provider) GetID() string { return "alibaba" }

// GetInstanceID returns the unique instance identifier.
func (p *Provider) GetInstanceID() string { return p.instanceID }

// GetName returns the human-readable provider name.
func (p *Provider) GetName() string         { return p.name }
func (p *Provider) SetName(name string)     { p.name = name }
func (p *Provider) SetInstanceID(id string) { p.instanceID = id }

// SetupAuth handles API-key authentication and persists credentials.
func (p *Provider) SetupAuth(options *types.AuthOptions) error {
	token, baseURL, name, config, err := SetupAPIKeyAuth(p.instanceID, options)
	if err != nil {
		return err
	}
	p.token = token
	p.baseURL = baseURL
	p.name = name
	p.config = config
	return nil
}

func (p *Provider) GetToken() string    { return p.token }
func (p *Provider) RefreshToken() error { return nil }

func (p *Provider) GetBaseURL() string {
	p.ensureConfig()
	return p.baseURL
}

func (p *Provider) GetHeaders(forVision bool) map[string]string {
	p.ensureConfig()
	return Headers(p.token, false, p.config)
}

func (p *Provider) GetConfig() map[string]interface{} {
	p.ensureConfig()
	return p.config
}

func (p *Provider) ensureConfig() {
	// sync.Once guarantees this runs exactly once across concurrent goroutines,
	// replacing the racy if p.configLoaded { return } pattern.
	p.configOnce.Do(func() {
		store := database.NewProviderConfigStore()
		rec, err := store.Get(p.instanceID)
		if err != nil || rec == nil {
			return
		}
		var cfg map[string]interface{}
		if err := json.Unmarshal([]byte(rec.ConfigData), &cfg); err != nil {
			log.Warn().Err(err).Str("provider", p.instanceID).Msg("alibaba: failed to parse config")
			return
		}
		p.applyConfig(cfg)
	})
}

func (p *Provider) applyConfig(cfg map[string]interface{}) {
	if p.config == nil {
		p.config = make(map[string]interface{}, len(cfg))
	}
	for k, v := range cfg {
		p.config[k] = v
	}
	p.baseURL = NormalizeBaseURL(p.config)
}

func (p *Provider) GetModels() (*types.ModelsResponse, error) {
	p.ensureConfig()
	return GetModels(p.instanceID, p.token, p.baseURL, p.config)
}

func (p *Provider) CreateChatCompletions(payload map[string]interface{}) (map[string]interface{}, error) {
	return nil, fmt.Errorf("alibaba: use the adapter for chat completions")
}

func (p *Provider) CreateEmbeddings(payload map[string]interface{}) (map[string]interface{}, error) {
	return nil, fmt.Errorf("alibaba: embeddings not implemented")
}

func (p *Provider) GetUsage() (map[string]interface{}, error) {
	return map[string]interface{}{}, nil
}

func (p *Provider) GetAdapter() types.ProviderAdapter {
	return &Adapter{provider: p}
}

// LoadFromDB restores persisted credentials and config from the database.
func (p *Provider) LoadFromDB() error {
	token, tokenBaseURL, _, err := LoadTokenFromDB(p.instanceID)
	if err != nil {
		return err
	}
	if token != "" {
		p.token = token
	}

	// Load the full provider config (plan, region, base_url, etc.) from
	// provider_configs. This must happen before marking configOnce as done so
	// that GetModels and other callers get the correct base URL.
	store := database.NewProviderConfigStore()
	rec, err := store.Get(p.instanceID)
	if err != nil {
		log.Warn().Err(err).Str("provider", p.instanceID).Msg("alibaba: failed to load provider config")
	} else if rec != nil {
		var cfg map[string]interface{}
		if jsonErr := json.Unmarshal([]byte(rec.ConfigData), &cfg); jsonErr != nil {
			log.Warn().Err(jsonErr).Str("provider", p.instanceID).Msg("alibaba: failed to parse provider config")
		} else {
			p.applyConfig(cfg)
			// Legacy providers stored access_token directly in provider_configs.
			// Migrate it to p.token if the tokens table had nothing.
			if p.token == "" {
				if at, ok := shared.FirstString(cfg, "access_token"); ok && at != "" {
					p.token = at
				}
			}
		}
	} else {
		// No provider_configs row — legacy provider that only has a tokens row.
		// The token record may carry a base_url (stored by old auth flows); use
		// it so the correct regional endpoint is preserved instead of falling
		// back to the hardcoded global default.
		if tokenBaseURL != "" {
			p.baseURL = EnsureBaseURL(tokenBaseURL)
		}
	}

	if p.name == "" {
		p.name = APIKeyProviderName(p.config)
	}
	// Mark configOnce as done so ensureConfig is a no-op — credentials were
	// already loaded above via LoadFromDB (called at startup by the registry).
	p.configOnce.Do(func() {})
	log.Debug().Str("provider", p.instanceID).Bool("has_token", p.token != "").Str("base_url", p.baseURL).Msg("Alibaba: loaded from DB")
	return nil
}

// SetupAPIKeyAuth saves credentials and returns resolved values.
func SetupAPIKeyAuth(instanceID string, options *types.AuthOptions) (token, baseURL, name string, config map[string]interface{}, err error) {
	if options.APIKey == "" {
		return "", "", "", nil, fmt.Errorf("alibaba: API key is required")
	}

	region := strings.TrimSpace(options.Region)
	if region == "" {
		region = "global"
	}
	plan := NormalizeAPIPlan(options.Plan)

	tokenStore := database.NewTokenStore()
	tokenData := map[string]interface{}{"access_token": options.APIKey}
	if err := tokenStore.Save(instanceID, tokenData); err != nil {
		return "", "", "", nil, fmt.Errorf("alibaba: failed to save token: %w", err)
	}

	cfg := map[string]interface{}{
		"auth_type": "api-key",
		"region":    region,
		"plan":      plan,
	}
	if endpoint := strings.TrimSpace(options.Endpoint); endpoint != "" {
		cfg["base_url"] = endpoint
	}

	configStore := database.NewProviderConfigStore()
	if err := configStore.Save(instanceID, cfg); err != nil {
		return "", "", "", nil, fmt.Errorf("alibaba: failed to save config: %w", err)
	}

	resolvedURL := NormalizeBaseURL(cfg)
	resolvedName := APIKeyProviderName(cfg)

	log.Info().Str("provider", instanceID).Str("region", region).Str("plan", plan).
		Msg("Alibaba authenticated via API key")

	return options.APIKey, resolvedURL, resolvedName, cfg, nil
}

// LoadTokenFromDB reads the persisted Alibaba token from the database.
func LoadTokenFromDB(instanceID string) (token, baseURL string, config map[string]interface{}, err error) {
	tokenStore := database.NewTokenStore()
	record, err := tokenStore.Get(instanceID)
	if err != nil {
		return "", "", nil, fmt.Errorf("alibaba: failed to load token: %w", err)
	}
	if record == nil {
		return "", "", nil, nil
	}

	var td TokenData
	if err := json.Unmarshal([]byte(record.TokenData), &td); err != nil {
		return "", "", nil, fmt.Errorf("alibaba: failed to parse token data: %w", err)
	}

	cfg := map[string]interface{}{
		"auth_type": td.AuthType,
		"base_url":  td.BaseURL,
	}
	return td.AccessToken, NormalizeBaseURL(cfg), cfg, nil
}

// Headers returns HTTP headers for DashScope requests.
func Headers(token string, stream bool, config map[string]interface{}) map[string]string {
	h := map[string]string{
		"Authorization": "Bearer " + token,
		"Content-Type":  "application/json",
		"Accept":        "application/json",
	}
	if stream {
		h["Accept"] = "text/event-stream"
	}
	return h
}

// ChatURL returns the chat completions endpoint for the given base URL.
func ChatURL(baseURL string) string {
	if baseURL == "" {
		baseURL = BaseURLGlobal
	}
	return strings.TrimRight(baseURL, "/") + "/chat/completions"
}

// NormalizeBaseURL derives the base URL from a provider config map.
func NormalizeBaseURL(config map[string]interface{}) string {
	if baseURL, ok := shared.FirstString(config, "base_url", "baseUrl"); ok {
		return EnsureBaseURL(baseURL)
	}
	plan, _ := shared.FirstString(config, "plan")
	region, _ := shared.FirstString(config, "region")
	return DefaultAPIBaseURL(plan, region)
}

// EnsureBaseURL normalizes a base URL to have https scheme and /v1 suffix.
func EnsureBaseURL(raw string) string {
	s := strings.TrimSpace(raw)
	if s == "" {
		return BaseURLGlobal
	}
	if !strings.HasPrefix(s, "http://") && !strings.HasPrefix(s, "https://") {
		s = "https://" + s
	}
	s = strings.TrimRight(s, "/")
	if !strings.HasSuffix(s, "/v1") {
		s += "/v1"
	}
	return s
}

// NormalizeAPIPlan normalizes a plan string to "coding-plan" or "standard".
func NormalizeAPIPlan(plan string) string {
	switch strings.ToLower(strings.TrimSpace(plan)) {
	case "coding", "coding-plan", "coding_plan":
		return "coding-plan"
	default:
		return "standard"
	}
}

// DefaultAPIBaseURL returns the default DashScope base URL for plan+region.
func DefaultAPIBaseURL(plan, region string) string {
	switch NormalizeAPIPlan(plan) {
	case "coding-plan":
		if strings.EqualFold(strings.TrimSpace(region), "china") {
			return CodingPlanBaseURLChina
		}
		return CodingPlanBaseURLGlobal
	default:
		if strings.EqualFold(strings.TrimSpace(region), "china") {
			return BaseURLChina
		}
		return BaseURLGlobal
	}
}

// APIKeyProviderName returns the display name for this provider.
func APIKeyProviderName(config map[string]interface{}) string {
	region, _ := shared.FirstString(config, "region")
	if region == "" {
		region = "global"
	}
	planRaw, _ := config["plan"].(string)
	switch NormalizeAPIPlan(planRaw) {
	case "coding-plan":
		return "Alibaba Coding Plan (" + region + ")"
	default:
		return "Alibaba DashScope Standard (" + region + ")"
	}
}

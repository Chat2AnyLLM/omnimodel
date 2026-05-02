// Package database provides SQLite-based persistence for OmniLLM
package database

import "time"

// parseTime converts a string timestamp to time.Time
// SQLite returns timestamps as strings from modernc.org/sqlite
func parseTime(s string) time.Time {
	if s == "" {
		return time.Time{}
	}
	// Try RFC3339 format first (ISO 8601 with timezone)
	t, err := time.Parse(time.RFC3339, s)
	if err == nil {
		return t
	}
	// Try RFC3339Nano format
	t, err = time.Parse(time.RFC3339Nano, s)
	if err == nil {
		return t
	}
	// Try SQLite datetime format
	t, err = time.Parse("2006-01-02 15:04:05", s)
	if err == nil {
		return t
	}
	// Return zero time if parsing fails
	return time.Time{}
}

type ConfigRecord struct {
	Key       string    `json:"key"`
	Value     string    `json:"value"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type ProviderInstanceRecord struct {
	InstanceID string    `json:"instance_id"`
	ProviderID string    `json:"provider_id"`
	Name       string    `json:"name"`
	Subtitle   string    `json:"subtitle"`
	Priority   int       `json:"priority"`
	Activated  bool      `json:"activated"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type TokenRecord struct {
	InstanceID string    `json:"instance_id"`
	TokenData  string    `json:"token_data"` // JSON string
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type ProviderModelStateRecord struct {
	InstanceID string    `json:"instance_id"`
	ModelID    string    `json:"model_id"`
	Enabled    bool      `json:"enabled"`
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type ProviderConfigRecord struct {
	InstanceID string    `json:"instance_id"`
	ConfigData string    `json:"config_data"` // JSON string
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type ModelConfigRecord struct {
	InstanceID string    `json:"instance_id"`
	ModelID    string    `json:"model_id"`
	Version    int       `json:"version"`
	ConfigData string    `json:"config_data"` // JSON string - for model-specific configuration
	CreatedAt  time.Time `json:"created_at"`
	UpdatedAt  time.Time `json:"updated_at"`
}

type ProviderModelsCacheRecord struct {
	InstanceID string    `json:"instance_id"`
	ModelsData string    `json:"models_data"` // JSON array of cached models
	CachedAt   time.Time `json:"cached_at"`
}

type ChatSessionRecord struct {
	SessionID string    `json:"session_id"`
	Title     string    `json:"title"`
	ModelID   string    `json:"model_id"`
	APIShape  string    `json:"api_shape"`
	CreatedAt time.Time `json:"created_at"`
	UpdatedAt time.Time `json:"updated_at"`
}

type ChatMessageRecord struct {
	MessageID string    `json:"message_id"`
	SessionID string    `json:"session_id"`
	Role      string    `json:"role"` // user, assistant, system
	Content   string    `json:"content"`
	CreatedAt time.Time `json:"created_at"`
}

type LbStrategy string

const (
	LbStrategyRoundRobin LbStrategy = "round-robin"
	LbStrategyRandom     LbStrategy = "random"
	LbStrategyPriority   LbStrategy = "priority"
	LbStrategyWeighted   LbStrategy = "weighted"
)

type VirtualModelRecord struct {
	VirtualModelID string     `json:"virtual_model_id"`
	Name           string     `json:"name"`
	Description    string     `json:"description"`
	APIShape       string     `json:"api_shape"`
	LbStrategy     LbStrategy `json:"lb_strategy"`
	Enabled        bool       `json:"enabled"`
	CreatedAt      time.Time  `json:"created_at"`
	UpdatedAt      time.Time  `json:"updated_at"`
}

type VirtualModelUpstreamRecord struct {
	ID             int64     `json:"id"`
	VirtualModelID string    `json:"virtual_model_id"`
	ProviderID     string    `json:"provider_id"`
	ModelID        string    `json:"model_id"`
	Weight         int       `json:"weight"`
	Priority       int       `json:"priority"`
	CreatedAt      time.Time `json:"created_at"`
	UpdatedAt      time.Time `json:"updated_at"`
}

// AccessTokenRecord holds a generated access token for authenticating with the proxy.
type AccessTokenRecord struct {
	ID             string     `json:"id"`
	Name           string     `json:"name"`
	TokenHash      string     `json:"-"`               // SHA-256 hash, never exposed via JSON
	TokenPlaintext string     `json:"token_plaintext,omitempty"`
	Prefix         string     `json:"prefix"`           // first 8 chars for display
	CreatedAt      time.Time  `json:"created_at"`
	ExpiresAt      *time.Time `json:"expires_at"`       // nil means no expiry
	LastUsedAt     *time.Time `json:"last_used_at"`
	Enabled        bool       `json:"enabled"`
}

// MeteringRecord holds per-request usage data recorded after each API call.
type MeteringRecord struct {
	ID           int64     `json:"id"`
	RequestID    string    `json:"request_id"`
	ModelID      string    `json:"model_id"`      // canonical model name as requested
	ModelUsed    string    `json:"model_used"`    // actual model reported by provider
	ProviderID   string    `json:"provider_id"`   // provider instance_id
	Client       string    `json:"client"`
	APIShape     string    `json:"api_shape"`     // "openai" | "anthropic"
	InputTokens  int       `json:"input_tokens"`
	OutputTokens int       `json:"output_tokens"`
	TotalTokens  int       `json:"total_tokens"`
	LatencyMS    int64     `json:"latency_ms"`
	IsStream     bool      `json:"is_stream"`
	StatusCode   int       `json:"status_code"`  // 200 on success, 4xx/5xx on error
	ErrorMessage string    `json:"error_message"`
	CreatedAt    time.Time `json:"created_at"`
}

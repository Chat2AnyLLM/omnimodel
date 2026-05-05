package omnicode

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// Config holds OmniCode runtime preferences persisted across sessions.
type Config struct {
	Model        string `json:"model,omitempty"`
	Mode         string `json:"mode,omitempty"`
	APIShape     string `json:"api_shape,omitempty"`
	AgentBackend string `json:"agent_backend,omitempty"`
	Autopilot    bool   `json:"autopilot"`
	MaxTurns     int    `json:"max_turns,omitempty"`
}

func configPath() (string, error) {
	home, err := os.UserHomeDir()
	if err != nil {
		return "", fmt.Errorf("home dir: %w", err)
	}
	return filepath.Join(home, ".config", "omnicode", "config.json"), nil
}

// LoadConfig reads the config file, creating it with defaults if it does not exist.
func LoadConfig() (*Config, error) {
	path, err := configPath()
	if err != nil {
		return nil, err
	}

	data, err := os.ReadFile(path)
	if os.IsNotExist(err) {
		cfg := &Config{}
		if err := SaveConfig(cfg); err != nil {
			return nil, err
		}
		return cfg, nil
	}
	if err != nil {
		return nil, fmt.Errorf("read config: %w", err)
	}

	var cfg Config
	if err := json.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	return &cfg, nil
}

// SaveConfig writes the config to disk, creating parent directories as needed.
func SaveConfig(cfg *Config) error {
	path, err := configPath()
	if err != nil {
		return err
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o700); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	data, err := json.MarshalIndent(cfg, "", "  ")
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	if err := os.WriteFile(path, data, 0o600); err != nil {
		return fmt.Errorf("write config: %w", err)
	}
	return nil
}

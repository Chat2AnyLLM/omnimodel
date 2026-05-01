// Package github provides GitHub API client functionality for OAuth and Copilot token management
package github

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/rs/zerolog/log"
)

// Shared HTTP client with default timeout for non-streaming requests.
var httpClient = &http.Client{Timeout: 15 * time.Second}

const (
	GitHubBaseURL    = "https://github.com"
	GitHubAPIBaseURL = "https://api.github.com"
	ClientID         = "Iv1.b507a08c87ecfe98"
	AppScopes        = "read:user"
	CopilotVersion   = "0.26.7"
	APIVersion       = "2025-04-01"
	UserAgent        = "GitHubCopilotChat/0.26.7"
	EditorVersion    = "vscode/1.83.1"
	PluginVersion    = "copilot-chat/0.26.7"
)

// DeviceCodeResponse from GitHub's device code endpoint
type DeviceCodeResponse struct {
	DeviceCode              string `json:"device_code"`
	UserCode                string `json:"user_code"`
	VerificationURI         string `json:"verification_uri"`
	VerificationURIComplete string `json:"verification_uri_complete"`
	ExpiresIn               int    `json:"expires_in"`
	Interval                int    `json:"interval"`
}

// AccessTokenResponse from GitHub's OAuth token endpoint
type AccessTokenResponse struct {
	AccessToken string `json:"access_token"`
	TokenType   string `json:"token_type"`
	Scope       string `json:"scope"`
	Error       string `json:"error,omitempty"`
}

// CopilotTokenResponse from the Copilot internal API
type CopilotTokenResponse struct {
	Token     string `json:"token"`
	ExpiresAt int64  `json:"expires_at"`
	RefreshIn int    `json:"refresh_in"`
}

// GetDeviceCode initiates the GitHub OAuth device code flow
func GetDeviceCode() (*DeviceCodeResponse, error) {
	body, _ := json.Marshal(map[string]string{
		"client_id": ClientID,
		"scope":     AppScopes,
	})

	req, err := http.NewRequest("POST", GitHubBaseURL+"/login/device/code", bytes.NewBuffer(body))
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")

	client := httpClient
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("device code request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("device code request failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	var result DeviceCodeResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode device code response: %w", err)
	}

	return &result, nil
}

// PollAccessToken polls for the access token after the user has authorized the device
func PollAccessToken(deviceCode *DeviceCodeResponse) (string, error) {
	body, _ := json.Marshal(map[string]string{
		"client_id":   ClientID,
		"device_code": deviceCode.DeviceCode,
		"grant_type":  "urn:ietf:params:oauth:grant-type:device_code",
	})

	interval := time.Duration(deviceCode.Interval+1) * time.Second
	deadline := time.Now().Add(time.Duration(deviceCode.ExpiresIn) * time.Second)

	for time.Now().Before(deadline) {
		req, err := http.NewRequest("POST", GitHubBaseURL+"/login/oauth/access_token", bytes.NewBuffer(body))
		if err != nil {
			return "", fmt.Errorf("failed to create request: %w", err)
		}
		req.Header.Set("Content-Type", "application/json")
		req.Header.Set("Accept", "application/json")

		client := httpClient
		resp, err := client.Do(req)
		if err != nil {
			log.Warn().Err(err).Msg("Token poll request failed, retrying...")
			time.Sleep(interval)
			continue
		}

		var result AccessTokenResponse
		if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
			resp.Body.Close()
			log.Warn().Err(err).Msg("Failed to decode token response, retrying...")
			time.Sleep(interval)
			continue
		}
		resp.Body.Close()

		if result.AccessToken != "" {
			return result.AccessToken, nil
		}

		// Check for specific error states
		if result.Error == "expired_token" {
			return "", fmt.Errorf("device code expired, please try again")
		}

		// authorization_pending or slow_down - keep polling
		if result.Error == "slow_down" {
			interval += 5 * time.Second
		}

		time.Sleep(interval)
	}

	return "", fmt.Errorf("device code expired before authorization")
}

// GetCopilotToken exchanges a GitHub access token for a short-lived Copilot API token
func GetCopilotToken(githubToken string) (*CopilotTokenResponse, error) {
	req, err := http.NewRequest("GET", GitHubAPIBaseURL+"/copilot_internal/v2/token", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "token "+githubToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Editor-Version", EditorVersion)
	req.Header.Set("Editor-Plugin-Version", PluginVersion)
	req.Header.Set("User-Agent", UserAgent)
	req.Header.Set("X-Github-Api-Version", APIVersion)

	client := httpClient
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("copilot token request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("copilot token request failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	var result CopilotTokenResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode copilot token response: %w", err)
	}

	return &result, nil
}

// CopilotProviderName returns a human-friendly display name for a GitHub Copilot
// provider, derived from the /user API response.  Priority:
//  1. "name (email)"  — when both real name and email are present
//  2. "name (login)"  — when real name is present but no public email
//  3. "email"         — when only email is present
//  4. "login"         — fallback
func CopilotProviderName(user map[string]interface{}) string {
	login, _ := user["login"].(string)
	email, _ := user["email"].(string)
	if email == "" {
		email, _ = user["notification_email"].(string)
	}
	realName, _ := user["name"].(string)

	switch {
	case realName != "" && email != "":
		return fmt.Sprintf("GitHub Copilot (%s · %s)", realName, email)
	case realName != "" && login != "":
		return fmt.Sprintf("GitHub Copilot (%s · %s)", realName, login)
	case email != "":
		return fmt.Sprintf("GitHub Copilot (%s)", email)
	case login != "":
		return fmt.Sprintf("GitHub Copilot (%s)", login)
	default:
		return "GitHub Copilot"
	}
}

// GetUser returns basic GitHub user info
func GetUser(githubToken string) (map[string]interface{}, error) {
	req, err := http.NewRequest("GET", GitHubAPIBaseURL+"/user", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "token "+githubToken)
	req.Header.Set("Accept", "application/json")
	req.Header.Set("User-Agent", UserAgent)

	client := httpClient
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("user info request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("user info request failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode user response: %w", err)
	}

	return result, nil
}

// GetCopilotUsage returns Copilot usage/quota information
func GetCopilotUsage(githubToken string) (map[string]interface{}, error) {
	req, err := http.NewRequest("GET", GitHubAPIBaseURL+"/copilot_internal/user", nil)
	if err != nil {
		return nil, fmt.Errorf("failed to create request: %w", err)
	}

	req.Header.Set("Authorization", "token "+githubToken)
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("Accept", "application/json")
	req.Header.Set("Editor-Version", EditorVersion)
	req.Header.Set("Editor-Plugin-Version", PluginVersion)
	req.Header.Set("User-Agent", UserAgent)

	client := httpClient
	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("usage request failed: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		respBody, _ := io.ReadAll(resp.Body)
		return nil, fmt.Errorf("usage request failed with status %d: %s", resp.StatusCode, string(respBody))
	}

	var result map[string]interface{}
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		return nil, fmt.Errorf("failed to decode usage response: %w", err)
	}

	return result, nil
}

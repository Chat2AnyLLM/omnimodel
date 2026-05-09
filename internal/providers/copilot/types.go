// Package copilot contains shared types for the GitHub Copilot provider
package copilot

import (
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"time"

	ghservice "omnillm/internal/services/github"
)

// Shared HTTP clients: one for normal requests with timeout, one for streaming.
var (
	copilotHTTPClient = &http.Client{
		Timeout: 120 * time.Second,
		Transport: &http.Transport{
			Proxy:                 http.ProxyFromEnvironment,
			ForceAttemptHTTP2:     true,
			MaxIdleConns:          100,
			MaxIdleConnsPerHost:   20,
			MaxConnsPerHost:       50,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
		},
	}
	copilotStreamClient = &http.Client{
		Transport: &http.Transport{
			Proxy:                 http.ProxyFromEnvironment,
			ForceAttemptHTTP2:     true,
			MaxIdleConns:          100,
			MaxIdleConnsPerHost:   20,
			MaxConnsPerHost:       50,
			IdleConnTimeout:       90 * time.Second,
			TLSHandshakeTimeout:   10 * time.Second,
			ExpectContinueTimeout: 1 * time.Second,
		},
	}
)

type GitHubCopilotProvider struct {
	id           string
	instanceID   string
	name         string
	token        string // short-lived Copilot API token
	githubToken  string // long-lived GitHub OAuth token
	expiresAt    int64  // Copilot token expiry (unix timestamp)
	baseURL      string
	tokenFetcher func(string) (*ghservice.CopilotTokenResponse, error)
	shapeCache   modelShapeCache // populated by GetModels(); nil means uncached
}

type CopilotAdapter struct {
	provider *GitHubCopilotProvider
}

const (
	copilotMaxUserIDLength   = 64
	copilotMaxToolNameLength = 64
)

// copilotAPIShape identifies which upstream wire format to use for a model.
type copilotAPIShape string

const (
	shapeChat      copilotAPIShape = "chat_completions" // POST /chat/completions
	shapeResponses copilotAPIShape = "responses"        // POST /responses
)

// modelShapeCache maps a Copilot model ID to its preferred API shape.
// Built from the supported_endpoints field returned by GET /models.
type modelShapeCache map[string]copilotAPIShape

var copilotToolNamePattern = regexp.MustCompile(`[^A-Za-z0-9_-]`)

type copilotAPIError struct {
	statusCode int
	body       []byte
}

func (e *copilotAPIError) Error() string {
	return fmt.Sprintf("API request failed with status %d: %s", e.statusCode, string(e.body))
}

func (e *copilotAPIError) StatusCode() int {
	if e == nil {
		return 0
	}
	return e.statusCode
}

func (e *copilotAPIError) IsAuthenticationError() bool {
	if e == nil {
		return false
	}
	if e.statusCode == http.StatusUnauthorized || e.statusCode == http.StatusForbidden {
		return true
	}

	body := strings.ToLower(string(e.body))
	return strings.Contains(body, "token expired") || strings.Contains(body, "unauthorized")
}

type copilotErrorEnvelope struct {
	Error struct {
		Message string `json:"message"`
		Type    string `json:"type"`
		Code    string `json:"code"`
	} `json:"error"`
}

type copilotToolNameMapper struct {
	upstreamByCanonical map[string]string
	canonicalByUpstream map[string]string
}

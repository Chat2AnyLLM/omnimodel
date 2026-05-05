package routes

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"net/http"
	"strings"
	"sync"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"

	antigravitypkg "omnillm/internal/providers/antigravity"
	"omnillm/internal/database"
	"omnillm/internal/registry"
)

// ─── Pending OAuth state ──────────────────────────────────────────────────────

type antigravityOAuthState struct {
	ProviderID    string
	ClientID      string
	ClientSecret  string
	RedirectURI   string
	State         string
	Expiry        time.Time
	IsNewProvider bool // true = auth-and-create, false = re-auth existing
	Done          bool
	Error         string
}

var (
	agOAuthMu     sync.Mutex
	agOAuthStates = map[string]*antigravityOAuthState{} // keyed by state nonce

	// agOAuthResults holds completed OAuth results keyed by provider_id so the
	// frontend can poll for completion instead of relying on window.opener
	// (which is severed by Google's Cross-Origin-Opener-Policy header).
	agOAuthResultsMu sync.Mutex
	agOAuthResults   = map[string]*antigravityOAuthResult{} // keyed by provider_id
)

type antigravityOAuthResult struct {
	Done       bool
	Error      string
	ProviderID string
	IsNew      bool
	Expiry     time.Time
}

func newAntigravityOAuthState(providerID, clientID, clientSecret, redirectURI string, isNew bool) *antigravityOAuthState {
	b := make([]byte, 16)
	_, _ = rand.Read(b)
	nonce := hex.EncodeToString(b)
	s := &antigravityOAuthState{
		ProviderID:    providerID,
		ClientID:      clientID,
		ClientSecret:  clientSecret,
		RedirectURI:   redirectURI,
		State:         nonce,
		Expiry:        time.Now().Add(10 * time.Minute),
		IsNewProvider: isNew,
	}
	agOAuthMu.Lock()
	agOAuthStates[nonce] = s
	agOAuthMu.Unlock()
	return s
}

func getAntigravityOAuthState(nonce string) *antigravityOAuthState {
	agOAuthMu.Lock()
	defer agOAuthMu.Unlock()
	return agOAuthStates[nonce]
}

func deleteAntigravityOAuthState(nonce string) {
	agOAuthMu.Lock()
	delete(agOAuthStates, nonce)
	agOAuthMu.Unlock()
}

// ─── Route: POST /providers/antigravity/start-oauth ──────────────────────────
// Body: { "client_id": "…", "client_secret": "…", "provider_id": "…" (optional for re-auth) }
// Returns: { "auth_url": "…", "state": "…" }

func handleAntigravityStartOAuth(c *gin.Context) {
	var req struct {
		ClientID     string `json:"client_id"`
		ClientSecret string `json:"client_secret"`
		ProviderID   string `json:"provider_id"` // empty = create new
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "invalid request body"})
		return
	}
	req.ClientID = strings.TrimSpace(req.ClientID)
	req.ClientSecret = strings.TrimSpace(req.ClientSecret)
	if req.ClientID == "" || req.ClientSecret == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "client_id and client_secret are required"})
		return
	}

	// Determine provider ID — use an existing one or allocate a new one.
	isNew := req.ProviderID == ""
	providerID := req.ProviderID
	if isNew {
		reg := registry.GetProviderRegistry()
		providerID = reg.NextInstanceID("antigravity")
	}

	// Build redirect URI pointing back to this server.
	scheme := "http"
	host := c.Request.Host
	redirectURI := fmt.Sprintf("%s://%s/api/admin/providers/antigravity/oauth-callback", scheme, host)

	state := newAntigravityOAuthState(providerID, req.ClientID, req.ClientSecret, redirectURI, isNew)
	authURL := antigravitypkg.BuildAuthURL(req.ClientID, redirectURI, state.State)

	log.Info().
		Str("provider", providerID).
		Bool("is_new", isNew).
		Msg("Antigravity: Google OAuth flow started")

	c.JSON(http.StatusOK, gin.H{
		"auth_url":    authURL,
		"state":       state.State,
		"provider_id": providerID,
	})
}

// ─── Route: GET /providers/antigravity/oauth-callback ────────────────────────
// Google redirects here with ?code=…&state=…
// This route must be public (no auth) — Google's redirect carries no Authorization header.

// HandleAntigravityOAuthCallbackPublic is the exported handler for server.go to register
// on the public router group.
var HandleAntigravityOAuthCallbackPublic = handleAntigravityOAuthCallback

func handleAntigravityOAuthCallback(c *gin.Context) {
	code := c.Query("code")
	nonce := c.Query("state")
	errParam := c.Query("error")

	if errParam != "" {
		renderOAuthResult(c, false, "Google denied access: "+errParam)
		return
	}
	if code == "" || nonce == "" {
		renderOAuthResult(c, false, "Missing code or state parameter")
		return
	}

	state := getAntigravityOAuthState(nonce)
	if state == nil || time.Now().After(state.Expiry) {
		renderOAuthResult(c, false, "OAuth state expired or not found — please try again")
		return
	}
	deleteAntigravityOAuthState(nonce)

	// Exchange authorization code for tokens.
	tokenResp, err := antigravitypkg.ExchangeCode(state.ClientID, state.ClientSecret, code, state.RedirectURI)
	if err != nil {
		log.Error().Err(err).Str("provider", state.ProviderID).Msg("Antigravity: OAuth code exchange failed")
		renderOAuthResult(c, false, fmt.Sprintf("Token exchange failed: %v", err))
		return
	}

	// Discover the user's Cloud Code project ID (required for API calls).
	projectID := antigravitypkg.DiscoverProject(tokenResp.AccessToken)
	log.Info().Str("provider", state.ProviderID).Str("project_id", projectID).Msg("Antigravity: discovered project")

	// Fetch the authenticated user's email to use as a friendly identifier.
	email := antigravitypkg.FetchUserEmail(tokenResp.AccessToken)
	log.Info().Str("provider", state.ProviderID).Str("email", email).Msg("Antigravity: fetched user email")

	// Persist credentials + tokens + project_id.
	tokenStore := database.NewTokenStore()
	tokenData := map[string]interface{}{
		"access_token":  tokenResp.AccessToken,
		"client_id":     state.ClientID,
		"client_secret": state.ClientSecret,
		"project_id":    projectID,
	}
	if email != "" {
		tokenData["email"] = email
	}
	if tokenResp.RefreshToken != "" {
		tokenData["refresh_token"] = tokenResp.RefreshToken
	}
	if err := tokenStore.Save(state.ProviderID, tokenData); err != nil {
		log.Error().Err(err).Str("provider", state.ProviderID).Msg("Antigravity: failed to save tokens")
		renderOAuthResult(c, false, "Failed to save credentials — please try again")
		return
	}

	// Register provider (or update existing).
	reg := registry.GetProviderRegistry()
	if state.IsNewProvider {
		prov := antigravitypkg.NewProvider(state.ProviderID, "")
		prov.ApplyTokenFromDB()
		if err := reg.Register(prov, true); err != nil {
			log.Warn().Err(err).Str("provider", state.ProviderID).Msg("Antigravity: failed to register after OAuth")
		}
	} else {
		// Update existing — reload token from DB so in-memory state is fresh.
		if prov, provErr := reg.GetProvider(state.ProviderID); provErr == nil {
			if ap, ok := prov.(*antigravitypkg.Provider); ok {
				ap.ApplyTokenFromDB()
			}
		}
	}

	// Store the completed result so the frontend can poll for it.
	agOAuthResultsMu.Lock()
	agOAuthResults[state.ProviderID] = &antigravityOAuthResult{
		Done:       true,
		ProviderID: state.ProviderID,
		IsNew:      state.IsNewProvider,
		Expiry:     time.Now().Add(5 * time.Minute),
	}
	agOAuthResultsMu.Unlock()

	log.Info().Str("provider", state.ProviderID).Msg("Antigravity: Google OAuth completed successfully")
	renderOAuthResult(c, true, "")
}

// renderOAuthResult writes a small self-closing HTML page.
// It no longer uses window.opener (severed by Google's COOP header).
// The frontend polls /oauth-status instead.
func renderOAuthResult(c *gin.Context, success bool, errMsg string) {
	c.Header("Content-Type", "text/html; charset=utf-8")
	if success {
		c.String(http.StatusOK, `<!DOCTYPE html><html><head><title>Antigravity — Authenticated</title></head><body>
<p>Authenticated successfully. This window will close…</p>
<script>setTimeout(function(){ window.close(); }, 1000);</script>
</body></html>`)
	} else {
		c.String(http.StatusOK, `<!DOCTYPE html><html><head><title>Antigravity — Error</title></head><body>
<p style="color:red">OAuth failed: `+errMsg+`</p>
<p><button onclick="window.close()">Close</button></p>
</body></html>`)
	}
}

// ─── Route: GET /providers/antigravity/oauth-status ──────────────────────────
// Frontend polls this to learn when the OAuth flow completes.
// This route must be public (no auth) — the frontend polls it before auth is established.
// Query param: provider_id

// HandleAntigravityOAuthStatusPublic is the exported handler for server.go to register
// on the public router group.
var HandleAntigravityOAuthStatusPublic = handleAntigravityOAuthStatus

func handleAntigravityOAuthStatus(c *gin.Context) {
	providerID := strings.TrimSpace(c.Query("provider_id"))
	if providerID == "" {
		c.JSON(http.StatusBadRequest, gin.H{"error": "provider_id is required"})
		return
	}

	agOAuthResultsMu.Lock()
	result, ok := agOAuthResults[providerID]
	if ok && result.Done {
		delete(agOAuthResults, providerID) // consume once
	}
	agOAuthResultsMu.Unlock()

	if !ok {
		c.JSON(http.StatusOK, gin.H{"done": false})
		return
	}
	if result.Error != "" {
		c.JSON(http.StatusOK, gin.H{"done": true, "error": result.Error})
		return
	}
	c.JSON(http.StatusOK, gin.H{
		"done":        true,
		"provider_id": result.ProviderID,
		"is_new":      result.IsNew,
	})
}

package routes

import (
	"bytes"
	"context"
	"encoding/json"
	"io"
	"net/http"
	"net/http/httptest"
	"omnillm/internal/database"
	"omnillm/internal/registry"
	"os"
	"strings"
	"testing"

	"github.com/gin-gonic/gin"
)

func TestMain(m *testing.M) {
	gin.SetMode(gin.TestMode)

	tmpDir, err := os.MkdirTemp("", "routes-test-*")
	if err != nil {
		panic(err)
	}
	defer os.RemoveAll(tmpDir)

	if err := database.InitializeDatabase(tmpDir); err != nil {
		panic(err)
	}

	os.Exit(m.Run())
}

type roundTripFunc func(*http.Request) (*http.Response, error)

func (f roundTripFunc) RoundTrip(req *http.Request) (*http.Response, error) {
	return f(req)
}

func newAdminTestRouter() *gin.Engine {
	router := gin.New()
	admin := router.Group("/api/admin")
	SetupAdminRoutes(admin, 4141)
	return router
}

func performJSONRequest(
	t *testing.T,
	router http.Handler,
	method string,
	path string,
	body string,
) *httptest.ResponseRecorder {
	t.Helper()

	req := httptest.NewRequest(method, path, bytes.NewBufferString(body))
	req.Header.Set("Content-Type", "application/json")

	recorder := httptest.NewRecorder()
	router.ServeHTTP(recorder, req)
	return recorder
}

func decodeJSONBody[T any](t *testing.T, recorder *httptest.ResponseRecorder) T {
	t.Helper()

	var payload T
	if err := json.Unmarshal(recorder.Body.Bytes(), &payload); err != nil {
		t.Fatalf("failed to decode JSON body %q: %v", recorder.Body.String(), err)
	}
	return payload
}

func stubDefaultTransport(t *testing.T, fn roundTripFunc) {
	t.Helper()

	original := http.DefaultTransport
	http.DefaultTransport = fn
	t.Cleanup(func() {
		http.DefaultTransport = original
	})
}

func resetAdminTestState(t *testing.T) {
	t.Helper()

	reg := registry.GetProviderRegistry()
	configStore := database.NewProviderConfigStore()

	for instanceID := range reg.GetProviderMap() {
		_ = reg.Remove(instanceID)
		_ = configStore.Delete(instanceID)
	}

	activeAuthFlowMu.Lock()
	if activeAuthFlow != nil && activeAuthFlow.cancelFn != nil {
		activeAuthFlow.cancelFn()
	}
	activeAuthFlow = nil
	activeAuthFlowMu.Unlock()
}

func TestHandleAddProviderInstanceRouteIsRemoved(t *testing.T) {
	resetAdminTestState(t)
	t.Cleanup(func() { resetAdminTestState(t) })

	router := newAdminTestRouter()

	recorder := performJSONRequest(
		t,
		router,
		http.MethodPost,
		"/api/admin/providers/add/alibaba",
		`{}`,
	)

	if recorder.Code != http.StatusNotFound {
		t.Fatalf("expected legacy add-provider route to be removed with 404, got %d: %s", recorder.Code, recorder.Body.String())
	}

	reg := registry.GetProviderRegistry()
	if reg.IsRegistered("alibaba") {
		t.Fatal("did not expect legacy add route to register placeholder alibaba provider")
	}
}

func TestHandleAuthAndCreateProviderAzureOpenAISavesAuthenticatedProvider(t *testing.T) {
	resetAdminTestState(t)
	t.Cleanup(func() { resetAdminTestState(t) })

	router := newAdminTestRouter()

	recorder := performJSONRequest(
		t,
		router,
		http.MethodPost,
		"/api/admin/providers/auth-and-create/azure-openai",
		`{"apiKey":"azure-secret"}`,
	)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", recorder.Code, recorder.Body.String())
	}

	var payload struct {
		Success  bool `json:"success"`
		Provider struct {
			ID         string `json:"id"`
			Type       string `json:"type"`
			AuthStatus string `json:"authStatus"`
		} `json:"provider"`
	}
	payload = decodeJSONBody[struct {
		Success  bool `json:"success"`
		Provider struct {
			ID         string `json:"id"`
			Type       string `json:"type"`
			AuthStatus string `json:"authStatus"`
		} `json:"provider"`
	}](t, recorder)

	if !payload.Success {
		t.Fatal("expected success=true")
	}
	if payload.Provider.ID != "azure-openai" {
		t.Fatalf("expected canonical instance id azure-openai, got %q", payload.Provider.ID)
	}
	if payload.Provider.Type != "azure-openai" {
		t.Fatalf("expected provider type azure-openai, got %q", payload.Provider.Type)
	}
	if payload.Provider.AuthStatus != "authenticated" {
		t.Fatalf("expected authenticated status, got %q", payload.Provider.AuthStatus)
	}

	reg := registry.GetProviderRegistry()
	if !reg.IsRegistered("azure-openai") {
		t.Fatal("expected azure-openai provider to be registered")
	}

	record, err := database.NewTokenStore().Get("azure-openai")
	if err != nil {
		t.Fatalf("failed to read saved token: %v", err)
	}
	if record == nil || !strings.Contains(record.TokenData, "azure-secret") {
		t.Fatalf("expected saved azure token, got %#v", record)
	}
}

func TestHandleAuthAndCreateProviderAlibabaAPIKeyUsesCanonicalID(t *testing.T) {
	resetAdminTestState(t)
	t.Cleanup(func() { resetAdminTestState(t) })

	router := newAdminTestRouter()

	recorder := performJSONRequest(
		t,
		router,
		http.MethodPost,
		"/api/admin/providers/auth-and-create/alibaba",
		`{"method":"api-key","plan":"standard","apiKey":"sk-alibaba-abcdef123456","region":"global"}`,
	)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", recorder.Code, recorder.Body.String())
	}

	var payload struct {
		Success  bool `json:"success"`
		Provider struct {
			ID         string `json:"id"`
			Type       string `json:"type"`
			AuthStatus string `json:"authStatus"`
		} `json:"provider"`
	}
	payload = decodeJSONBody[struct {
		Success  bool `json:"success"`
		Provider struct {
			ID         string `json:"id"`
			Type       string `json:"type"`
			AuthStatus string `json:"authStatus"`
		} `json:"provider"`
	}](t, recorder)

	const expectedID = "alibaba-standard-global-123456"

	if !payload.Success {
		t.Fatal("expected success=true")
	}
	if payload.Provider.ID != expectedID {
		t.Fatalf("expected canonical instance id %q, got %q", expectedID, payload.Provider.ID)
	}
	if payload.Provider.Type != "alibaba" {
		t.Fatalf("expected provider type alibaba, got %q", payload.Provider.Type)
	}
	if payload.Provider.AuthStatus != "authenticated" {
		t.Fatalf("expected authenticated status, got %q", payload.Provider.AuthStatus)
	}

	reg := registry.GetProviderRegistry()
	if !reg.IsRegistered(expectedID) {
		t.Fatalf("expected provider %q to be registered", expectedID)
	}
	if reg.IsRegistered("alibaba") {
		t.Fatal("did not expect placeholder alibaba instance to be registered")
	}

	tokenRecord, err := database.NewTokenStore().Get(expectedID)
	if err != nil {
		t.Fatalf("failed to read token record: %v", err)
	}
	if tokenRecord == nil || !strings.Contains(tokenRecord.TokenData, "abcdef123456") {
		t.Fatalf("expected saved alibaba token, got %#v", tokenRecord)
	}

	configRecord, err := database.NewProviderConfigStore().Get(expectedID)
	if err != nil {
		t.Fatalf("failed to read provider config: %v", err)
	}
	if configRecord == nil || !strings.Contains(configRecord.ConfigData, `"region":"global"`) || !strings.Contains(configRecord.ConfigData, `"plan":"standard"`) {
		t.Fatalf("expected saved region config, got %#v", configRecord)
	}
}

func TestHandleAuthAndCreateProviderAlibabaOAuthReturnsError(t *testing.T) {
	resetAdminTestState(t)
	t.Cleanup(func() { resetAdminTestState(t) })

	router := newAdminTestRouter()

	recorder := performJSONRequest(
		t,
		router,
		http.MethodPost,
		"/api/admin/providers/auth-and-create/alibaba",
		`{"method":"oauth"}`,
	)

	if recorder.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", recorder.Code, recorder.Body.String())
	}

	var payload struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
	payload = decodeJSONBody[struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}](t, recorder)

	if payload.Success {
		t.Fatal("expected success=false for unsupported OAuth")
	}
	if payload.Message == "" {
		t.Fatal("expected error message")
	}
}

func TestHandleAuthAndCreateProviderGitHubCopilotTokenUsesCanonicalID(t *testing.T) {
	resetAdminTestState(t)
	t.Cleanup(func() { resetAdminTestState(t) })

	stubDefaultTransport(t, func(req *http.Request) (*http.Response, error) {
		switch {
		case req.URL.Host == "api.github.com" && req.URL.Path == "/copilot_internal/v2/token":
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body: io.NopCloser(strings.NewReader(
					`{"token":"copilot-access-token","expires_at":4102444800}`,
				)),
				Request: req,
			}, nil
		case req.URL.Host == "api.github.com" && req.URL.Path == "/user":
			return &http.Response{
				StatusCode: http.StatusOK,
				Header:     make(http.Header),
				Body:       io.NopCloser(strings.NewReader(`{"login":"octocat"}`)),
				Request:    req,
			}, nil
		default:
			return nil, context.Canceled
		}
	})

	router := newAdminTestRouter()

	recorder := performJSONRequest(
		t,
		router,
		http.MethodPost,
		"/api/admin/providers/auth-and-create/github-copilot",
		`{"method":"token","token":"gh-test-token"}`,
	)

	if recorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", recorder.Code, recorder.Body.String())
	}

	var payload struct {
		Success  bool `json:"success"`
		Provider struct {
			ID         string `json:"id"`
			Name       string `json:"name"`
			Type       string `json:"type"`
			AuthStatus string `json:"authStatus"`
		} `json:"provider"`
	}
	payload = decodeJSONBody[struct {
		Success  bool `json:"success"`
		Provider struct {
			ID         string `json:"id"`
			Name       string `json:"name"`
			Type       string `json:"type"`
			AuthStatus string `json:"authStatus"`
		} `json:"provider"`
	}](t, recorder)

	if !payload.Success {
		t.Fatal("expected success=true")
	}
	const expectedID = "github-copilot-octocat"
	if payload.Provider.ID != expectedID {
		t.Fatalf("expected canonical instance id %q, got %q", expectedID, payload.Provider.ID)
	}
	if payload.Provider.Type != "github-copilot" {
		t.Fatalf("expected provider type github-copilot, got %q", payload.Provider.Type)
	}
	if payload.Provider.Name != "GitHub Copilot (octocat)" {
		t.Fatalf("unexpected provider name: %q", payload.Provider.Name)
	}
	if payload.Provider.AuthStatus != "authenticated" {
		t.Fatalf("expected authenticated status, got %q", payload.Provider.AuthStatus)
	}

	tokenRecord, err := database.NewTokenStore().Get(expectedID)
	if err != nil {
		t.Fatalf("failed to read saved token: %v", err)
	}
	if tokenRecord == nil {
		t.Fatal("expected token record to be saved")
	}
	if !strings.Contains(tokenRecord.TokenData, `"github_token":"gh-test-token"`) {
		t.Fatalf("expected github token in saved record, got %s", tokenRecord.TokenData)
	}
	if !strings.Contains(tokenRecord.TokenData, `"copilot_token":"copilot-access-token"`) {
		t.Fatalf("expected copilot token in saved record, got %s", tokenRecord.TokenData)
	}
}

func TestHandleCancelAuthCancelsInProgressFlowsOnly(t *testing.T) {
	resetAdminTestState(t)
	t.Cleanup(func() { resetAdminTestState(t) })

	router := newAdminTestRouter()

	cancelledPending := false
	activeAuthFlowMu.Lock()
	activeAuthFlow = &authFlowState{
		ProviderID: "copilot-pending",
		Status:     "awaiting_user",
		cancelFn: func() {
			cancelledPending = true
		},
	}
	activeAuthFlowMu.Unlock()

	pendingRecorder := performJSONRequest(
		t,
		router,
		http.MethodPost,
		"/api/admin/auth/cancel",
		`{}`,
	)

	if pendingRecorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", pendingRecorder.Code, pendingRecorder.Body.String())
	}

	var pendingPayload struct {
		Success bool `json:"success"`
	}
	pendingPayload = decodeJSONBody[struct {
		Success bool `json:"success"`
	}](t, pendingRecorder)

	if !pendingPayload.Success {
		t.Fatal("expected pending flow cancellation to succeed")
	}
	if !cancelledPending {
		t.Fatal("expected pending flow cancel function to be called")
	}
	if activeAuthFlow != nil {
		t.Fatal("expected pending flow to be cleared")
	}

	cancelledComplete := false
	activeAuthFlowMu.Lock()
	activeAuthFlow = &authFlowState{
		ProviderID: "copilot-complete",
		Status:     "complete",
		cancelFn: func() {
			cancelledComplete = true
		},
	}
	activeAuthFlowMu.Unlock()

	completeRecorder := performJSONRequest(
		t,
		router,
		http.MethodPost,
		"/api/admin/auth/cancel",
		`{}`,
	)

	if completeRecorder.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", completeRecorder.Code, completeRecorder.Body.String())
	}

	var completePayload struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}
	completePayload = decodeJSONBody[struct {
		Success bool   `json:"success"`
		Message string `json:"message"`
	}](t, completeRecorder)

	if completePayload.Success {
		t.Fatal("expected completed flow cancellation to be a no-op")
	}
	if completePayload.Message != "No active auth flow" {
		t.Fatalf("unexpected complete-flow message: %q", completePayload.Message)
	}
	if cancelledComplete {
		t.Fatal("did not expect complete flow cancel function to be called")
	}
	if activeAuthFlow == nil || activeAuthFlow.ProviderID != "copilot-complete" {
		t.Fatal("expected completed flow to remain available for final polling")
	}
}

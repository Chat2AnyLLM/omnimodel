# Adding a New Provider to OmniLLM

This guide walks through the full process of adding a new LLM provider to OmniLLM — from backend implementation to frontend integration.

## Overview

A provider in OmniLLM consists of:

1. **Backend package** (`internal/providers/<name>/`) — implements the `types.Provider` interface (auth, models, config) and optionally a `types.ProviderAdapter` (request execution).
2. **Route wiring** (`internal/routes/admin_providers.go`) — adds cases to the provider creation/auth handlers.
3. **Frontend UI** (`frontend/src/pages/ProvidersPage.tsx`) — adds the provider type to the registry and an auth form.
4. **Config normalization** (`internal/routes/admin_provider_config.go`) — maps config keys between frontend and storage formats.

---

## Step 0: Ask the User About the Backend API Shape

Before writing any code, clarify the provider's API shape. This determines the adapter implementation and how requests are translated.

Key questions to ask:

- **API protocol**: Does it speak the OpenAI chat completions wire format (`/chat/completions`)? Anthropic's Messages API (`/v1/messages`)? A custom protocol?
- **Authentication**: API key (Bearer token)? OAuth (device code flow, client credentials)? No auth (open endpoints)?
- **Model list**: Static (hardcoded catalog) or dynamic (fetch from `/models` or similar endpoint)?
- **Streaming**: Does the API support SSE streaming? Is the format standard OpenAI-style or custom?
- **Model remapping**: Does the provider use different model names than the canonical ones (e.g., Azure deployment names, Antigravity model aliases)?
- **Base URL**: Fixed or user-configurable? If user-configurable, should local/private endpoints be allowed?

If the provider speaks the **OpenAI chat completions format**, you can reuse `internal/providers/openaicompat` for HTTP execution and only implement the config/auth layer. If it uses a custom format, you'll need a full adapter.

---

## Step 1: Backend Provider Package

Create `internal/providers/<name>/` with the following files:

### 1a. `provider.go` — Provider struct and interface implementation

Implement the `types.Provider` interface (defined in `internal/providers/types/types.go`):

```go
package <name>

import (
    "omnillm/internal/providers/types"
)

type Provider struct {
    instanceID   string
    name         string
    token        string
    config       map[string]interface{}
    configLoaded bool
}

func NewProvider(instanceID, name string) *Provider {
    return &Provider{
        instanceID: instanceID,
        name:       name,
    }
}

// ── Identity ──────────────────────────────────────────────
func (p *Provider) GetID() string         { return "<provider-type>" }
func (p *Provider) GetInstanceID() string { return p.instanceID }
func (p *Provider) GetName() string       { return p.name }

// ── Auth ──────────────────────────────────────────────────
func (p *Provider) SetupAuth(options *types.AuthOptions) error {
    // 1. Validate required fields
    // 2. Persist token via database.NewTokenStore().Save(...)
    // 3. Persist config via database.NewProviderConfigStore().Save(...)
    // 4. Set in-memory fields (p.token, p.config, etc.)
    return nil
}

func (p *Provider) GetToken() string    { return p.token }
func (p *Provider) RefreshToken() error { return nil } // No-op for API keys

// ── Config ────────────────────────────────────────────────
func (p *Provider) GetBaseURL() string {
    p.ensureConfig()
    return p.baseURL
}

func (p *Provider) GetHeaders(forVision bool) map[string]string {
    return map[string]string{
        "Authorization": "Bearer " + p.token,
        "Content-Type":  "application/json",
    }
}

// ── Models ────────────────────────────────────────────────
func (p *Provider) GetModels() (*types.ModelsResponse, error) {
    // Return static model list or fetch from remote API
    // See: openaicompatprovider.fetchModels, antigravity's static catalog
}

// ── Adapter ───────────────────────────────────────────────
func (p *Provider) GetAdapter() types.ProviderAdapter {
    return &Adapter{provider: p}
}

// ── Config loading ────────────────────────────────────────
func (p *Provider) ensureConfig() {
    if p.configLoaded { return }
    // Load from database.NewProviderConfigStore().Get(p.instanceID)
    // and database.NewTokenStore().Get(p.instanceID)
}

// ── Legacy stubs ──────────────────────────────────────────
func (p *Provider) CreateChatCompletions(...) (...) { return nil, fmt.Errorf("use adapter") }
func (p *Provider) CreateEmbeddings(...) (...)      { return nil, fmt.Errorf("not implemented") }
func (p *Provider) GetUsage() (...)                 { return map[string]interface{}{}, nil }
```

### 1b. `adapter.go` — Request execution (if not OpenAI-compatible)

Implement `types.ProviderAdapter`:

```go
type Adapter struct {
    provider *Provider
}

func (a *Adapter) GetProvider() types.Provider { return a.provider }

func (a *Adapter) RemapModel(canonicalModel string) string {
    // Map canonical model names to provider-specific names
    // No-op passthrough is fine for most providers
    return strings.TrimSpace(canonicalModel)
}

func (a *Adapter) Execute(ctx context.Context, request *cif.CanonicalRequest) (*cif.CanonicalResponse, error) {
    // 1. Build provider-specific payload from request
    // 2. Send HTTP request
    // 3. Parse response into cif.CanonicalResponse
}

func (a *Adapter) ExecuteStream(ctx context.Context, request *cif.CanonicalRequest) (<-chan cif.CIFStreamEvent, error) {
    // 1. Build streaming payload
    // 2. Send HTTP request with SSE support
    // 3. Parse SSE stream into cif.CIFStreamEvent channel
}
```

**If the provider speaks OpenAI chat completions format**, you can skip writing a custom adapter and instead delegate to `internal/providers/openaicompat`:

```go
func (a *Adapter) Execute(ctx context.Context, request *cif.CanonicalRequest) (*cif.CanonicalResponse, error) {
    cr, err := openaicompat.BuildChatRequest(a.RemapModel(request.Model), request, false, openaicompat.Config{})
    if err != nil { return nil, err }
    return openaicompat.Execute(ctx, chatURL, headers, cr)
}
```

### 1c. `auth.go` — Auth helpers (optional)

Extract credential validation and persistence into a shared helper if the logic is complex. See `internal/providers/openaicompatprovider/provider.go` (`SetupProviderAuth`) for a reference pattern.

### 1d. `models.go` — Model catalog (optional)

For providers with a static model list:

```go
var Models = []types.Model{
    {ID: "model-a", Name: "Model A"},
    {ID: "model-b", Name: "Model B"},
}
```

### 1e. `http.go` / `stream.go` — HTTP helpers (optional)

Extract HTTP client calls and SSE parsing into separate files if the adapter is non-trivial.

---

## Step 2: Wire Backend Routes

### 2a. Add provider type constant (optional)

In `internal/providers/types/types.go`:

```go
const Provider<Name> ProviderID = "<provider-type>"
```

### 2b. Add auth-and-create handler

In `internal/routes/admin_providers.go`, add a case to `handleAuthAndCreateProvider`:

```go
// ——————————————————————————————————————————————————————————————
case "<provider-type>":
    // Derive canonical instance ID (see openaicompatprovider.CanonicalInstanceID
    // or copilot's deriveGitHubCopilotID for patterns)
    instanceID := deriveCanonicalID(req)
    prov := <name>pkg.NewProvider(instanceID, "")

    if err := prov.SetupAuth(&req); err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{
            "success": false,
            "message": fmt.Sprintf("Authentication failed: %v", err),
        })
        return
    }

    if err := providerRegistry.Register(prov, true); err != nil {
        c.JSON(http.StatusInternalServerError, gin.H{
            "success": false,
            "message": fmt.Sprintf("Failed to register provider: %v", err),
        })
        return
    }

    c.JSON(http.StatusOK, gin.H{
        "success": true,
        "provider": gin.H{
            "id":         prov.GetInstanceID(),
            "type":       prov.GetID(),
            "name":       prov.GetName(),
            "isActive":   false,
            "authStatus": "authenticated",
        },
    })
```

For **OAuth/device-code flows** (like GitHub Copilot), the handler needs to:
1. Initiate the device code flow
2. Return `{ requiresAuth: true, user_code, verification_uri, pending_id }` to the frontend
3. Poll for completion in a background goroutine

### 2c. Add config normalization (if the provider has custom config fields)

In `internal/routes/admin_provider_config.go`, add cases to:

- `normalizeProviderConfigForFrontend` — maps storage keys (snake_case) to frontend keys (camelCase)
- `normalizeProviderConfigForStorage` — maps frontend keys to storage keys

See the existing `openai-compatible` and `azure-openai` cases for reference.

---

## Step 3: Frontend Integration

### 3a. Register the provider type

In `frontend/src/pages/providers/constants/providerRegistry.ts`:

```typescript
// Add to PROVIDER_ACCENT
"<provider-type>": "<hex-color>",

// Add icon to PROVIDER_ICONS (SVG component)
"<provider-type>": (<svg>...</svg>),

// Add display name to TYPE_NAMES
"<provider-type>": "<Display Name>",

// Add to PROVIDER_TYPES array
"<provider-type>",
```

### 3b. Add the auth form

In `frontend/src/pages/ProvidersPage.tsx`:

1. **Add an AddFlow form component** (like `AddFlowOpenAICompatibleForm`) that collects the required fields and calls `onSubmit` with the auth options.

2. **Wire it into the AddFlow** — add a case in the AddFlow rendering:

```tsx
{selectedType === "<provider-type>" && (
    <AddFlow<Name>Form {...authFormProps} />
)}
```

3. **If the provider has a detail panel** (for editing config after creation), add model management and config editing UI within the provider detail section (look for the `{provider.type === "openai-compatible" && ...}` blocks for reference).

### 3c. API functions

The frontend API layer (`frontend/src/api/index.ts`) already provides generic functions:

- `authAndCreateProvider(type, body)` — POST `/api/admin/providers/auth-and-create/:type`
- `authProvider(id, body)` — POST `/api/admin/providers/:id/auth`
- `updateProviderConfig(id, config)` — PUT `/api/admin/providers/:id/config`
- `getProviderModels(id)` — GET `/api/admin/providers/:id/models`
- `toggleProviderModel(id, modelId, enabled)` — POST `/api/admin/providers/:id/models/toggle`

These are generic and work for any provider — no changes needed unless you need a custom auth flow (e.g., Antigravity's OAuth has dedicated endpoints).

---

## Step 4: Database Stores

The following stores are available (all in `internal/database/`):

| Store | Purpose | Key Methods |
|-------|---------|-------------|
| `TokenStore` | Persist credentials (API keys, tokens) | `Save(instanceID, providerID, data)`, `Get(instanceID)`, `Delete(instanceID)` |
| `ProviderConfigStore` | Persist provider configuration | `Save(instanceID, config)`, `Get(instanceID)`, `Delete(instanceID)` |
| `ProviderInstanceStore` | Persist provider instance metadata | `Save(record)`, `Get(instanceID)`, `GetAll()`, `Delete(instanceID)` |
| `ModelStateStore` | Per-model enabled/disabled state | `SetEnabled(instanceID, modelID, enabled)`, `GetAllByInstance(instanceID)`, `Delete(instanceID, modelID)` |
| `ModelConfigStore` | Per-model configuration | `GetAllByInstance(instanceID)`, `Delete(instanceID, modelID)` |
| `ProviderModelsCacheStore` | Cache model lists (24h TTL) | `Save(instanceID, modelsJSON)`, `Get(instanceID, ttl)`, `Delete(instanceID)` |

---

## Step 5: Security Considerations

- **Endpoint validation**: If the provider accepts a user-supplied base URL, validate it with `security.ValidateEndpoint(endpoint, allowLocal)`. This blocks localhost/private IPs unless `allowLocal` is true.
- **Token storage**: Tokens are stored in the database via `TokenStore`. Ensure sensitive fields are never logged.
- **Input validation**: Validate all `AuthOptions` fields in `SetupAuth` before persisting.

---

## Step 6: Testing

1. **Backend**: Add unit tests for `SetupAuth`, `GetModels`, and adapter `Execute`/`ExecuteStream`.
2. **Integration**: Add the provider via the UI, verify models load and can be toggled, send a test request.
3. **Persistence**: Restart the server and verify the provider loads from DB (the registry calls `LoadFromDB` at startup for registered providers).

---

## Reference: Existing Provider Patterns

| Provider | Auth | Models | API Shape | Adapter |
|----------|------|--------|-----------|---------|
| **GitHub Copilot** | OAuth device code + token exchange | Remote fetch | Custom (Copilot-specific) | Custom adapter with token refresh |
| **Alibaba (DashScope)** | API key | Static catalog + remote | OpenAI-compatible | Delegates to openaicompat |
| **Azure OpenAI** | API key + endpoint | Config-driven (deployments) | OpenAI-compatible + Responses | Custom RemapModel (model→deployment) |
| **Google Gemini** | API key | Remote fetch | Google-specific | Custom Execute/Stream |
| **Antigravity (Vertex)** | OAuth client credentials | Static catalog | Google-specific | Custom Execute/Stream |
| **OpenAI-Compatible** | API key (optional) | Remote `/models` + user-defined | OpenAI chat/responses | Delegates to openaicompat |

---

## Quick Checklist

- [ ] Asked user about the provider's API shape (protocol, auth, models, streaming)
- [ ] Created `internal/providers/<name>/` with `provider.go`, `adapter.go`, and helpers
- [ ] Implemented `types.Provider` interface (GetID, SetupAuth, GetModels, GetAdapter, etc.)
- [ ] Implemented `types.ProviderAdapter` (Execute, ExecuteStream, RemapModel)
- [ ] Added token/config persistence via `TokenStore` and `ProviderConfigStore`
- [ ] Implemented `LoadFromDB` or `ensureConfig` for startup rehydration
- [ ] Added case to `handleAuthAndCreateProvider` in `admin_providers.go`
- [ ] Added config normalization in `admin_provider_config.go` (if needed)
- [ ] Added provider to frontend registry (`providerRegistry.ts`)
- [ ] Added AddFlow form component in `ProvidersPage.tsx`
- [ ] Verified backend compiles (`go build ./...`)
- [ ] Verified frontend compiles (`bun run tsc --noEmit`)
- [ ] Tested end-to-end: add provider, load models, toggle models, send request

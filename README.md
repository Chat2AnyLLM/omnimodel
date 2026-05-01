<div align="center">

# 🌐 OmniLLM

**Intelligent LLM Router &mdash; Unify every AI provider behind one OpenAI-compatible API.**

*One gateway. Every model. Zero rewrites.*

<br/>

[![Go](https://img.shields.io/badge/Go-1.23-00ADD8?style=for-the-badge&logo=go&logoColor=white)](https://golang.org)
[![Bun](https://img.shields.io/badge/Bun-1.2+-fbf0df?style=for-the-badge&logo=bun&logoColor=black)](https://bun.sh)
[![TypeScript](https://img.shields.io/badge/TypeScript-5.x-3178C6?style=for-the-badge&logo=typescript&logoColor=white)](https://www.typescriptlang.org)

[![npm version](https://img.shields.io/npm/v/omnillm?style=flat-square&logo=npm&color=CB3837)](https://www.npmjs.com/package/omnillm)
[![Docker](https://img.shields.io/docker/v/zhujian0805/omnillm?style=flat-square&logo=docker&label=docker&color=2496ED)](https://hub.docker.com/r/zhujian0805/omnillm)
[![License: MIT](https://img.shields.io/github/license/OmniLLM/omnillm?style=flat-square&color=green)](LICENSE)
[![Stars](https://img.shields.io/github/stars/OmniLLM/omnillm?style=flat-square&logo=github)](https://github.com/OmniLLM/omnillm/stargazers)
[![Last Commit](https://img.shields.io/github/last-commit/OmniLLM/omnillm?style=flat-square&logo=github)](https://github.com/OmniLLM/omnillm/commits)
[![PRs Welcome](https://img.shields.io/badge/PRs-welcome-brightgreen?style=flat-square)](https://github.com/OmniLLM/omnillm/pulls)

<br/>

[🇨🇳 中文文档](README.zh-CN.md) &nbsp;|&nbsp;
[✨ Features](#-features) &nbsp;|&nbsp;
[🚀 Quick Start](#-quick-start) &nbsp;|&nbsp;
[📦 Deployment](#-deployment) &nbsp;|&nbsp;
[⚙️ Config](#%EF%B8%8F-configuration) &nbsp;|&nbsp;
[🔌 Providers](#-supported-providers) &nbsp;|&nbsp;
[🏗 Architecture](#-architecture) &nbsp;|&nbsp;
[📡 API](#-api-reference) &nbsp;|&nbsp;
[❓ FAQ](#-faq)

</div>

---

## 🤔 What is OmniLLM?

OmniLLM is a unified control plane and API gateway for LLM model access. It sits between your applications, agents, and coding tools, and upstream LLM providers, translating requests through a **Canonical Intermediate Format (CIF)** so any client can talk to any provider regardless of API shape.

> 💡 **TL;DR** &mdash; Point `OPENAI_BASE_URL` or `ANTHROPIC_BASE_URL` at OmniLLM and your existing code works with **every** supported provider, with automatic failover, virtual models, metering, and a full admin console.

### Why OmniLLM?

| Problem | OmniLLM Solution |
|---|---|
| Different providers have different APIs | One unified OpenAI/Anthropic-compatible endpoint |
| Provider outages break your app | Automatic priority-based failover across providers |
| Juggling API keys for every tool | Single gateway key; rotate upstream keys in one place |
| No visibility into model costs/usage | Per-request metering, token counts, latency tracking |
| Manually configuring every AI coding tool | ToolConfig UI manages Claude Code, Codex, Droid & more |

---

## 📸 Screenshots

![OmniLLM Admin Console](docs/assets/admin-console.png)

<details>
<summary>💬 Chat Interface &nbsp;|&nbsp; 🔀 Virtual Models &nbsp;|&nbsp; 🔧 ToolConfig</summary>

### Chat

![Chat Interface](docs/assets/chat.png)

### Virtual Models

![Virtual Models](docs/assets/virtual-models.png)

### ToolConfig

![ToolConfig Interface](docs/assets/toolconfig.png)

</details>

---

## ✨ Features

<table>
<tr>
<td width="50%">

🔗 **Unified API**  
OpenAI & Anthropic-compatible endpoints from a single gateway — no client rewrites needed.

🔄 **Priority-Based Failover**  
Automatically retries across providers when one fails mid-request.

🎛 **Virtual Models**  
Abstract model IDs with round-robin, random, priority, or weighted load-balancing.

🧩 **CIF Translation**  
Canonical Intermediate Format eliminates pairwise format translations.

</td>
<td width="50%">

🔌 **7+ Provider Types**  
GitHub Copilot, Alibaba DashScope, Azure OpenAI, Google, Kimi, Antigravity, and any OpenAI-compatible endpoint (Ollama, vLLM, LM Studio, llama.cpp).

📊 **Metering & Logging**  
Per-request token counts, latency, provider attribution, client tracking, and live log streaming via SSE/WebSocket.

🛡 **Security First**  
API key auth, SSRF protection, CORS restrictions, token masking, config editing guard.

🔧 **ToolConfig**  
Manage config files for Claude Code, Codex, Droid, OpenCode, and AMP from a single admin UI.

</td>
</tr>
</table>

Additional capabilities:
- **Streaming Resilience** &mdash; Auto-retries failed SSE streams as non-streaming requests and re-streams locally
- **Admin Console** &mdash; Provider switching, model discovery, log streaming, config editing, and virtual model management

---

## 🚀 Quick Start

> **One-line install** &mdash; No config needed. Just run and go.

```sh
bunx omnillm@latest start
```

Then open `http://localhost:4141/admin/` in your browser.

### Prerequisites (for development)

- [Bun](https://bun.sh) >= 1.2
- [Go](https://golang.org) >= 1.22

### Development mode

```sh
bun run dev
```

Starts both backend (Go, port 5000) and frontend (Vite, port 5080). Admin UI at `http://localhost:5080/admin/`.

### API key setup

All API routes are protected by an API key. On first start, OmniLLM auto-generates a random key and persists it to `~/.config/omnillm/api-key`.

```sh
# Option A: Set a known key (recommended)
bun run omni start --api-key my-secret-key

# Option B: Environment variable
OMNILLM_API_KEY=my-secret-key bun run omni start

# Option C: Use the auto-generated key
bun run omni start
cat ~/.config/omnillm/api-key
```

<details>
<summary>Windows (PowerShell) examples</summary>

```powershell
# Set key via environment variable
$env:OMNILLM_API_KEY = "my-secret-key"
bun run omni restart --rebuild --host localhost --server-port 5000 --frontend-port 5080 -v

# Or pre-create the key file
New-Item -ItemType Directory -Force -Path "$env:USERPROFILE\.config\omnillm"
"my-secret-key" | Set-Content -NoNewline -Path "$env:USERPROFILE\.config\omnillm\api-key"

# Read the auto-generated key
cat "$env:USERPROFILE\.config\omnillm\api-key"
```

</details>

Include the key in API requests:

```sh
curl -H "Authorization: Bearer <your-api-key>" http://localhost:5000/v1/models
curl -H "x-api-key: <your-api-key>" http://localhost:5000/v1/models
```

The admin UI automatically injects the key via a `<meta>` tag, so no manual auth is needed in the browser.

### Background mode

```sh
bun run omni start          # start both services
bun run omni status         # check status
bun run omni stop           # stop all services
bun run omni restart        # restart services
bun run omni restart --rebuild --host localhost  # rebuild + restart
```

---

## 📦 Deployment

### bunx (quickest)

```sh
bunx omnillm@latest start
```

### Docker

```sh
docker build -t omnillm .
docker run -p 4141:4141 -v $(pwd)/proxy-data:/root/.local/share/omnillm omnillm
```

Set a known API key:

```sh
docker run -p 4141:4141 -e OMNILLM_API_KEY=my-secret-key omnillm
```

### Docker Compose

```yaml
version: "3.8"
services:
  omnillm:
    build: .
    ports:
      - "4141:4141"
    volumes:
      - omnillm-data:/root/.local/share/omnillm
    environment:
      - OMNILLM_API_KEY=my-secret-key
    restart: unless-stopped

volumes:
  omnillm-data:
```

```sh
docker compose up -d
```

### Manual build

```sh
bun install
bun run build        # builds Go binary + frontend
bun run omni start   # starts the gateway
```

---

## ⚙️ Configuration

### Environment variables & CLI flags

| Environment Variable | CLI Flag | Default | Description |
|---|---|---|---|
| `OMNILLM_API_KEY` | `--api-key` | auto-generated | Inbound API key for route protection |
| `OMNILLM_PORT` | `--port`, `-p` | `5000` | Listening port |
| — | `--provider` | `github-copilot` | Default active provider on startup |
| — | `--verbose`, `-v` | `false` | Enable verbose logging |
| — | `--account-type`, `-a` | `individual` | Copilot plan (`individual`, `business`, `enterprise`) |
| — | `--rate-limit`, `-r` | none | Minimum seconds between requests |
| — | `--wait`, `-w` | `false` | Queue instead of failing on rate limit |
| — | `--manual` | `false` | Require manual approval per request |
| — | `--github-token`, `-g` | none | Provide GitHub token directly |
| — | `--claude-code`, `-c` | `false` | Guided Claude Code setup |
| — | `--show-token` | `false` | Expose full tokens via `/token` endpoint |
| — | `--proxy-env` | `false` | Read proxy settings from environment |
| — | `--allow-local-endpoints` | `false` | Allow localhost/private OpenAI-compatible endpoints |
| — | `--enable-config-edit` | `false` | Allow editing external config files via admin API |

### API key resolution order

1. `--api-key` CLI flag
2. `OMNILLM_API_KEY` environment variable
3. Persisted file at `~/.config/omnillm/api-key`
4. Auto-generated random key (persisted to the file above)

---

## 🔌 Supported Providers

| Provider | Auth Method | Notes |
|---|---|---|
| 🐙 **GitHub Copilot** | OAuth device flow or token | Requires active Copilot subscription |
| 🟧 **Alibaba DashScope** | API key | Supports global and China regions; coding plan variant |
| 🤖 **OpenAI-Compatible** | API key (optional) | Any OpenAI-compatible endpoint: Ollama, vLLM, LM Studio, llama.cpp, OpenAI |
| ☁️ **Azure OpenAI** | API key | Configurable endpoint, API version, and deployments |
| 🔵 **Google** | API key | Generic Google provider |
| 🌙 **Kimi** | API key | Generic Kimi provider |
| 🌐 **Antigravity** | Google OAuth | Requires Google OAuth client credentials |

New providers are registered with canonical instance IDs derived from their endpoint URL and API key suffix, ensuring stable identification across restarts.

> 💡 **Adding a new provider?** See [docs/ADDING_A_PROVIDER.md](docs/ADDING_A_PROVIDER.md) — only two CIF adapters are needed.

---

## 🏗 Architecture

```
Clients / Agents / Internal Apps
            |
            v
        OmniLLM Gateway
   ┌─────────────────────────────────────────┐
   │  Inbound API Key Auth                   │
   │  (Bearer / x-api-key / SSE query)       │
   ├─────────────────────────────────────────┤
   │  Ingestion Layer                        │
   │  OpenAI format ──┐                      │
   │  Anthropic format├─► CIF (Canonical     │
   │  Responses API  ─┘   Intermediate       │
   │                      Format)            │
   ├─────────────────────────────────────────┤
   │  Model Routing + Priority Failover      │
   │  Virtual Model Resolution               │
   │  Rate Limiting + Manual Approval        │
   ├─────────────────────────────────────────┤
   │  Provider CIF Adapters                  │
   │  (Execute / ExecuteStream)              │
   ├─────────────────────────────────────────┤
   │  Serialization Layer                    │
   │  CIF ──► OpenAI format / Anthropic      │
   │         format (SSE or non-streaming)   │
   ├─────────────────────────────────────────┤
   │  Admin Console + SSE/WS Log Streaming   │
   │  SQLite Persistence (provider, tokens,  │
   │  configs, chat sessions, virtual models)│
   └─────────────────────────────────────────┘
            |
            v
  GitHub Copilot / Alibaba DashScope / Azure OpenAI
  / Google / Kimi / Antigravity / Ollama / vLLM
  / OpenAI / any OpenAI-compatible endpoint
```

<details>
<summary>Key components</summary>

| Package | Path | Purpose |
|---|---|---|
| **Server** | `internal/server/` | Gin HTTP server, route registration, CORS, auth middleware, admin UI serving, SSE/WebSocket log streaming |
| **Routes** | `internal/routes/` | HTTP handlers for chat, models, embeddings, messages, responses, usage, token, admin, config files, virtual models |
| **Ingestion** | `internal/ingestion/` | Parses incoming OpenAI/Anthropic/Responses requests into `cif.CanonicalRequest` with fail-fast validation |
| **CIF** | `internal/cif/` | Canonical Intermediate Format — the normalized request/response data model all providers translate to/from |
| **Serialization** | `internal/serialization/` | Converts CIF responses back to the client's expected API format (OpenAI SSE, Anthropic SSE, non-streaming JSON) |
| **Providers** | `internal/providers/` | Per-provider implementations (Copilot, Alibaba, Azure, Google, Kimi, Antigravity, OpenAI-Compatible, Generic) |
| **Registry** | `internal/registry/` | Thread-safe `ProviderRegistry` — manages registered providers, active provider selection, failover |
| **Model Routing** | `internal/lib/modelrouting/` | Resolves model names to candidate providers with caching |
| **Virtual Model Routing** | `internal/lib/virtualmodelrouting/` | Routes abstract virtual model IDs to specific provider models with load-balancing strategies |
| **Database** | `internal/database/` | SQLite persistence via `modernc.org/sqlite` (pure Go, no CGO) — provider instances, tokens, configs, chat sessions, virtual models |
| **Security** | `internal/security/` | SSRF protection for OpenAI-compatible endpoints |
| **Rate Limiting** | `internal/lib/ratelimit/` | Configurable rate limiter with optional queue-on-reject behavior |
| **Approval** | `internal/lib/approval/` | Manual request approval mode (`--manual` flag) |
| **GitHub Service** | `internal/services/github/` | GitHub Copilot-specific logic (token refresh, usage, quota) |
| **Frontend** | `frontend/` | React 19 + Vite + MUI/Tailwind v4 admin console |

</details>

<details>
<summary>Request flow</summary>

1. Client sends request (e.g., `POST /v1/chat/completions`) to OmniLLM
2. Auth middleware validates inbound API key (Bearer / x-api-key / query param)
3. Logging middleware generates request ID, captures client IP and User-Agent
4. Rate limiter checks throttling; if `--manual` mode, prompts for operator approval
5. Ingestion parser deserializes body into `cif.CanonicalRequest`
6. Model routing normalizes model name, resolves candidate providers from registry
7. Provider iteration loops through candidates in priority order:
   - Gets provider's CIF adapter via `GetAdapter()`
   - Remaps model name to provider's internal naming
   - Calls `Execute()` (non-streaming) or `ExecuteStream()` (streaming, returns Go channel)
   - On failure, tries next candidate (automatic failover)
8. Serialization converts CIF response back to client's expected API format
9. Response sent back with structured logging of the full lifecycle

</details>

### Tech Stack

| Layer | Technologies |
|---|---|
| **Backend** | Go 1.23, Gin, zerolog, Cobra CLI, modernc.org/sqlite (pure Go SQLite) |
| **Frontend** | React 19, Vite, MUI v7, Tailwind v4, Radix UI, Lucide icons, Sonner, TypeScript |
| **Tooling** | Bun (runtime + package manager), ESLint, simple-git-hooks + lint-staged |
| **Deployment** | Multi-stage Dockerfile (`golang:1.23-alpine` &rarr; `oven/bun:1.2.19-alpine` &rarr; `alpine:3.20`), standalone binary, or `bunx omnillm@latest` |

---

## 🔒 Security

| Control | Description |
|---|---|
| **API Key Auth** | All routes protected; supports Bearer, x-api-key header, and query param for SSE |
| **SSRF Protection** | Rejects localhost/private/link-local addresses unless `--allow-local-endpoints` is set |
| **CORS** | Restricted to `localhost`, `127.0.0.1`, and `::1` origins |
| **Token Masking** | `/token` endpoint returns masked tokens by default (first 4 + last 4 visible) |
| **Config Editing Guard** | External config editing disabled by default; requires `--enable-config-edit` |

<details>
<summary>Production recommendations</summary>

- Run behind an internal reverse proxy or API gateway
- Restrict admin access to trusted operators
- Isolate persistent state with controlled filesystem permissions
- Avoid exposing diagnostic/token endpoints on public networks
- Review each provider's contractual and compliance posture before shared organizational use

</details>

---

## 📡 API Reference

### OpenAI-compatible endpoints

| Method | Endpoint | Description |
|---|---|---|
| `POST` | `/v1/chat/completions` | Chat completions |
| `GET` | `/v1/models` | List available models |
| `POST` | `/v1/embeddings` | Text embeddings |
| `POST` | `/v1/responses` | Responses API |

### Anthropic-compatible endpoints

| Method | Endpoint | Description |
|---|---|---|
| `POST` | `/v1/messages` | Messages API |
| `POST` | `/v1/messages/count_tokens` | Token counting |

### Utility endpoints

| Method | Endpoint | Description |
|---|---|---|
| `GET` | `/usage` | Active provider usage and quota |
| `GET` | `/token` | Current provider token (masked by default) |
| `GET` | `/health` | Health check with timestamp |
| `GET` | `/healthz` | Minimal health check |

<details>
<summary>Admin API (full list)</summary>

| Method | Endpoint | Description |
|---|---|---|
| `GET` | `/api/admin/info` | Version, port, backend type, uptime |
| `GET` | `/api/admin/status` | Server and provider status |
| `GET` | `/api/admin/providers` | List providers with auth status and model counts |
| `POST` | `/api/admin/providers/switch` | Switch active provider |
| `POST` | `/api/admin/providers/add/:type` | Add a new provider instance |
| `POST` | `/api/admin/providers/auth-and-create/:type` | Authenticate and create provider in one step |
| `DELETE` | `/api/admin/providers/:id` | Delete a provider instance |
| `GET` | `/api/admin/providers/:id/models` | List models for a provider |
| `POST` | `/api/admin/providers/:id/models/refresh` | Force-refresh model list from upstream |
| `POST` | `/api/admin/providers/:id/models/toggle` | Enable/disable a model |
| `GET` | `/api/admin/providers/:id/models/:modelId/version` | Get model version string |
| `PUT` | `/api/admin/providers/:id/models/:modelId/version` | Set model version string |
| `GET` | `/api/admin/providers/:id/usage` | Provider-specific usage |
| `POST` | `/api/admin/providers/:id/auth` | Authenticate a provider |
| `POST` | `/api/admin/providers/:id/auth/initiate-device-code` | Start OAuth device code flow |
| `POST` | `/api/admin/providers/:id/auth/complete-device-code` | Complete OAuth device code flow |
| `PUT` | `/api/admin/providers/:id/config` | Update provider configuration |
| `POST` | `/api/admin/providers/:id/activate` | Activate a provider |
| `POST` | `/api/admin/providers/:id/deactivate` | Deactivate a provider |
| `GET` | `/api/admin/providers/priorities` | Get provider failover priorities |
| `POST` | `/api/admin/providers/priorities` | Set provider failover priorities |
| `GET` | `/api/admin/auth-status` | Current OAuth flow state |
| `POST` | `/api/admin/auth/cancel` | Cancel active OAuth flow |
| `GET` | `/api/admin/settings/log-level` | Get current log level |
| `PUT` | `/api/admin/settings/log-level` | Change log level dynamically |
| `GET` | `/api/admin/logs/stream` | SSE log stream |
| `GET` | `/api/admin/chat/sessions` | List chat sessions |
| `POST` | `/api/admin/chat/sessions` | Create chat session |
| `GET` | `/api/admin/chat/sessions/:id` | Get chat session with messages |
| `PUT` | `/api/admin/chat/sessions/:id` | Update chat session title |
| `POST` | `/api/admin/chat/sessions/:id/messages` | Add message to session |
| `DELETE` | `/api/admin/chat/sessions/:id` | Delete chat session |
| `DELETE` | `/api/admin/chat/sessions` | Delete all chat sessions |
| `GET` | `/api/admin/config` | List available config files |
| `GET` | `/api/admin/config/:name` | Read a config file |
| `PUT` | `/api/admin/config/:name` | Save a config file |
| `POST` | `/api/admin/config/:name/import` | Import config from uploaded file |
| `POST` | `/api/admin/config/:name/backup` | Create timestamped backup in same directory |
| `GET` | `/api/admin/virtualmodels` | List virtual models |
| `POST` | `/api/admin/virtualmodels` | Create virtual model |
| `GET` | `/api/admin/virtualmodels/:id` | Get virtual model detail |
| `PUT` | `/api/admin/virtualmodels/:id` | Update virtual model |
| `DELETE` | `/api/admin/virtualmodels/:id` | Delete virtual model |

</details>

---

## 🔧 ToolConfig &mdash; AI Assistant Configuration Management

Manage configuration files for popular AI coding assistants from the admin UI.

| Tool | Config File | Template | Docs |
|------|-------------|----------|------|
| **[Claude Code](https://claude.ai/code)** | `~/.claude/settings.json` | [Example](.claude/settings.json.example) | [Docs](https://code.claude.com/docs/en/settings) |
| **[Codex](https://github.com/openai/codex)** | `~/.codex/config.toml` | [Example](.codex/config.toml.example) | [Docs](https://developers.openai.com/codex/config-reference) |
| **[Droid](https://docs.factory.ai/cli)** | `~/.factory/settings.json` | [Example](.factory/settings.json.example) | [Docs](https://docs.factory.ai/cli/byok/overview#understanding-providers) |
| **[OpenCode](https://opencode.ai)** | `~/.opencode/config.json` | [Example](.opencode/config.json.example) | [Docs](https://opencode.ai/docs/config/) |
| **[AMP](https://ampcode.com)** | `~/.amp/config.json` | [Example](.amp/config.json.example) | [Docs](https://ampcode.com/manual#configuration) |

**Features:** Structured editors, one-click backup, template files, real-time validation, auto-create, import/export.

> Config file editing is **disabled by default** for security. Enable with `--enable-config-edit`.

---

## 🤖 Claude Code Integration

### Guided setup

```sh
bun run start --claude-code
```

### Manual `.claude/settings.json`

```json
{
  "env": {
    "ANTHROPIC_BASE_URL": "http://localhost:5000",
    "ANTHROPIC_AUTH_TOKEN": "dummy",
    "ANTHROPIC_MODEL": "gpt-4.1",
    "ANTHROPIC_DEFAULT_SONNET_MODEL": "gpt-4.1",
    "ANTHROPIC_SMALL_FAST_MODEL": "gpt-4.1",
    "ANTHROPIC_DEFAULT_HAIKU_MODEL": "gpt-4.1",
    "DISABLE_NON_ESSENTIAL_MODEL_CALLS": "1",
    "CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC": "1"
  }
}
```

<details>
<summary>Cost optimization with GitHub Copilot upstream</summary>

GitHub Copilot charges per request, not per token. Claude Code makes many small background calls that add up. Override small/fast models to a free or low-cost model:

```json
{
  "env": {
    "ANTHROPIC_BASE_URL": "http://localhost:5000",
    "ANTHROPIC_MODEL": "claude-haiku-4.5",
    "ANTHROPIC_DEFAULT_HAIKU_MODEL": "qwen3.6-plus",
    "ANTHROPIC_SMALL_FAST_MODEL": "qwen3.6-plus",
    "CLAUDE_CODE_SUBAGENT_MODEL": "qwen3.6-plus",
    "CLAUDE_CODE_DISABLE_NONESSENTIAL_TRAFFIC": "1"
  }
}
```

</details>

---

## 📋 CLI Reference

| Command | Purpose |
|---|---|
| `start` | Start the OmniLLM gateway |
| `auth` | Authenticate providers without starting the server |
| `check-usage` | Print GitHub Copilot usage/quota information |
| `debug` | Print runtime, version, and path diagnostics |
| `sync-names` | Refresh provider display names from live API metadata |
| `provider` | Manage LLM provider instances (list, add, delete, activate, deactivate, switch, rename, priorities, usage) |
| `model` | Manage models for a provider (list, refresh, toggle, version) |
| `virtualmodel` | Manage virtual model aliases with load-balancing (list, get, create, update, delete) |
| `config` | Manage external tool config files (list, get, set, import, backup) |
| `settings` | View and update server settings (get, set) |
| `status` | Show server and auth status |
| `chat` | Interactive chat REPL or manage saved chat sessions |
| `logs` | Stream or view server logs (tail) |

---

## ❓ FAQ

| Question | Answer |
|---|---|
| **No authenticated providers** | Run `omnillm auth` or authenticate in the admin UI |
| **401 / Unauthorized** | Re-authenticate the provider; include `Authorization: Bearer <api-key>` in requests |
| **No models returned** | Provider auth may be incomplete or upstream is unavailable |
| **Rate-limit failures** | Increase `--rate-limit` interval or use `--wait` to queue |
| **Endpoint rejected** | SSRF protection is blocking localhost/private IPs; use `--allow-local-endpoints` |
| **How do I use Ollama locally?** | Add an OpenAI-compatible provider with `http://localhost:11434` and `--allow-local-endpoints` |
| **Can I use multiple providers at once?** | Yes. Set priorities via admin UI or `omnillm provider priorities`. Failover is automatic. |
| **How do virtual models work?** | Define an abstract model ID that maps to one or more provider models with load-balancing (round-robin, random, priority, weighted) |
| **Where is data stored?** | SQLite database at `~/.local/share/omnillm/omnillm.db` (or Docker volume). API key at `~/.config/omnillm/api-key`. |
| **How do I reset everything?** | Stop the server and delete `~/.local/share/omnillm/` and `~/.config/omnillm/` |

---

## 🛠 Development

```sh
bun install

bun run dev                    # backend + frontend (foreground)
bun run dev --server-port 8080 # custom ports

bun run omni start             # background mode
bun run omni stop

bun run dev:server             # backend only
bun run dev:frontend           # frontend only

bun run build                  # production build
bun run typecheck              # type check
```

Frontend source lives in `frontend/` and uses Vite + React + Tailwind v4. In development mode, Vite proxies `/api/*`, `/v1/*`, and `/usage` to the Go backend. The frontend auto-detects the backend port at runtime.

---

## 🤝 Contributing

Contributions are welcome! Here's how to get started:

1. **Fork** the repository and create your branch from `main`
2. **Install** dependencies: `bun install`
3. **Make** your changes, following the code style (run `bun run lint` to check)
4. **Test** your changes: `bun test`
5. **Submit** a pull request — please describe what you changed and why

> For adding new LLM providers, see [docs/ADDING_A_PROVIDER.md](docs/ADDING_A_PROVIDER.md).

[![PRs Welcome](https://img.shields.io/badge/PRs-welcome-brightgreen?style=flat-square)](https://github.com/OmniLLM/omnillm/pulls)

---

## 🌟 Star History

[![Star History Chart](https://api.star-history.com/svg?repos=OmniLLM/omnillm&type=Date)](https://star-history.com/#OmniLLM/omnillm&Date)

---

## 📄 License

[MIT](LICENSE)

---

<div align="center">

Made with ❤️ by the OmniLLM contributors &nbsp;·&nbsp; [⭐ Star us on GitHub](https://github.com/OmniLLM/omnillm)

</div>

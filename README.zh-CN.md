# OmniLLM

<p align="center">统一的 LLM 网关与控制平面，兼容 OpenAI 和 Anthropic 客户端。</p>

<p align="center"><a href="README.md">English</a></p>

OmniLLM 位于应用、智能体、编程工具与上游模型提供商之间。它会先把请求归一化为 CIF（Canonical Intermediate Format，规范中间格式），再做模型解析、提供商选择、故障转移和响应序列化，最后返回客户端期望的 API 形态。

直接效果是：你的客户端只需要接入一次 OmniLLM，就能在不重写业务代码的前提下切换提供商、做故障转移、管理虚拟模型、查看用量与日志，并使用统一的管理后台。

## 二进制入口

当前仓库面向用户的三个主要入口分别是：

| 二进制 | 入口文件 | 作用 |
|---|---|---|
| `omnillm` | `main.go` | 主服务与管理 CLI。用于启动网关、管理提供商、模型、虚拟模型、聊天、设置、配置文件和日志。 |
| `omniproxy` | `cmd/omniproxy/main.go` | 面向代理场景的入口，命令能力与 OmniLLM 服务端流程保持一致。 |
| `omnicode` | `cmd/omnicode/main.go` | 面向编程场景的交互式聊天与 agent CLI，基于 OmniLLM 的会话与 API 工作。 |

如果你只需要网关本身，优先使用 `omnillm`。如果你的部署或脚本希望使用代理命名，则使用 `omniproxy`。如果你要通过一个交互式 CLI 连到 OmniLLM 做编程助手或会话式调用，则使用 `omnicode`。

## 这个项目解决什么问题

常见痛点通常包括：

- 每个提供商都有不同的 API 形态
- 一旦某个提供商失败，客户端就要自己兜底
- 本地或团队工具需要分别管理不同的密钥和配置
- 很难统一查看请求来源、延迟、用量和路由结果

OmniLLM 把这些能力集中到一个入口，提供：

- OpenAI 兼容端点：`/v1/chat/completions`、`/v1/models`、`/v1/embeddings`、`/v1/responses`
- Anthropic 兼容端点：`/v1/messages`、`/v1/messages/count_tokens`
- 提供商优先级和自动故障转移
- 支持轮询、随机、优先级、加权策略的虚拟模型
- 管理后台，用于提供商、日志、聊天会话、计量、访问令牌、设置和 ToolConfig

## 界面截图

![OmniLLM 管理控制台](docs/assets/admin-console.png)

其他页面：

- Chat: ![聊天界面](docs/assets/chat.png)
- 虚拟模型: ![虚拟模型](docs/assets/virtual-models.png)
- ToolConfig: ![ToolConfig](docs/assets/toolconfig.png)

## 快速开始

### 方式一：直接运行已发布包

```sh
bunx omnillm@latest start
```

运行时默认地址：

- API 服务：`http://127.0.0.1:5000`
- 管理后台：`http://127.0.0.1:5000/admin/`

### 方式二：从源码运行

前置要求：

- Bun 1.2+
- Go 1.25+

构建并运行主二进制：

```sh
bun install
bun run build:go
$HOME/.local/bin/omnillm start
```

Windows PowerShell：

```powershell
bun install
bun run build:go
$env:USERPROFILE/.local/bin/omnillm.exe start
```

### 运行 `omniproxy`

仓库也提供一个独立的代理入口：

```sh
go run ./cmd/omniproxy start
```

它适合那些希望使用代理命名，但又不想改变服务端行为的场景。

### 运行 `omnicode`

`omnicode` 会连接到一个已经运行中的 OmniLLM 服务，并复用相同的会话与管理 API：

```sh
go run ./cmd/omnicode --server http://127.0.0.1:5000 --api-key my-secret-key --prompt "Explain this repo"
```

交互式模式：

```sh
go run ./cmd/omnicode --server http://127.0.0.1:5000 --api-key my-secret-key
```

### API 密钥行为

所有 API 和管理路由都受保护。入站密钥解析顺序为：

1. `--api-key`
2. `OMNILLM_API_KEY`
3. `~/.config/omnillm/api-key`
4. 如果都没有，则自动生成并持久化到上述文件

示例：

```sh
omnillm start --api-key my-secret-key
curl -H "Authorization: Bearer my-secret-key" http://127.0.0.1:5000/v1/models
curl -H "x-api-key: my-secret-key" http://127.0.0.1:5000/v1/models
```

Windows PowerShell：

```powershell
$env:OMNILLM_API_KEY = "my-secret-key"
omnillm start
Invoke-RestMethod -Uri "http://127.0.0.1:5000/v1/models" -Headers @{ Authorization = "Bearer my-secret-key" }
```

浏览器中的管理后台会自动注入这个密钥，因此前端访问管理 API 时通常不需要手工填写 token。

## 开发模式与运行模式

当前代码库实际上有两种模式。

### 运行模式

- 一个 Go 服务器
- 默认端口 `5000`
- 同时提供 API 和 `/admin/`

启动方式：

```sh
omnillm start
```

### 开发模式

- Go 后端默认跑在 `5002`
- Vite 前端默认跑在 `5080`
- 管理后台通过 Vite 开发服务器的 `/admin/` 提供

启动方式：

```sh
bun run dev
```

常用脚本：

```sh
bun run dev
bun run dev:frontend
bun run omni start
bun run omni status
bun run omni restart --rebuild
```

其中 `bun run omni` 是开发环境管理器，用来同时管理后端二进制和 Vite 服务，不等同于生产或发布时的运行方式。

## 核心能力

### 统一 API 兼容层

现有 OpenAI 风格和 Anthropic 风格客户端都可以直接指向 OmniLLM，由网关完成上游差异处理。

### CIF 归一化

请求会先进入 CIF，再做调度与序列化。这样新增提供商时，通常只需要实现与 CIF 的双向适配，而不是写一整套两两转换逻辑。

### 提供商路由与故障转移

模型解析支持直接模型名、带提供商前缀的模型名，以及按候选提供商顺序自动回退。

### 虚拟模型

你可以给客户端暴露一个稳定的模型 ID，在后台把它映射到一个或多个真实上游，并定义轮询、随机、优先级或加权策略。

### 管理后台

管理 API 和 Web UI 目前覆盖：

- 提供商接入、激活、重命名、优先级、用量
- 模型发现、刷新、启停、版本信息
- 基于 SSE 的实时日志
- 按提供商、模型、客户端聚合的计量统计
- 内置聊天会话
- 访问令牌管理
- 外部工具配置文件管理

### ToolConfig 与编程工具接入

OmniLLM 可以统一管理 Claude Code、Codex、Droid、OpenCode、AMP 等工具的配置文件。出于安全考虑，编辑外部配置文件需要显式开启 `--enable-config-edit`。

### OmniCode CLI

`omnicode` 不是简单的别名，而是一个面向编程场景的交互式聊天与 agent CLI，可运行一次性 prompt，也可持久化和复用会话。

## 支持的提供商

当前代码库中面向用户的接入流程支持以下提供商类型：

| 提供商 | 认证方式 | 说明 |
|---|---|---|
| GitHub Copilot | OAuth 设备码或 token | CLI 默认启动提供商 |
| OpenAI-Compatible | API key，部分上游可选 | Ollama、vLLM、LM Studio、OpenAI、llama.cpp 等 |
| Alibaba DashScope | API key | 支持 region 与 plan 变体 |
| Azure OpenAI | API key | 支持自定义 endpoint 和 deployment |
| Google | API key | 通用 Google 提供商 |
| Kimi | API key | 通用 Kimi 提供商 |
| Codex | API key | OpenAI Codex 集成 |
| Antigravity | Google OAuth | 通过管理后台 OAuth 流程接入 |

如果你要新增一个提供商，可以参考 [docs/ADDING_A_PROVIDER.md](docs/ADDING_A_PROVIDER.md)。

## API 兼容面

### OpenAI 兼容路由

| 方法 | 路由 |
|---|---|
| `POST` | `/v1/chat/completions` |
| `GET` | `/v1/models` |
| `GET` | `/v1/models/metadata` |
| `POST` | `/v1/embeddings` |
| `POST` | `/v1/responses` |

### Anthropic 兼容路由

| 方法 | 路由 |
|---|---|
| `POST` | `/v1/messages` |
| `POST` | `/v1/messages/count_tokens` |

### 工具路由

| 方法 | 路由 |
|---|---|
| `GET` | `/health` |
| `GET` | `/healthz` |
| `GET` | `/usage` |
| `GET` | `/token` |

### 管理路由

高频使用的管理路由分组包括：

- `/api/admin/providers`
- `/api/admin/virtualmodels`
- `/api/admin/metering/*`
- `/api/admin/chat/sessions`
- `/api/admin/access-tokens`
- `/api/admin/config`
- `/api/admin/settings/log-level`
- `/api/admin/logs/stream`

## CLI 概览

主二进制 `omnillm` 当前包含：

- `start`
- `auth`
- `usage`
- `check-usage`
- `sync-names`
- `debug`
- `provider`
- `model`
- `virtualmodel`
- `config`
- `settings`
- `status`
- `chat`
- `logs`

`omniproxy` 在代理语义下复用了相同的服务端流程。

`omnicode` 则面向交互式编程会话，常用参数包括：

- `--server`
- `--api-key`
- `--model`
- `--session`
- `--prompt`

服务端二进制常用 `start` 参数：

| 参数 | 默认值 | 用途 |
|---|---|---|
| `--port` | `5000` | 服务端口 |
| `--host` | `127.0.0.1` | 绑定地址 |
| `--provider` | `github-copilot` | 启动时默认提供商 |
| `--api-key` | 缺省时自动生成 | 入站认证密钥 |
| `--manual` | `false` | 手工审批模式 |
| `--rate-limit` | `0` | 请求最小间隔秒数 |
| `--wait` | `false` | 限流时等待而不是报错 |
| `--allow-local-endpoints` | `false` | 允许 localhost 和私网 OpenAI 兼容上游 |
| `--enable-config-edit` | `false` | 允许通过管理 API 编辑外部配置文件 |
| `--claude-code` | `false` | 输出 Claude Code 的引导配置 |

## 仓库结构速览

| 路径 | 作用 |
|---|---|
| `main.go` | `omnillm` CLI 主入口 |
| `cmd/omniproxy/` | proxy 形态入口 |
| `cmd/omnicode/` | `omnicode` 入口 |
| `internal/server/` | 服务启动、认证、管理 UI 注册 |
| `internal/routes/` | OpenAI、Anthropic、Responses、admin、metering、chat、config、virtual model 路由处理 |
| `internal/providers/` | 各提供商实现与适配器 |
| `internal/cif/` | CIF 类型定义 |
| `internal/serialization/` | 返回格式序列化 |
| `internal/database/` | SQLite 持久化 |
| `internal/omnicode/` | OmniCode 配置、根命令与会话行为 |
| `frontend/` | React 19 + Vite 管理前端源码 |
| `pages/admin/` | 运行模式下由 Go 服务直接托管的已构建前端 |
| `tests/` | Bun 驱动的 API、前端、浏览器行为测试 |
| `docs/` | 迁移记录、关键变更文档、实现指南 |

## 架构快照

```text
客户端或工具
  -> OmniLLM API/认证层
  -> 进入 CIF
  -> 模型解析与虚拟模型路由
  -> 提供商适配器执行
  -> 序列化为 OpenAI、Anthropic 或 Responses 形态
  -> 计量、日志、持久化、后台可视化
```

后端技术栈：

- Go 1.25
- Gin
- Cobra
- zerolog
- modernc.org/sqlite

前端技术栈：

- React 19
- Vite
- TypeScript
- Material UI 7
- Tailwind CSS 4

## 安全说明

当前内建的安全控制包括：

- API 和管理路由统一使用 API key 保护
- 对 OpenAI-compatible 上游地址做 SSRF 检查
- 面向浏览器的 localhost 风格 CORS 策略
- 默认对 token 做脱敏显示，除非开启 `--show-token`
- 编辑外部配置文件需要显式开启 `--enable-config-edit`

如果不是纯本地或内网使用，建议把 OmniLLM 放在你自己的反向代理或网关之后，并限制管理后台的访问范围。

## 构建、检查与测试

```sh
bun run build
bun run build:go
bun run lint
bun run lint:all
bun run typecheck
bun test
bun run test:frontend
```

`scripts/` 目录里还包含依赖环境的 live 测试脚本，例如模型矩阵和特定提供商验证。

## 进一步阅读

推荐先看：

- [docs/ADDING_A_PROVIDER.md](docs/ADDING_A_PROVIDER.md)
- [docs/CIF_MIGRATION.md](docs/CIF_MIGRATION.md)
- [docs/CONFIG_TEMPLATES.md](docs/CONFIG_TEMPLATES.md)
- [docs/TOOLCONFIG_FIXES.md](docs/TOOLCONFIG_FIXES.md)
- [docs/FRONTEND_TESTS_SUMMARY.md](docs/FRONTEND_TESTS_SUMMARY.md)

`docs/` 目录还保留了大量关于提供商兼容、流式处理、路由、前端和协议迁移的关键变更记录。

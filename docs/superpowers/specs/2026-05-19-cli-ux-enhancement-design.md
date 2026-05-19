# CLI UX Enhancement Design

**Date:** 2026-05-19  
**Scope:** OmniLLM CLI (`omnillm` / `omniproxy`)  
**Target audience:** Interactive operators  
**Mode:** Operator-first enhancement layer — no breaking changes to existing commands

---

## 1. Goals

1. Make the CLI feel consistent, guided, and forgiving for human operators.
2. Keep all existing commands and flags working unchanged.
3. Preserve scriptability (non-TTY paths must still fail clearly without prompts).

## 2. Information Architecture

### 2.1 Keep existing top-level commands

Retain all current commands unchanged:
- `start`, `auth`, `usage`, `check-usage`, `sync-names`, `debug`
- `provider`, `model`, `virtualmodel`, `config`, `settings`, `status`, `logs`

### 2.2 Add operator-friendly aliases

| New name / alias | Maps to |
|---|---|
| `virtual-model` | `virtualmodel` |
| `providers` | `provider` |
| `models` | `model` |
| `model enable <provider-id> <model-id>` | `model toggle --enable` |
| `model disable <provider-id> <model-id>` | `model toggle --disable` |

`model toggle` stays for backward compatibility.

### 2.3 Add `doctor` command

New top-level command for operator troubleshooting. Distinct from `debug` (raw tech inspection).

`doctor` answers:
- Is the config directory present?
- Is the database reachable?
- Is the server running at the configured address?
- Is the API key configured?
- How many providers are configured and active?
- Is there an in-progress auth flow?
- How many virtual models are configured?
- What is the recommended next action?

Output: key/value sections with ✓ / ✗ indicators and a clear "Next step:" footer.

---

## 3. Interactive UX Rules

### 3.1 Prompting philosophy

- **TTY only:** interactive prompts run only when `IsTerminalWriter(cmd.OutOrStdout())` is true.
- **Non-TTY:** missing args fail with normal Cobra argument errors (safe for scripting).
- **No regression:** all commands that already accept args continue to work as before.

### 3.2 Commands that gain interactive ID selection

When a required positional ID is omitted in a TTY session, these commands fetch the relevant list and prompt the user to pick:

| Command | Prompts for |
|---|---|
| `provider delete` | provider ID from `GET /api/admin/providers` |
| `provider activate` | provider ID from provider list |
| `provider deactivate` | provider ID from provider list |
| `provider switch` | provider ID from provider list |
| `provider usage` | provider ID from provider list |
| `provider rename` | provider ID from provider list |
| `model list` | provider ID from provider list |
| `model refresh` | provider ID from provider list |
| `model enable` | provider ID, then model ID from model list |
| `model disable` | provider ID, then model ID from model list |
| `virtualmodel get` | virtual model ID from `GET /api/admin/virtualmodels` |
| `virtualmodel update` | virtual model ID from virtual model list |
| `virtualmodel delete` | virtual model ID from virtual model list |

Implementation: add a helper `resolveProviderID(cmd, c)` and `resolveVirtualModelID(cmd, c)` using the existing `SelectFromOptions`/`SelectAuthProvider` prompt infrastructure from `client.go`.

### 3.3 Post-command summaries and next steps

After key mutating operations, print a short "What's next?" hint to guide the operator. These hints are shown only in table (human) mode.

| Command | Next-step hint |
|---|---|
| `provider add` | `Run 'omnillm model list <id>' to see available models` |
| `provider activate` | `Run 'omnillm status' to confirm active provider` |
| `provider switch` | `Run 'omnillm status' to verify the change` |
| `model enable` | `Run 'omnillm model list <provider-id>' to verify` |
| `virtualmodel create` | `Run 'omnillm virtual-model get <id>' to review` |
| `start` | Print admin console URL and API key hint |

---

## 4. Output Normalization

### 4.1 Stdout vs stderr discipline

**Rule:** all user-facing results go to `cmd.OutOrStdout()`. All diagnostics/progress go to `cmd.ErrOrStderr()`.

**Files requiring fixes:**

| File | Issue |
|---|---|
| `debug.go` | Uses `fmt.Println`/`fmt.Printf` directly → `os.Stdout` |
| `sync-names.go` | Uses `fmt.Printf` directly → `os.Stdout` |
| `virtualmodel.go` `vmGetCmd` | Uses `fmt.Printf` directly → `os.Stdout` |
| `provider.go` `providerUsageCmd` | Uses `fmt.Println`/`fmt.Printf` directly → `os.Stdout` |
| `settings.go` `settingsGetLogLevelCmd` | Uses `fmt.Printf` directly → `os.Stdout` |
| `logs.go` `logsTailCmd` | Uses `fmt.Println`/`fmt.Printf` directly → `os.Stdout` |
| `config.go` `configGetCmd` | Uses `fmt.Println`/`fmt.Printf` directly → `os.Stdout` |
| `model.go` `modelVersionGetCmd` | Uses `fmt.Println`/`fmt.Printf` directly → `os.Stdout` |
| `client.go` `PrintJSON` | Prints to `os.Stdout` not the command writer |
| `client.go` `SelectFromList` | Uses `fmt.Println`/`fmt.Printf` directly |

**Fix pattern:** replace all `fmt.Fprintf(os.Stdout, ...)`, `fmt.Println(...)`, `fmt.Printf(...)` in command `RunE` functions with writes to `cmd.OutOrStdout()` using the existing `PrintSection`, `PrintKeyValue`, `NewTable`, `PrintEmpty` helpers.

### 4.2 `Client.PrintJSON` fix

`Client.PrintJSON` currently ignores `cmd` and writes to `os.Stdout`. Fix: in `NewClient(cmd)`, populate the `Client.stdout` field with `cmd.OutOrStdout()`, and change `Client.PrintJSON` to call `PrintJSON(c.stdout, data)` instead of `PrintJSON(os.Stdout, data)`. The free-function `PrintJSON(w io.Writer, data []byte)` already exists and is correct.

### 4.3 Detail views

Normalize ad hoc detail blocks to consistent pattern:

```
<Entity>: <id>
─────────────────────────────────────────────
Name:             <value>
Description:      <value>
...

<Related section title>
────────────────
<table>
```

Files: `virtualmodel.go` `vmGetCmd`, `provider.go` `providerUsageCmd`, `status.go`.

---

## 5. Flag Validation

### 5.1 Mutually exclusive flags

| Command | Flags | Fix |
|---|---|---|
| `model toggle` | `--enable` / `--disable` | `MarkFlagsMutuallyExclusive` + `MarkFlagsOneRequired` |
| `virtualmodel update` | `--enabled` / `--disabled` | `MarkFlagsMutuallyExclusive` |
| `config set` | `--file` / `--stdin` | `MarkFlagsMutuallyExclusive` + `MarkFlagsOneRequired` |

### 5.2 Allowed values validation

Add `ValidArgs` or `RegisterFlagCompletionFunc` to enforce known values:

| Command / Flag | Valid values |
|---|---|
| Root `--output` | `table`, `json` |
| `logs tail --level` | `fatal`, `error`, `warn`, `info`, `debug`, `trace` |
| `settings set log-level <level>` | `fatal`, `error`, `warn`, `info`, `debug`, `trace` |
| `virtualmodel create/update --strategy` | `round-robin`, `random`, `priority`, `weighted` |
| `virtualmodel create/update --api-shape` | `openai`, `anthropic` |
| `provider add <type>` | `github-copilot`, `openai-compatible`, `alibaba`, `azure-openai`, `google`, `kimi`, `codex` |
| `auth [type]` | same as provider add |
| `provider add --method` | `oauth`, `token` |
| `usage --breakdown` | `provider`, `providers`, `model`, `models`, `client`, `clients`, `none` |

### 5.3 Required flag enforcement

`virtualmodel create --name` is manually validated in `RunE`. Move to `MarkFlagRequired("name")`.

---

## 6. Shell Completions

### 6.1 Add `completion` subcommand

Register `cobra.GenBashCompletion`, `cobra.GenZshCompletion`, `cobra.GenFishCompletion`, `cobra.GenPowerShellCompletion` via a standard `completion` subcommand.

### 6.2 Dynamic completions

Use `RegisterFlagCompletionFunc` to provide live completions from the server:

| Flag / arg | Completion source |
|---|---|
| `provider <id>` args | `GET /api/admin/providers` → list IDs |
| `model list <provider-id>` | provider IDs |
| `model toggle <provider-id>` | provider IDs |
| `model toggle _ <model-id>` | `GET /api/admin/providers/<id>/models` |
| `virtualmodel get/update/delete <id>` | `GET /api/admin/virtualmodels` → list IDs |
| `--strategy` | static list |
| `--api-shape` | static list |
| `--level` (logs) | static list |
| `--output` (root) | static list |
| `auth [type]` | static provider type list |
| `provider add <type>` | static provider type list |

**Important:** live completions (those calling the server) must fail silently and return an empty list if the server is unreachable. Static completions always work offline.

---

## 7. Help and Examples

### 7.1 Add `Example` blocks

Every command below needs a new `Example` field:

| Command | Representative examples |
|---|---|
| `start` | minimal start, with provider, with API key, with rate limit |
| `auth` | interactive, with type, with token flag |
| `provider add` | github-copilot (device), openai-compatible (with flags), alibaba |
| `provider switch` | by ID |
| `model list` | by provider ID |
| `model enable` / `disable` | provider/model pair |
| `virtualmodel create` | round-robin with two upstreams |
| `config set` | from file, from stdin |
| `logs tail` | with level filter |
| `usage` | by provider, by model, with date range |
| `doctor` | (no args needed) |
| `status` | (no args needed) |

### 7.2 Improve `Long` descriptions

Commands that currently have missing or minimal `Long` text:
- `start` — explain flag groups and env var fallbacks
- `model toggle` — explain `--enable`/`--disable` requirement
- `virtualmodel` — expand on upstream syntax (`provider-id/model-id:weight:priority`)
- `provider priorities` — explain `--set id:N` format
- `config set` — explain `--file` vs `--stdin` and when to use each

### 7.3 Root command `--help` grouping

Group subcommands in root help using Cobra `GroupID`:

```
Server:
  start               Start the LLM proxy server

Providers:
  auth                Authenticate and add a provider
  provider            Manage LLM providers
  model               Manage models for a provider
  virtual-model       Manage virtual models (load-balanced aliases)

Admin:
  config              Manage external tool config files
  settings            View and update server settings
  status              Show server status
  logs                Stream or view server logs
  usage               Show usage metrics

Troubleshooting:
  doctor              Check configuration and server health
  debug               Print raw debug information
  sync-names          Refresh provider display names
```

---

## 8. Startup / Onboarding Polish

### 8.1 `start` command UX improvements

- After server starts, print a summary:
  ```
  ✓ OmniLLM started on http://127.0.0.1:5000
    Admin console:  http://127.0.0.1:5000/admin/
    API key:        <first 8 chars>...
  ```
- If `--claude-code` is specified, print the launch command clearly with formatting.
- Keep the `--api-key` default generation behavior, just surface it better.

### 8.2 First-run hint

If no provider is configured when the server starts, log a hint to stderr:
```
  No providers configured. Run 'omnillm auth' to add one.
```

---

## 9. Files to Create / Modify

### New files

| File | Purpose |
|---|---|
| `internal/commands/doctor.go` | New `doctor` command |
| `internal/commands/completion.go` | Shell completion subcommand |

### Modified files

| File | Changes |
|---|---|
| `main.go` | Register `virtual-model`, `providers`, `models` aliases; register `doctor`, `completion`; add command groups |
| `cmd/omniproxy/main.go` | Mirror same alias/group registrations |
| `internal/commands/model.go` | Add `model enable`, `model disable`; add `MarkFlagsMutuallyExclusive`/`MarkFlagsOneRequired` for toggle; add completions; fix `modelVersionGetCmd` output writer |
| `internal/commands/virtualmodel.go` | Add `virtual-model` alias; fix `vmGetCmd` output writer; add `MarkFlagsMutuallyExclusive` for enabled/disabled; add completions; improve `Long` for upstream syntax; add interactive ID selection |
| `internal/commands/provider.go` | Fix `providerUsageCmd` output writer; add interactive ID selection; add completions; add next-step hints |
| `internal/commands/auth.go` | Add completions for provider type arg |
| `internal/commands/settings.go` | Fix output writer; add allowed-values validation |
| `internal/commands/logs.go` | Fix output writers; add allowed-values completion for `--level` |
| `internal/commands/config.go` | Fix output writers; add `MarkFlagsMutuallyExclusive`+`MarkFlagsOneRequired` for `--file`/`--stdin` |
| `internal/commands/debug.go` | Fix all direct `fmt` prints to use `cmd.OutOrStdout()` |
| `internal/commands/sync-names.go` | Fix all direct `fmt` prints to use `cmd.OutOrStdout()` |
| `internal/commands/client.go` | Fix `PrintJSON` writer; fix `SelectFromList` writer |
| `internal/commands/start.go` | Improve startup summary output; add first-run hint; improve help/examples |
| `internal/commands/output.go` | No structural changes; helpers already correct |

---

## 10. Out of Scope

- Frontend/admin UI changes
- New server-side API routes
- Provider implementation changes
- Breaking changes to any existing flag names, command names, or exit codes
- Major command tree restructuring (e.g., moving subcommands between parents)

---

## 11. Testing

- Unit tests in `commands_test.go` for new aliases (verify they route to correct commands)
- Unit tests for `doctor` command (mock server response)
- Unit tests for new `model enable`/`model disable` subcommands
- Unit tests for `MarkFlagsMutuallyExclusive` behavior on toggle/config set/virtualmodel update
- Extend existing output tests in `output_test.go` for normalized detail views
- All tests run via `go test ./internal/commands/...`

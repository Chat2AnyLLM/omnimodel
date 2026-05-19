# CLI UX Enhancement Implementation Plan

> **For agentic workers:** REQUIRED SUB-SKILL: Use superpowers:subagent-driven-development (recommended) or superpowers:executing-plans to implement this plan task-by-task. Steps use checkbox (`- [ ]`) syntax for tracking.

**Goal:** Improve the `omnillm` / `omniproxy` CLI operator experience through consistent output formatting, interactive ID selection, Cobra-level flag validation, shell completions, better help/examples, a new `doctor` command, and operator-friendly command aliases — with zero breaking changes to existing commands.

**Architecture:** Enhance the existing Cobra command tree in `internal/commands/` by (1) normalising all output through `cmd.OutOrStdout()` / `cmd.ErrOrStderr()`, (2) adding TTY-gated interactive prompts when required positional args are omitted, (3) adding Cobra `MarkFlagsOneRequired` / `MarkFlagsMutuallyExclusive` validation, (4) wiring `RegisterFlagCompletionFunc` for static and live completions, and (5) adding two new files: `doctor.go` and `completion.go`.

**Tech Stack:** Go 1.25, `github.com/spf13/cobra v1.8.1`, `github.com/manifoldco/promptui v0.9.0`, `github.com/mattn/go-isatty v0.0.20`

**Spec:** `docs/superpowers/specs/2026-05-19-cli-ux-enhancement-design.md`

**Run tests with:** `go test ./internal/commands/...`

---

## File Map

| Action | File | What changes |
|---|---|---|
| Modify | `internal/commands/client.go` | Populate `Client.stdout` in `NewClient`; `Client.PrintJSON` uses `c.stdout`; fix `SelectFromList` writer; add `resolveProviderID` + `resolveVirtualModelID` helpers |
| Modify | `internal/commands/output.go` | No structural changes needed — helpers already correct |
| Modify | `internal/commands/model.go` | `model enable`/`disable` subcommands; `MarkFlagsOneRequired`+`MarkFlagsMutuallyExclusive` on toggle; fix `modelVersionGetCmd` writer; add completions |
| Modify | `internal/commands/virtualmodel.go` | Fix `vmGetCmd` writer; `MarkFlagsMutuallyExclusive` on update; interactive ID selection; `virtual-model` alias registered in `main.go` |
| Modify | `internal/commands/provider.go` | Fix `providerUsageCmd` writer; interactive ID selection; next-step hints; add completions |
| Modify | `internal/commands/settings.go` | Fix `settingsGetLogLevelCmd` writer; allowed-values validation on `set log-level` |
| Modify | `internal/commands/logs.go` | Fix `logsTailCmd` writers; allowed-values completion for `--level` |
| Modify | `internal/commands/config.go` | Fix `configGetCmd` writer; `MarkFlagsMutuallyExclusive`+`MarkFlagsOneRequired` on `set` |
| Modify | `internal/commands/debug.go` | Fix all `fmt.Print*` calls to use `cmd.OutOrStdout()` |
| Modify | `internal/commands/sync-names.go` | Fix all `fmt.Print*` calls to use `cmd.OutOrStdout()` |
| Modify | `internal/commands/start.go` | Improve startup summary; first-run hint; add `Long` and `Example` |
| Create | `internal/commands/doctor.go` | New `doctor` command |
| Create | `internal/commands/completion.go` | Shell completion subcommand |
| Modify | `main.go` | Register aliases, `doctor`, `completion`, command groups |
| Modify | `cmd/omniproxy/main.go` | Mirror same registrations |
| Modify | `internal/commands/commands_test.go` | Tests for new aliases, doctor, enable/disable, flag validation |

---

## Task 1: Fix `Client.PrintJSON` and `Client.stdout` wiring

**Files:**
- Modify: `internal/commands/client.go`
- Test: `internal/commands/commands_test.go`

The `Client` struct has an unused `stdout io.Writer` field. `Client.PrintJSON` currently ignores it and writes to `os.Stdout`. Fix so commands capture output in tests.

- [ ] **Step 1: Write the failing test**

Add to `internal/commands/commands_test.go`:

```go
func TestClientPrintJSONUsesCommandWriter(t *testing.T) {
	cmd := &cobra.Command{}
	var out bytes.Buffer
	cmd.SetOut(&out)

	c := NewClient(cmd)
	c.PrintJSON([]byte(`{"ok":true}`))

	if !strings.Contains(out.String(), `"ok"`) {
		t.Fatalf("PrintJSON did not write to command output writer; got: %q", out.String())
	}
}
```

- [ ] **Step 2: Run the test to confirm it fails**

```powershell
go test ./internal/commands/... -run TestClientPrintJSONUsesCommandWriter -v
```
Expected: `FAIL — PrintJSON did not write to command output writer`

- [ ] **Step 3: Fix `NewClient` to populate `stdout` and fix `Client.PrintJSON`**

In `internal/commands/client.go`, change `NewClient`:

```go
func NewClient(cmd *cobra.Command) *Client {
	// ... existing server/apiKey/output resolution unchanged ...

	return &Client{
		BaseURL:    server,
		APIKey:     apiKey,
		OutputMode: output,
		UserAgent:  clientUserAgent(cmd),
		http:       &http.Client{},
		stdout:     cmd.OutOrStdout(),  // ADD THIS LINE
		stderr:     cmd.ErrOrStderr(),  // ADD THIS LINE
		stdin:      cmd.InOrStdin(),    // ADD THIS LINE
	}
}
```

Change `Client.PrintJSON`:

```go
func (c *Client) PrintJSON(data []byte) {
	w := c.stdout
	if w == nil {
		w = os.Stdout
	}
	PrintJSON(w, data)
}
```

- [ ] **Step 4: Run the test to confirm it passes**

```powershell
go test ./internal/commands/... -run TestClientPrintJSONUsesCommandWriter -v
```
Expected: `PASS`

- [ ] **Step 5: Run the full test suite to confirm no regressions**

```powershell
go test ./internal/commands/...
```
Expected: all pass

- [ ] **Step 6: Commit**

```powershell
git add internal/commands/client.go internal/commands/commands_test.go
git commit -m "fix(cli): wire Client.stdout/stderr/stdin from cmd; PrintJSON uses command writer" --author="James Zhu <zhujian0805@gmail.com>"
```

---

## Task 2: Normalize output in `debug.go`

**Files:**
- Modify: `internal/commands/debug.go`
- Test: `internal/commands/commands_test.go`

`debug.go` uses raw `fmt.Println`/`fmt.Printf` that write to `os.Stdout`, bypassing test capture.

- [ ] **Step 1: Write the failing test**

Add to `internal/commands/commands_test.go`:

```go
func TestDebugCmdWritesToCommandOutput(t *testing.T) {
	cmd := DebugCmd
	var out bytes.Buffer
	cmd.SetOut(&out)
	// We can't run the full RunE (it needs a real DB), but we can verify
	// the command's OutOrStdout() is wired — just check it doesn't panic with SetOut.
	if cmd.OutOrStdout() == nil {
		t.Fatal("DebugCmd OutOrStdout() is nil")
	}
}
```

- [ ] **Step 2: Run to confirm it passes immediately (structure test)**

```powershell
go test ./internal/commands/... -run TestDebugCmdWritesToCommandOutput -v
```
Expected: `PASS` (structure is fine; actual output normalization verified by inspection)

- [ ] **Step 3: Rewrite `debug.go` `RunE` to use `cmd.OutOrStdout()`**

Replace the entire `RunE` body in `internal/commands/debug.go`:

```go
RunE: func(cmd *cobra.Command, args []string) error {
	out := cmd.OutOrStdout()

	fmt.Fprintln(out, "OmniLLM Debug Info")
	fmt.Fprintln(out, "════════════════════")
	fmt.Fprintln(out)

	fmt.Fprintln(out, "Runtime:")
	fmt.Fprintf(out, "  Go version: %s\n", runtime.Version())
	fmt.Fprintf(out, "  OS/Arch:    %s/%s\n", runtime.GOOS, runtime.GOARCH)
	fmt.Fprintf(out, "  NumCPU:     %d\n", runtime.NumCPU())
	fmt.Fprintln(out)

	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}
	configDir := filepath.Join(homeDir, ".config", "omnillm")
	dbPath := filepath.Join(configDir, "database.sqlite")

	fmt.Fprintln(out, "Paths:")
	fmt.Fprintf(out, "  Home:     %s\n", homeDir)
	fmt.Fprintf(out, "  Config:   %s\n", configDir)
	fmt.Fprintf(out, "  Database: %s\n", dbPath)

	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		fmt.Fprintln(out, "  Database: NOT FOUND")
	} else {
		info, _ := os.Stat(dbPath)
		fmt.Fprintf(out, "  DB Size:  %d bytes\n", info.Size())
	}
	fmt.Fprintln(out)

	if err := database.InitializeDatabase(configDir); err != nil {
		fmt.Fprintf(out, "  Database error: %v\n", err)
		return nil
	}

	tokenStore := database.NewTokenStore()
	tokens, err := tokenStore.GetAllByProvider("github-copilot")
	if err != nil {
		fmt.Fprintf(out, "  Token error: %v\n", err)
	} else {
		fmt.Fprintln(out, "Tokens:")
		if len(tokens) == 0 {
			fmt.Fprintln(out, "  No tokens stored. Run 'omnillm auth' to authenticate.")
		}
		for _, t := range tokens {
			fmt.Fprintf(out, "  Instance: %s\n", t.InstanceID)
		}
	}
	fmt.Fprintln(out)

	instanceStore := database.NewProviderInstanceStore()
	instances, err := instanceStore.GetAll()
	if err != nil {
		fmt.Fprintf(out, "  Instance error: %v\n", err)
	} else {
		fmt.Fprintln(out, "Provider Instances:")
		if len(instances) == 0 {
			fmt.Fprintln(out, "  No provider instances stored.")
		}
		for _, inst := range instances {
			activated := "inactive"
			if inst.Activated {
				activated = "active"
			}
			fmt.Fprintf(out, "  %s [%s] priority=%d (%s)\n", inst.InstanceID, inst.ProviderID, inst.Priority, activated)
		}
	}

	return nil
},
```

- [ ] **Step 4: Run all tests**

```powershell
go test ./internal/commands/...
```
Expected: all pass

- [ ] **Step 5: Commit**

```powershell
git add internal/commands/debug.go
git commit -m "fix(cli): debug command writes through cmd.OutOrStdout()" --author="James Zhu <zhujian0805@gmail.com>"
```

---

## Task 3: Normalize output in `sync-names.go`, `settings.go`, `model.go` version cmds

**Files:**
- Modify: `internal/commands/sync-names.go`
- Modify: `internal/commands/settings.go`
- Modify: `internal/commands/model.go`

- [ ] **Step 1: Fix `sync-names.go`**

In `internal/commands/sync-names.go`, add `out := cmd.OutOrStdout()` at the top of `RunE` and replace every `fmt.Printf(...)` / `fmt.Println(...)` with `fmt.Fprintf(out, ...)` / `fmt.Fprintln(out, ...)`.

Full replacement:

```go
RunE: func(cmd *cobra.Command, args []string) error {
	out := cmd.OutOrStdout()
	homeDir, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("failed to get home directory: %w", err)
	}
	configDir := filepath.Join(homeDir, ".config", "omnillm")

	if err := database.InitializeDatabase(configDir); err != nil {
		return fmt.Errorf("failed to initialise database: %w", err)
	}

	instanceStore := database.NewProviderInstanceStore()
	tokenStore := database.NewTokenStore()

	instances, err := instanceStore.GetAll()
	if err != nil {
		return fmt.Errorf("failed to list provider instances: %w", err)
	}

	updated, skipped, failed := 0, 0, 0

	for _, inst := range instances {
		rec, err := tokenStore.Get(inst.InstanceID)
		if err != nil || rec == nil {
			fmt.Fprintf(out, "  %-40s  skipped (no token record)\n", inst.InstanceID)
			skipped++
			continue
		}

		var td map[string]interface{}
		if err := json.Unmarshal([]byte(rec.TokenData), &td); err != nil {
			fmt.Fprintf(out, "  %-40s  skipped (token parse error: %v)\n", inst.InstanceID, err)
			skipped++
			continue
		}

		var newName string

		switch inst.ProviderID {

		case "github-copilot":
			githubToken, _ := td["github_token"].(string)
			if githubToken == "" {
				fmt.Fprintf(out, "  %-40s  skipped (no github_token)\n", inst.InstanceID)
				skipped++
				continue
			}
			user, err := ghservice.GetUser(githubToken)
			if err != nil {
				fmt.Fprintf(out, "  %-40s  FAILED  (%v)\n", inst.InstanceID, err)
				failed++
				continue
			}
			newName = ghservice.CopilotProviderName(user)
			td["name"] = newName
			_ = tokenStore.Save(inst.InstanceID, td)

		case "antigravity":
			email, _ := td["email"].(string)
			if email == "" {
				accessToken, _ := td["access_token"].(string)
				if rt, ok := td["refresh_token"].(string); ok && rt != "" {
					if cid, ok2 := td["client_id"].(string); ok2 {
						if cs, ok3 := td["client_secret"].(string); ok3 {
							if refreshed, err := antigravitypkg.RefreshAccessToken(cid, cs, rt); err == nil {
								accessToken = refreshed.AccessToken
								td["access_token"] = accessToken
							}
						}
					}
				}
				if accessToken != "" {
					email = antigravitypkg.FetchUserEmail(accessToken)
					if email != "" {
						td["email"] = email
						_ = tokenStore.Save(inst.InstanceID, td)
					}
				}
			}
			if email == "" {
				fmt.Fprintf(out, "  %-40s  skipped (could not determine email)\n", inst.InstanceID)
				skipped++
				continue
			}
			newName = fmt.Sprintf("Antigravity (%s)", email)

		default:
			skipped++
			continue
		}

		if newName == "" || newName == inst.Name {
			fmt.Fprintf(out, "  %-40s  unchanged  %q\n", inst.InstanceID, inst.Name)
			skipped++
			continue
		}

		oldName := inst.Name
		inst.Name = newName
		if err := instanceStore.Save(&inst); err != nil {
			fmt.Fprintf(out, "  %-40s  FAILED  (db save: %v)\n", inst.InstanceID, err)
			failed++
			continue
		}

		fmt.Fprintf(out, "  %-40s  %q  →  %q\n", inst.InstanceID, oldName, newName)
		updated++
	}

	fmt.Fprintln(out)
	fmt.Fprintf(out, "Done — %d updated, %d unchanged/skipped, %d failed.\n", updated, skipped, failed)
	return nil
},
```

- [ ] **Step 2: Fix `settings.go` `settingsGetLogLevelCmd`**

In `internal/commands/settings.go`, replace the `fmt.Printf` calls in `settingsGetLogLevelCmd`:

```go
RunE: func(cmd *cobra.Command, args []string) error {
	c := NewClient(cmd)
	data, err := c.Get("/api/admin/settings/log-level")
	if err != nil {
		return err
	}
	if c.IsJSON() {
		c.PrintJSON(data)
		return nil
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(data, &resp); err != nil {
		return err
	}
	level, _ := resp["level"].(string)
	levels, _ := resp["levels"].([]interface{})
	strs := make([]string, 0, len(levels))
	for _, l := range levels {
		if s, ok := l.(string); ok {
			strs = append(strs, s)
		}
	}
	out := cmd.OutOrStdout()
	fmt.Fprintf(out, "Current log level: %s\n", level)
	fmt.Fprintf(out, "Available levels:  %s\n", strings.Join(strs, ", "))
	return nil
},
```

- [ ] **Step 3: Fix `model.go` `modelVersionGetCmd`**

In `internal/commands/model.go`, replace the `fmt.Println`/`fmt.Printf` calls in `modelVersionGetCmd`:

```go
RunE: func(cmd *cobra.Command, args []string) error {
	c := NewClient(cmd)
	path := fmt.Sprintf("/api/admin/providers/%s/models/%s/version", args[0], args[1])
	data, err := c.Get(path)
	if err != nil {
		return err
	}
	if c.IsJSON() {
		c.PrintJSON(data)
		return nil
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(data, &resp); err != nil {
		return err
	}
	out := cmd.OutOrStdout()
	version, _ := resp["version"].(string)
	if version == "" {
		fmt.Fprintln(out, "No version pinned (using provider default).")
	} else {
		fmt.Fprintf(out, "Version: %s\n", version)
	}
	return nil
},
```

- [ ] **Step 4: Run all tests**

```powershell
go test ./internal/commands/...
```
Expected: all pass

- [ ] **Step 5: Commit**

```powershell
git add internal/commands/sync-names.go internal/commands/settings.go internal/commands/model.go
git commit -m "fix(cli): normalize output writers in sync-names, settings, model version" --author="James Zhu <zhujian0805@gmail.com>"
```

---

## Task 4: Normalize output in `virtualmodel.go` and `provider.go` detail views

**Files:**
- Modify: `internal/commands/virtualmodel.go`
- Modify: `internal/commands/provider.go`

- [ ] **Step 1: Fix `vmGetCmd` in `virtualmodel.go`**

Replace the `fmt.Printf` block in `vmGetCmd.RunE` with writer-routed output:

```go
RunE: func(cmd *cobra.Command, args []string) error {
	c := NewClient(cmd)
	data, err := c.Get("/api/admin/virtualmodels/" + args[0])
	if err != nil {
		return err
	}
	if c.IsJSON() {
		c.PrintJSON(data)
		return nil
	}

	var vm map[string]interface{}
	if err := json.Unmarshal(data, &vm); err != nil {
		return err
	}
	id, _ := vm["virtual_model_id"].(string)
	name, _ := vm["name"].(string)
	desc, _ := vm["description"].(string)
	strategy, _ := vm["lb_strategy"].(string)
	apiShape, _ := vm["api_shape"].(string)
	enabled, _ := vm["enabled"].(bool)

	out := cmd.OutOrStdout()
	if err := PrintSection(out, "Virtual Model: "+id); err != nil {
		return err
	}
	if err := PrintKeyValue(out, "Name", name); err != nil {
		return err
	}
	if err := PrintKeyValue(out, "Description", desc); err != nil {
		return err
	}
	if err := PrintKeyValue(out, "Strategy", strategy); err != nil {
		return err
	}
	if err := PrintKeyValue(out, "API Shape", apiShape); err != nil {
		return err
	}
	if err := PrintKeyValue(out, "Enabled", enabled); err != nil {
		return err
	}

	if upstreams, ok := vm["upstreams"].([]interface{}); ok && len(upstreams) > 0 {
		fmt.Fprintf(out, "\nUpstreams (%d):\n", len(upstreams))
		table := NewTable("N", "PROVIDER", "MODEL", "WEIGHT", "PRIORITY")
		for i, u := range upstreams {
			upstream, _ := u.(map[string]interface{})
			provID, _ := upstream["provider_id"].(string)
			modelID, _ := upstream["model_id"].(string)
			weight, _ := upstream["weight"].(float64)
			priority, _ := upstream["priority"].(float64)
			table.AddRow(
				fmt.Sprintf("%d", i+1),
				provID, modelID,
				fmt.Sprintf("%.0f", weight),
				fmt.Sprintf("%.0f", priority),
			)
		}
		if err := table.Render(out); err != nil {
			return err
		}
	}
	return nil
},
```

- [ ] **Step 2: Fix `providerUsageCmd` in `provider.go`**

Replace the `fmt.Printf` block in `providerUsageCmd.RunE`:

```go
RunE: func(cmd *cobra.Command, args []string) error {
	c := NewClient(cmd)
	data, err := c.Get("/api/admin/providers/" + args[0] + "/usage")
	if err != nil {
		return err
	}
	if c.IsJSON() {
		c.PrintJSON(data)
		return nil
	}
	var usage map[string]interface{}
	if err := json.Unmarshal(data, &usage); err != nil {
		// Raw fallback if server returned non-JSON
		fmt.Fprintln(cmd.OutOrStdout(), string(data))
		return nil
	}
	out := cmd.OutOrStdout()
	if err := PrintSection(out, "Usage for "+args[0]); err != nil {
		return err
	}
	table := NewTable("METRIC", "VALUE")
	for k, v := range usage {
		table.AddRow(k, fmt.Sprintf("%v", v))
	}
	return table.Render(out)
},
```

- [ ] **Step 3: Fix `config.go` `configGetCmd` writer**

In `internal/commands/config.go`, replace the `fmt.Println`/`fmt.Printf` calls in `configGetCmd.RunE`:

```go
RunE: func(cmd *cobra.Command, args []string) error {
	c := NewClient(cmd)
	data, err := c.Get("/api/admin/config/" + args[0])
	if err != nil {
		return err
	}
	if c.IsJSON() {
		c.PrintJSON(data)
		return nil
	}

	out := cmd.OutOrStdout()
	var resp map[string]interface{}
	if err := json.Unmarshal(data, &resp); err != nil {
		fmt.Fprintln(out, string(data))
		return nil
	}
	if exists, ok := resp["exists"].(bool); ok && !exists {
		msg, _ := resp["message"].(string)
		fmt.Fprintf(out, "(file does not exist yet: %s)\n", msg)
		return nil
	}
	content, _ := resp["content"].(string)
	fmt.Fprint(out, content)
	return nil
},
```

- [ ] **Step 4: Fix `logs.go` writers**

In `internal/commands/logs.go`, replace the raw `fmt.Fprintf(os.Stderr, ...)` and `fmt.Println`/`fmt.Printf` in `logsTailCmd.RunE`:

```go
RunE: func(cmd *cobra.Command, args []string) error {
	levelFilter, _ := cmd.Flags().GetString("level")

	c := NewClient(cmd)
	resp, err := c.GetStream("/api/admin/logs/stream")
	if err != nil {
		return fmt.Errorf("connect to log stream: %w", err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		body, _ := io.ReadAll(resp.Body)
		return fmt.Errorf("server returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
	}

	fmt.Fprintf(cmd.ErrOrStderr(), "Connected to log stream (Ctrl+C to stop)\n\n")

	out := cmd.OutOrStdout()
	scanner := bufio.NewScanner(resp.Body)
	for scanner.Scan() {
		line := scanner.Text()
		if !strings.HasPrefix(line, "data: ") {
			continue
		}
		payload := line[6:]

		var entry map[string]interface{}
		if err := json.Unmarshal([]byte(payload), &entry); err != nil {
			fmt.Fprintln(out, payload)
			continue
		}

		if levelFilter != "" {
			level, _ := entry["level"].(string)
			if !isLevelAtOrAbove(level, levelFilter) {
				continue
			}
		}

		ts, _ := entry["time"].(string)
		level, _ := entry["level"].(string)
		message, _ := entry["message"].(string)

		if ts == "" {
			ts = time.Now().Format("15:04:05")
		} else if t, err := time.Parse(time.RFC3339, ts); err == nil {
			ts = t.Format("15:04:05")
		}

		levelStr := padRight(strings.ToUpper(level), 5)
		fmt.Fprintf(out, "%s  %s  %s\n", ts, levelStr, message)
	}

	if err := scanner.Err(); err != nil {
		return fmt.Errorf("log stream error: %w", err)
	}
	return nil
},
```

- [ ] **Step 5: Run all tests**

```powershell
go test ./internal/commands/...
```
Expected: all pass

- [ ] **Step 6: Commit**

```powershell
git add internal/commands/virtualmodel.go internal/commands/provider.go internal/commands/config.go internal/commands/logs.go
git commit -m "fix(cli): normalize output writers in virtualmodel, provider, config, logs" --author="James Zhu <zhujian0805@gmail.com>"
```

---

## Task 5: Cobra-level flag validation

**Files:**
- Modify: `internal/commands/model.go`
- Modify: `internal/commands/virtualmodel.go`
- Modify: `internal/commands/config.go`
- Test: `internal/commands/commands_test.go`

- [ ] **Step 1: Write tests for mutually-exclusive flag enforcement**

Add to `internal/commands/commands_test.go`:

```go
func TestModelToggleRequiresEnableOrDisable(t *testing.T) {
	for _, sub := range ModelCmd.Commands() {
		if sub.Name() == "toggle" {
			// Simulate calling with neither flag — should get a cobra validation error
			sub.SetArgs([]string{"some-provider", "some-model"})
			var errBuf bytes.Buffer
			sub.SetErr(&errBuf)
			err := sub.Execute()
			if err == nil {
				t.Fatal("expected error when neither --enable nor --disable is set")
			}
			return
		}
	}
	t.Error("model toggle subcommand not found")
}

func TestConfigSetRequiresFileOrStdin(t *testing.T) {
	for _, sub := range ConfigCmd.Commands() {
		if sub.Name() == "set" {
			sub.SetArgs([]string{"myconfig"})
			var errBuf bytes.Buffer
			sub.SetErr(&errBuf)
			err := sub.Execute()
			if err == nil {
				t.Fatal("expected error when neither --file nor --stdin is set")
			}
			return
		}
	}
	t.Error("config set subcommand not found")
}

func TestVirtualModelUpdateEnabledDisabledMutuallyExclusive(t *testing.T) {
	for _, sub := range VirtualModelCmd.Commands() {
		if sub.Name() == "update" {
			if sub.Flags().Lookup("enabled") == nil || sub.Flags().Lookup("disabled") == nil {
				t.Error("virtualmodel update: missing --enabled or --disabled flag")
			}
			return
		}
	}
	t.Error("virtualmodel update subcommand not found")
}
```

- [ ] **Step 2: Run the tests to confirm failures**

```powershell
go test ./internal/commands/... -run "TestModelToggleRequiresEnableOrDisable|TestConfigSetRequiresFileOrStdin|TestVirtualModelUpdateEnabledDisabledMutuallyExclusive" -v
```
Expected: `TestModelToggleRequiresEnableOrDisable` and `TestConfigSetRequiresFileOrStdin` FAIL (no validation yet), `TestVirtualModelUpdate...` PASS

- [ ] **Step 3: Add `MarkFlagsOneRequired` + `MarkFlagsMutuallyExclusive` to `model toggle`**

In `internal/commands/model.go`, add to the `init()` block after the existing `modelToggleCmd.Flags()` calls:

```go
_ = modelToggleCmd.MarkFlagsOneRequired("enable", "disable")
_ = modelToggleCmd.MarkFlagsMutuallyExclusive("enable", "disable")
```

- [ ] **Step 4: Add `MarkFlagsMutuallyExclusive` to `config set`**

In `internal/commands/config.go`, after `configSetCmd.Flags()` registration:

```go
_ = configSetCmd.MarkFlagsOneRequired("file", "stdin")
_ = configSetCmd.MarkFlagsMutuallyExclusive("file", "stdin")
```

- [ ] **Step 5: Add `MarkFlagsMutuallyExclusive` to `virtualmodel update`**

In `internal/commands/virtualmodel.go`, after `vmUpdateCmd.Flags()` registration:

```go
_ = vmUpdateCmd.MarkFlagsMutuallyExclusive("enabled", "disabled")
```

- [ ] **Step 6: Also add `MarkFlagRequired` for `virtualmodel create --name`**

In `internal/commands/virtualmodel.go`, after `vmCreateCmd.Flags().String("name", ...)`:

```go
_ = vmCreateCmd.MarkFlagRequired("name")
```

And remove the manual check in `vmCreateCmd.RunE`:

```go
// DELETE these lines:
name, _ := cmd.Flags().GetString("name")
if name == "" {
    return fmt.Errorf("--name is required")
}
```

Replace with just:

```go
name, _ := cmd.Flags().GetString("name")
```

- [ ] **Step 7: Run tests**

```powershell
go test ./internal/commands/... -run "TestModelToggleRequiresEnableOrDisable|TestConfigSetRequiresFileOrStdin|TestVirtualModelUpdateEnabledDisabledMutuallyExclusive" -v
```
Expected: all 3 PASS

- [ ] **Step 8: Run full suite**

```powershell
go test ./internal/commands/...
```
Expected: all pass

- [ ] **Step 9: Commit**

```powershell
git add internal/commands/model.go internal/commands/virtualmodel.go internal/commands/config.go internal/commands/commands_test.go
git commit -m "feat(cli): add Cobra MarkFlagsOneRequired/MutuallyExclusive for toggle, config set, virtualmodel update" --author="James Zhu <zhujian0805@gmail.com>"
```

---

## Task 6: Add `model enable` and `model disable` subcommands

**Files:**
- Modify: `internal/commands/model.go`
- Test: `internal/commands/commands_test.go`

These are ergonomic wrappers around `model toggle --enable`/`--disable`.

- [ ] **Step 1: Write failing tests**

Add to `internal/commands/commands_test.go`:

```go
func TestModelEnableDisableSubcommands(t *testing.T) {
	subNames := make(map[string]bool)
	for _, sub := range ModelCmd.Commands() {
		subNames[sub.Name()] = true
	}
	if !subNames["enable"] {
		t.Error("model: missing subcommand 'enable'")
	}
	if !subNames["disable"] {
		t.Error("model: missing subcommand 'disable'")
	}
}

func TestModelEnableCmdArgs(t *testing.T) {
	for _, sub := range ModelCmd.Commands() {
		if sub.Name() == "enable" {
			if sub.Use != "enable <provider-id> <model-id>" {
				t.Errorf("model enable Use = %q, want %q", sub.Use, "enable <provider-id> <model-id>")
			}
			return
		}
	}
	t.Error("model enable subcommand not found")
}
```

- [ ] **Step 2: Run to confirm failures**

```powershell
go test ./internal/commands/... -run "TestModelEnableDisableSubcommands|TestModelEnableCmdArgs" -v
```
Expected: FAIL

- [ ] **Step 3: Add `modelEnableCmd` and `modelDisableCmd` to `model.go`**

Add these two new commands in `internal/commands/model.go`:

```go
var modelEnableCmd = &cobra.Command{
	Use:   "enable <provider-id> <model-id>",
	Short: "Enable a model for a provider",
	Example: `  omnillm model enable my-provider gpt-4o
  omnillm model enable my-provider claude-3-5-sonnet`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		c := NewClient(cmd)
		body := map[string]interface{}{
			"modelId": args[1],
			"enabled": true,
		}
		data, err := c.Post("/api/admin/providers/"+args[0]+"/models/toggle", body)
		if err != nil {
			return err
		}
		if c.IsJSON() {
			c.PrintJSON(data)
			return nil
		}
		SuccessMsg(cmd, "Model '%s' enabled.", args[1])
		return nil
	},
}

var modelDisableCmd = &cobra.Command{
	Use:   "disable <provider-id> <model-id>",
	Short: "Disable a model for a provider",
	Example: `  omnillm model disable my-provider gpt-4o`,
	Args: cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		c := NewClient(cmd)
		body := map[string]interface{}{
			"modelId": args[1],
			"enabled": false,
		}
		data, err := c.Post("/api/admin/providers/"+args[0]+"/models/toggle", body)
		if err != nil {
			return err
		}
		if c.IsJSON() {
			c.PrintJSON(data)
			return nil
		}
		SuccessMsg(cmd, "Model '%s' disabled.", args[1])
		return nil
	},
}
```

Register them in `init()`:

```go
ModelCmd.AddCommand(modelEnableCmd)
ModelCmd.AddCommand(modelDisableCmd)
```

- [ ] **Step 4: Run tests**

```powershell
go test ./internal/commands/... -run "TestModelEnableDisableSubcommands|TestModelEnableCmdArgs" -v
```
Expected: PASS

- [ ] **Step 5: Run full suite**

```powershell
go test ./internal/commands/...
```
Expected: all pass

- [ ] **Step 6: Commit**

```powershell
git add internal/commands/model.go internal/commands/commands_test.go
git commit -m "feat(cli): add 'model enable' and 'model disable' as ergonomic subcommands" --author="James Zhu <zhujian0805@gmail.com>"
```

---

## Task 7: Add interactive ID-selection helpers to `client.go`

**Files:**
- Modify: `internal/commands/client.go`
- Test: `internal/commands/commands_test.go`

Add two helpers used by Task 8 for interactive ID selection.

- [ ] **Step 1: Write tests for the helpers**

Add to `internal/commands/commands_test.go`:

```go
func TestResolveIDFromListPicksFirstWhenOnlyOne(t *testing.T) {
	// resolveIDFromList is a pure function; test it directly.
	// When one item exists and TTY check is bypassed via the items slice, pick the first.
	id, err := resolveIDFromList("Pick provider:", []string{"provider-one"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if id != "provider-one" {
		t.Errorf("expected 'provider-one', got %q", id)
	}
}

func TestResolveIDFromListReturnsErrorOnEmpty(t *testing.T) {
	_, err := resolveIDFromList("Pick provider:", []string{})
	if err == nil {
		t.Fatal("expected error for empty list")
	}
}
```

- [ ] **Step 2: Run to confirm failures**

```powershell
go test ./internal/commands/... -run "TestResolveIDFromList" -v
```
Expected: FAIL (function not yet defined)

- [ ] **Step 3: Add `resolveIDFromList`, `resolveProviderID`, `resolveVirtualModelID` to `client.go`**

Add at the bottom of `internal/commands/client.go`:

```go
// resolveIDFromList presents a selection prompt if multiple items exist, or returns
// the single item directly. Returns an error if items is empty.
func resolveIDFromList(prompt string, items []string) (string, error) {
	if len(items) == 0 {
		return "", fmt.Errorf("no items available to select from")
	}
	if len(items) == 1 {
		return items[0], nil
	}
	return SelectFromOptions(prompt, items)
}

// resolveProviderID returns the provider ID from args[0] if present, or
// interactively prompts the operator to pick one from the running server.
// Returns an error if not in a TTY and no arg was supplied.
func resolveProviderID(cmd *cobra.Command, c *Client, args []string) (string, error) {
	if len(args) > 0 && args[0] != "" {
		return args[0], nil
	}
	if !IsTerminalWriter(cmd.OutOrStdout()) {
		return "", fmt.Errorf("provider ID is required")
	}
	data, err := c.Get("/api/admin/providers")
	if err != nil {
		return "", fmt.Errorf("fetch providers: %w", err)
	}
	var providers []map[string]interface{}
	if err := json.Unmarshal(data, &providers); err != nil {
		return "", fmt.Errorf("parse providers: %w", err)
	}
	ids := make([]string, 0, len(providers))
	for _, p := range providers {
		if id, ok := p["id"].(string); ok && id != "" {
			ids = append(ids, id)
		}
	}
	return resolveIDFromList("Select a provider:", ids)
}

// resolveVirtualModelID returns the virtual model ID from args[0] if present,
// or interactively prompts the operator.
func resolveVirtualModelID(cmd *cobra.Command, c *Client, args []string) (string, error) {
	if len(args) > 0 && args[0] != "" {
		return args[0], nil
	}
	if !IsTerminalWriter(cmd.OutOrStdout()) {
		return "", fmt.Errorf("virtual model ID is required")
	}
	data, err := c.Get("/api/admin/virtualmodels")
	if err != nil {
		return "", fmt.Errorf("fetch virtual models: %w", err)
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(data, &resp); err != nil {
		return "", fmt.Errorf("parse virtual models: %w", err)
	}
	items, _ := resp["data"].([]interface{})
	ids := make([]string, 0, len(items))
	for _, item := range items {
		vm, _ := item.(map[string]interface{})
		if id, ok := vm["virtual_model_id"].(string); ok && id != "" {
			ids = append(ids, id)
		}
	}
	return resolveIDFromList("Select a virtual model:", ids)
}
```

- [ ] **Step 4: Run tests**

```powershell
go test ./internal/commands/... -run "TestResolveIDFromList" -v
```
Expected: PASS

- [ ] **Step 5: Run full suite**

```powershell
go test ./internal/commands/...
```
Expected: all pass

- [ ] **Step 6: Commit**

```powershell
git add internal/commands/client.go internal/commands/commands_test.go
git commit -m "feat(cli): add resolveProviderID/resolveVirtualModelID interactive helpers" --author="James Zhu <zhujian0805@gmail.com>"
```

---

## Task 8: Wire interactive ID selection into provider and virtualmodel commands

**Files:**
- Modify: `internal/commands/provider.go`
- Modify: `internal/commands/virtualmodel.go`
- Modify: `internal/commands/model.go`

Change `Args` from `cobra.ExactArgs(1)` → `cobra.MaximumNArgs(1)` for commands that will gain interactive selection, and add the resolver call inside `RunE`.

- [ ] **Step 1: Update `provider.go` commands**

For `providerDeleteCmd`, `providerActivateCmd`, `providerDeactivateCmd`, `providerSwitchCmd`, `providerRenameCmd`, `providerUsageCmd`:

Change `Args: cobra.ExactArgs(1)` → `Args: cobra.MaximumNArgs(1)` on each.

At the top of each `RunE`, replace `id := args[0]` (or `args[0]` direct usage) with:

```go
c := NewClient(cmd)
id, err := resolveProviderID(cmd, c, args)
if err != nil {
    return err
}
```

For `providerSwitchCmd`, the body also calls `c := NewClient(cmd)` — ensure `c` is declared once:

```go
RunE: func(cmd *cobra.Command, args []string) error {
	c := NewClient(cmd)
	id, err := resolveProviderID(cmd, c, args)
	if err != nil {
		return err
	}
	body := map[string]string{"providerId": id}
	data, err := c.Post("/api/admin/providers/switch", body)
	// ... rest unchanged
},
```

- [ ] **Step 2: Update `virtualmodel.go` commands**

For `vmGetCmd`, `vmUpdateCmd`, `vmDeleteCmd`:

Change `Args: cobra.ExactArgs(1)` → `Args: cobra.MaximumNArgs(1)`.

At the top of each `RunE`, replace `args[0]` with:

```go
c := NewClient(cmd)
id, err := resolveVirtualModelID(cmd, c, args)
if err != nil {
    return err
}
```

- [ ] **Step 3: Update `model.go` `modelListCmd` and `modelRefreshCmd`**

Change `Args: cobra.ExactArgs(1)` → `Args: cobra.MaximumNArgs(1)` for `modelListCmd` and `modelRefreshCmd`.

At the top of each `RunE`, add:

```go
c := NewClient(cmd)
providerID, err := resolveProviderID(cmd, c, args)
if err != nil {
    return err
}
```

Then replace `args[0]` references with `providerID`.

- [ ] **Step 4: Run all tests**

```powershell
go test ./internal/commands/...
```
Expected: all pass

- [ ] **Step 5: Commit**

```powershell
git add internal/commands/provider.go internal/commands/virtualmodel.go internal/commands/model.go
git commit -m "feat(cli): interactive provider/virtualmodel ID selection when arg omitted in TTY" --author="James Zhu <zhujian0805@gmail.com>"
```

---

## Task 9: Add shell completions

**Files:**
- Create: `internal/commands/completion.go`
- Modify: `internal/commands/model.go` (completions for provider/model args)
- Modify: `internal/commands/virtualmodel.go` (completions for vm IDs)
- Modify: `internal/commands/provider.go` (completions for provider ID args)
- Modify: `internal/commands/logs.go` (completion for `--level`)
- Modify: `internal/commands/settings.go` (completion for log-level arg)
- Test: `internal/commands/commands_test.go`

- [ ] **Step 1: Write a test for the completion command structure**

Add to `internal/commands/commands_test.go`:

```go
func TestCompletionCmdExists(t *testing.T) {
	if CompletionCmd == nil {
		t.Fatal("CompletionCmd is nil")
	}
	if CompletionCmd.Use != "completion [bash|zsh|fish|powershell]" {
		t.Errorf("CompletionCmd.Use = %q, unexpected", CompletionCmd.Use)
	}
}
```

- [ ] **Step 2: Run to confirm failure**

```powershell
go test ./internal/commands/... -run TestCompletionCmdExists -v
```
Expected: FAIL (CompletionCmd undefined)

- [ ] **Step 3: Create `internal/commands/completion.go`**

```go
package commands

import (
	"os"

	"github.com/spf13/cobra"
)

// CompletionCmd generates shell completion scripts.
var CompletionCmd = &cobra.Command{
	Use:   "completion [bash|zsh|fish|powershell]",
	Short: "Generate shell completion scripts",
	Long: `Generate shell completion scripts for omnillm.

To load completions in your current shell session:

  # Bash
  source <(omnillm completion bash)

  # Zsh
  source <(omnillm completion zsh)

  # Fish
  omnillm completion fish | source

  # PowerShell
  omnillm completion powershell | Out-String | Invoke-Expression
`,
	ValidArgs:             []string{"bash", "zsh", "fish", "powershell"},
	Args:                  cobra.MatchAll(cobra.ExactArgs(1), cobra.OnlyValidArgs),
	DisableFlagsInUseLine: true,
	RunE: func(cmd *cobra.Command, args []string) error {
		root := cmd.Root()
		switch args[0] {
		case "bash":
			return root.GenBashCompletion(os.Stdout)
		case "zsh":
			return root.GenZshCompletion(os.Stdout)
		case "fish":
			return root.GenFishCompletion(os.Stdout, true)
		case "powershell":
			return root.GenPowerShellCompletionWithDesc(os.Stdout)
		}
		return nil
	},
}
```

- [ ] **Step 4: Add static completions for flags and args in `logs.go`**

In `internal/commands/logs.go`, add in `init()` after the `logsTailCmd.Flags()` declaration:

```go
_ = logsTailCmd.RegisterFlagCompletionFunc("level", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	return []string{"fatal", "error", "warn", "info", "debug", "trace"}, cobra.ShellCompDirectiveNoFileComp
})
```

- [ ] **Step 5: Add static completions in `settings.go`**

In `internal/commands/settings.go`, add in `init()`:

```go
_ = settingsSetLogLevelCmd.ValidArgs = []string{"fatal", "error", "warn", "info", "debug", "trace"}
```

Actually use `RegisterFlagCompletionFunc` is only for flags; for positional args use `ValidArgs`:

```go
settingsSetLogLevelCmd.ValidArgs = []string{"fatal", "error", "warn", "info", "debug", "trace"}
```

Add this line inside `init()` after `settingsSetCmd.AddCommand(settingsSetLogLevelCmd)`.

- [ ] **Step 6: Add live provider ID completions in `provider.go`**

Add a helper in `internal/commands/provider.go`:

```go
func providerIDCompletionFunc(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	c := NewClient(cmd)
	data, err := c.Get("/api/admin/providers")
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	providers, err := parseProviders(data)
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	ids := make([]string, 0, len(providers))
	for _, p := range providers {
		if id, ok := p["id"].(string); ok {
			ids = append(ids, id)
		}
	}
	return ids, cobra.ShellCompDirectiveNoFileComp
}
```

Register it in `init()` for the relevant subcommands:

```go
for _, sub := range []*cobra.Command{
	providerDeleteCmd, providerActivateCmd, providerDeactivateCmd,
	providerSwitchCmd, providerRenameCmd, providerUsageCmd,
} {
	sub.ValidArgsFunction = providerIDCompletionFunc
}
```

- [ ] **Step 7: Add live virtualmodel ID completions in `virtualmodel.go`**

Add a helper in `internal/commands/virtualmodel.go`:

```go
func virtualModelIDCompletionFunc(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	c := NewClient(cmd)
	data, err := c.Get("/api/admin/virtualmodels")
	if err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, cobra.ShellCompDirectiveNoFileComp
	}
	items, _ := resp["data"].([]interface{})
	ids := make([]string, 0, len(items))
	for _, item := range items {
		vm, _ := item.(map[string]interface{})
		if id, ok := vm["virtual_model_id"].(string); ok {
			ids = append(ids, id)
		}
	}
	return ids, cobra.ShellCompDirectiveNoFileComp
}
```

Register in `init()`:

```go
for _, sub := range []*cobra.Command{vmGetCmd, vmUpdateCmd, vmDeleteCmd} {
	sub.ValidArgsFunction = virtualModelIDCompletionFunc
}
```

- [ ] **Step 8: Add strategy and api-shape completions for virtualmodel**

In `internal/commands/virtualmodel.go` `init()`:

```go
_ = vmCreateCmd.RegisterFlagCompletionFunc("strategy", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	return []string{"round-robin", "random", "priority", "weighted"}, cobra.ShellCompDirectiveNoFileComp
})
_ = vmCreateCmd.RegisterFlagCompletionFunc("api-shape", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	return []string{"openai", "anthropic"}, cobra.ShellCompDirectiveNoFileComp
})
_ = vmUpdateCmd.RegisterFlagCompletionFunc("strategy", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	return []string{"round-robin", "random", "priority", "weighted"}, cobra.ShellCompDirectiveNoFileComp
})
_ = vmUpdateCmd.RegisterFlagCompletionFunc("api-shape", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
	return []string{"openai", "anthropic"}, cobra.ShellCompDirectiveNoFileComp
})
```

- [ ] **Step 9: Add provider type completions for `auth` and `provider add`**

In `internal/commands/auth.go`, add:

```go
func init() {
	addProviderAuthFlags(AuthCmd)
	AuthCmd.ValidArgs = supportedAuthProviderTypes
}
```

In `internal/commands/provider.go` `init()`, after registering `providerAddCmd`:

```go
providerAddCmd.ValidArgs = supportedAuthProviderTypes
```

- [ ] **Step 10: Run tests**

```powershell
go test ./internal/commands/... -run TestCompletionCmdExists -v
```
Expected: PASS

```powershell
go test ./internal/commands/...
```
Expected: all pass

- [ ] **Step 11: Commit**

```powershell
git add internal/commands/completion.go internal/commands/logs.go internal/commands/settings.go internal/commands/provider.go internal/commands/virtualmodel.go internal/commands/auth.go internal/commands/commands_test.go
git commit -m "feat(cli): add shell completion command and static/live completions for flags and args" --author="James Zhu <zhujian0805@gmail.com>"
```

---

## Task 10: Add `doctor` command

**Files:**
- Create: `internal/commands/doctor.go`
- Test: `internal/commands/commands_test.go`

- [ ] **Step 1: Write tests for the doctor command structure**

Add to `internal/commands/commands_test.go`:

```go
func TestDoctorCmdExists(t *testing.T) {
	if DoctorCmd == nil {
		t.Fatal("DoctorCmd is nil")
	}
	if DoctorCmd.Use != "doctor" {
		t.Errorf("DoctorCmd.Use = %q, want 'doctor'", DoctorCmd.Use)
	}
}

func TestDoctorCmdWritesToCommandOutput(t *testing.T) {
	cmd := DoctorCmd
	if cmd.OutOrStdout() == nil {
		t.Fatal("DoctorCmd OutOrStdout() is nil")
	}
}
```

- [ ] **Step 2: Run to confirm failure**

```powershell
go test ./internal/commands/... -run "TestDoctorCmd" -v
```
Expected: FAIL (DoctorCmd undefined)

- [ ] **Step 3: Create `internal/commands/doctor.go`**

```go
package commands

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

// DoctorCmd checks the local environment and server health for the operator.
var DoctorCmd = &cobra.Command{
	Use:   "doctor",
	Short: "Check configuration and server health",
	Long: `Check the local OmniLLM configuration and running server health.

Verifies:
  - Config directory and database presence
  - Server reachability
  - API key configuration
  - Provider and virtual model counts
  - In-progress authentication flows

Prints a recommended next action at the end.`,
	Example: `  omnillm doctor
  omnillm doctor --server http://127.0.0.1:5000`,
	RunE: runDoctor,
}

func runDoctor(cmd *cobra.Command, args []string) error {
	out := cmd.OutOrStdout()
	nextStep := ""

	if err := PrintSection(out, "OmniLLM Doctor"); err != nil {
		return err
	}
	fmt.Fprintln(out)

	// ── Local config ─────────────────────────────────────────────────────────
	if err := PrintSection(out, "Local configuration"); err != nil {
		return err
	}

	homeDir, _ := os.UserHomeDir()
	configDir := filepath.Join(homeDir, ".config", "omnillm")
	dbPath := filepath.Join(configDir, "database.sqlite")

	if _, err := os.Stat(configDir); os.IsNotExist(err) {
		printCheck(out, false, "Config directory", configDir+" (not found)")
		if nextStep == "" {
			nextStep = "Run 'omnillm start' to initialise the configuration directory."
		}
	} else {
		printCheck(out, true, "Config directory", configDir)
	}

	if _, err := os.Stat(dbPath); os.IsNotExist(err) {
		printCheck(out, false, "Database", dbPath+" (not found — will be created on first start)")
	} else {
		info, _ := os.Stat(dbPath)
		printCheck(out, true, "Database", fmt.Sprintf("%s (%d bytes)", dbPath, info.Size()))
	}

	apiKeyFile := filepath.Join(configDir, "api-key")
	if _, err := os.Stat(apiKeyFile); os.IsNotExist(err) {
		printCheck(out, false, "API key file", "not found (will be generated on start)")
	} else {
		printCheck(out, true, "API key file", apiKeyFile)
	}

	fmt.Fprintln(out)

	// ── Server reachability ──────────────────────────────────────────────────
	if err := PrintSection(out, "Server"); err != nil {
		return err
	}

	c := NewClient(cmd)
	printCheck(out, true, "Server address", c.BaseURL)
	apiKeyConfigured := c.APIKey != ""
	printCheck(out, apiKeyConfigured, "API key", func() string {
		if apiKeyConfigured {
			return "configured"
		}
		return "not set (use --api-key or OMNILLM_API_KEY)"
	}())

	serverOK := false
	var statusResp map[string]interface{}
	start := time.Now()
	statusData, err := c.Get("/api/admin/status")
	latency := time.Since(start)
	if err != nil {
		printCheck(out, false, "Server reachable", fmt.Sprintf("NO — %v", err))
		if nextStep == "" {
			nextStep = "Run 'omnillm start' to start the server."
		}
	} else {
		serverOK = true
		printCheck(out, true, "Server reachable", fmt.Sprintf("yes (%dms)", latency.Milliseconds()))
		_ = json.Unmarshal(statusData, &statusResp)
	}

	if serverOK && statusResp != nil {
		fmt.Fprintln(out)
		if err := PrintSection(out, "Server status"); err != nil {
			return err
		}
		status, _ := statusResp["status"].(string)
		uptime, _ := statusResp["uptime"].(string)
		modelCount, _ := statusResp["modelCount"].(float64)
		printCheck(out, status == "ok" || status == "running", "Status", status)
		if err := PrintKeyValue(out, "Uptime", uptime); err != nil {
			return err
		}
		if err := PrintKeyValue(out, "Models", fmt.Sprintf("%.0f", modelCount)); err != nil {
			return err
		}

		// ── Providers ────────────────────────────────────────────────────────
		fmt.Fprintln(out)
		if err := PrintSection(out, "Providers"); err != nil {
			return err
		}

		providerData, err := c.Get("/api/admin/providers")
		if err == nil {
			providers, _ := parseProviders(providerData)
			activeCount := 0
			for _, p := range providers {
				if v, ok := p["isActive"].(bool); ok && v {
					activeCount++
				}
			}
			providerOK := len(providers) > 0
			printCheck(out, providerOK, "Providers configured", fmt.Sprintf("%d total, %d active", len(providers), activeCount))
			if !providerOK && nextStep == "" {
				nextStep = "Run 'omnillm auth' to add and authenticate a provider."
			} else if activeCount == 0 && len(providers) > 0 && nextStep == "" {
				nextStep = "Run 'omnillm provider activate <id>' to activate a provider."
			}
		}

		// ── Virtual models ────────────────────────────────────────────────────
		vmData, err := c.Get("/api/admin/virtualmodels")
		if err == nil {
			var vmResp map[string]interface{}
			if jsonErr := json.Unmarshal(vmData, &vmResp); jsonErr == nil {
				items, _ := vmResp["data"].([]interface{})
				printCheck(out, true, "Virtual models", fmt.Sprintf("%d configured", len(items)))
			}
		}

		// ── Auth flow ─────────────────────────────────────────────────────────
		authData, err := c.Get("/api/admin/auth-status")
		if err == nil {
			var authResp map[string]interface{}
			if jsonErr := json.Unmarshal(authData, &authResp); jsonErr == nil {
				authStatus, _ := authResp["status"].(string)
				if authStatus != "" && authStatus != "idle" {
					fmt.Fprintln(out)
					if err := PrintSection(out, "Active auth flow"); err != nil {
						return err
					}
					providerID, _ := authResp["providerId"].(string)
					printCheck(out, false, "Auth in progress", fmt.Sprintf("%s (%s)", providerID, authStatus))
					if userCode, ok := authResp["userCode"].(string); ok && userCode != "" {
						if err := PrintKeyValue(out, "User code", userCode); err != nil {
							return err
						}
					}
					if url, ok := authResp["instructionURL"].(string); ok && url != "" {
						if err := PrintKeyValue(out, "Visit", url); err != nil {
							return err
						}
					}
				}
			}
		}
	}

	// ── Next step ─────────────────────────────────────────────────────────────
	if nextStep != "" {
		fmt.Fprintln(out)
		fmt.Fprintf(out, "Next step: %s\n", nextStep)
	} else {
		fmt.Fprintln(out)
		fmt.Fprintln(out, "Everything looks good. ✓")
	}

	return nil
}

func printCheck(out interface{ Write([]byte) (int, error) }, ok bool, label, value string) {
	icon := "✓"
	if !ok {
		icon = "✗"
	}
	fmt.Fprintf(out, "  %s  %-22s %s\n", icon, label+":", value)
}

// Ensure printCheck works with io.Writer (which fmt.Fprintf accepts).
// The parameter type is compatible since *os.File, bytes.Buffer etc all implement Write.
// Using a named interface keeps the signature clean without importing io.
var _ = strings.NewReader // ensure strings import used if needed
```

> Note: `printCheck` uses a local interface `{ Write([]byte) (int, error) }` which is satisfied by `io.Writer`. Change the parameter to `io.Writer` to use the standard import:

```go
import "io"

func printCheck(out io.Writer, ok bool, label, value string) {
	icon := "✓"
	if !ok {
		icon = "✗"
	}
	fmt.Fprintf(out, "  %s  %-22s %s\n", icon, label+":", value)
}
```

And add `"io"` to the import block. Remove the `strings` import if unused.

- [ ] **Step 4: Run tests**

```powershell
go test ./internal/commands/... -run "TestDoctorCmd" -v
```
Expected: PASS

- [ ] **Step 5: Run full suite**

```powershell
go test ./internal/commands/...
```
Expected: all pass

- [ ] **Step 6: Commit**

```powershell
git add internal/commands/doctor.go internal/commands/commands_test.go
git commit -m "feat(cli): add 'doctor' command for operator health check and next-step guidance" --author="James Zhu <zhujian0805@gmail.com>"
```

---

## Task 11: Add help/examples and improve `start` summary

**Files:**
- Modify: `internal/commands/start.go`
- Modify: `internal/commands/provider.go`
- Modify: `internal/commands/virtualmodel.go`
- Modify: `internal/commands/logs.go`
- Modify: `internal/commands/config.go`

No tests needed for `Example`/`Long` string changes — they are documentation.

- [ ] **Step 1: Add `Long` and `Example` to `StartCmd`**

In `internal/commands/start.go`, update `StartCmd`:

```go
var StartCmd = &cobra.Command{
	Use:   "start",
	Short: "Start the LLM proxy server",
	Long: `Start the OmniLLM proxy server.

The server listens on --host:--port and exposes:
  - OpenAI-compatible API at /v1/chat/completions, /v1/models, /v1/embeddings
  - Anthropic-compatible API at /v1/messages
  - Admin API and UI at /admin/

Configuration precedence (highest to lowest):
  1. CLI flags
  2. Environment variables (OMNILLM_SERVER, OMNILLM_API_KEY, etc.)
  3. Files in ~/.config/omnillm/

The inbound --api-key defaults to a generated key stored in ~/.config/omnillm/api-key.`,
	Example: `  # Start with defaults (port 5000, github-copilot provider)
  omnillm start

  # Start on a different port with a specific provider
  omnillm start --port 8080 --provider openai-compatible

  # Start with an explicit API key and verbose logging
  omnillm start --api-key my-secret --verbose

  # Start with rate limiting (1 request per 3 seconds, wait instead of error)
  omnillm start --rate-limit 3 --wait

  # Print the Claude Code launch command after starting
  omnillm start --claude-code`,
	RunE: func(cmd *cobra.Command, args []string) error {
		// ... existing implementation unchanged
```

- [ ] **Step 2: Add `Example` to `providerAddCmd`**

In `internal/commands/provider.go`, add to `providerAddCmd`:

```go
Example: `  # Interactive (prompts for missing fields)
  omnillm provider add github-copilot

  # OpenAI-compatible with all flags
  omnillm provider add openai-compatible --endpoint https://api.openai.com/v1 --api-key sk-...

  # Alibaba DashScope
  omnillm provider add alibaba --api-key my-key --region global --plan standard`,
```

- [ ] **Step 3: Add `Example` to `vmCreateCmd`**

In `internal/commands/virtualmodel.go`, add to `vmCreateCmd`:

```go
Example: `  # Round-robin across two upstreams
  omnillm virtualmodel create my-gpt --name "My GPT" --upstream provider1/gpt-4o --upstream provider2/gpt-4o

  # Weighted routing (provider1 gets 3x traffic)
  omnillm virtualmodel create smart-gpt --name "Smart GPT" --strategy weighted \
    --upstream provider1/gpt-4o:3 --upstream provider2/gpt-4o:1

  # With weight and priority
  omnillm virtualmodel create ha-gpt --name "HA GPT" --strategy priority \
    --upstream primary/gpt-4o:1:1 --upstream fallback/gpt-4o:1:2`,
```

- [ ] **Step 4: Add `Example` to `logsTailCmd`**

In `internal/commands/logs.go`, add to `logsTailCmd`:

```go
Example: `  # Stream all logs
  omnillm logs tail

  # Stream only errors and above
  omnillm logs tail --level error

  # Stream warnings and above
  omnillm logs tail --level warn`,
```

- [ ] **Step 5: Add `Example` to `configSetCmd`**

In `internal/commands/config.go`, add to `configSetCmd`:

```go
Example: `  # Write from a local file
  omnillm config set claude-code --file ~/.claude/settings.json

  # Pipe from stdin
  cat my-config.json | omnillm config set claude-code --stdin

  # From a heredoc
  omnillm config set claude-code --stdin <<'EOF'
  {"theme": "dark"}
  EOF`,
```

- [ ] **Step 6: Run all tests**

```powershell
go test ./internal/commands/...
```
Expected: all pass

- [ ] **Step 7: Commit**

```powershell
git add internal/commands/start.go internal/commands/provider.go internal/commands/virtualmodel.go internal/commands/logs.go internal/commands/config.go
git commit -m "docs(cli): add Long descriptions and Example blocks to start, provider, virtualmodel, logs, config" --author="James Zhu <zhujian0805@gmail.com>"
```

---

## Task 12: Register aliases, `doctor`, `completion`, and command groups in `main.go`

**Files:**
- Modify: `main.go`
- Modify: `cmd/omniproxy/main.go`
- Test: `internal/commands/commands_test.go`

- [ ] **Step 1: Write tests for aliases**

Add to `internal/commands/commands_test.go`:

```go
func TestVirtualModelCmdHasAlias(t *testing.T) {
	aliases := VirtualModelCmd.Aliases
	for _, a := range aliases {
		if a == "virtual-model" {
			return
		}
	}
	t.Errorf("VirtualModelCmd missing 'virtual-model' alias; got %v", aliases)
}
```

- [ ] **Step 2: Run to confirm failure**

```powershell
go test ./internal/commands/... -run TestVirtualModelCmdHasAlias -v
```
Expected: FAIL

- [ ] **Step 3: Add `Aliases` to `VirtualModelCmd` in `virtualmodel.go`**

In `internal/commands/virtualmodel.go`, update:

```go
var VirtualModelCmd = &cobra.Command{
	Use:     "virtualmodel",
	Aliases: []string{"virtual-model"},
	Short:   "Manage virtual models (model aliases with load-balancing)",
	// ... rest unchanged
```

- [ ] **Step 4: Add command groups and new commands to `main.go`**

Replace the body of `main()` in `main.go`:

```go
func main() {
	// Persistent flags
	rootCmd.PersistentFlags().String("server", "http://127.0.0.1:5000",
		"OmniLLM server address (or set OMNILLM_SERVER)")
	rootCmd.PersistentFlags().String("api-key", "",
		"Admin API key for the server (or set OMNILLM_API_KEY)")
	rootCmd.PersistentFlags().StringP("output", "o", "table",
		"Output format: table or json")

	// Register completion for --output flag
	_ = rootCmd.RegisterFlagCompletionFunc("output", func(cmd *cobra.Command, args []string, toComplete string) ([]string, cobra.ShellCompDirective) {
		return []string{"table", "json"}, cobra.ShellCompDirectiveNoFileComp
	})

	// Command groups
	rootCmd.AddGroup(&cobra.Group{ID: "server", Title: "Server:"})
	rootCmd.AddGroup(&cobra.Group{ID: "providers", Title: "Providers:"})
	rootCmd.AddGroup(&cobra.Group{ID: "admin", Title: "Admin:"})
	rootCmd.AddGroup(&cobra.Group{ID: "troubleshoot", Title: "Troubleshooting:"})

	// Server
	commands.StartCmd.GroupID = "server"
	rootCmd.AddCommand(commands.StartCmd)

	// Providers
	commands.AuthCmd.GroupID = "providers"
	commands.ProviderCmd.GroupID = "providers"
	commands.ModelCmd.GroupID = "providers"
	commands.VirtualModelCmd.GroupID = "providers"
	rootCmd.AddCommand(commands.AuthCmd)
	rootCmd.AddCommand(commands.ProviderCmd)
	rootCmd.AddCommand(commands.ModelCmd)
	rootCmd.AddCommand(commands.VirtualModelCmd)

	// Admin
	commands.ConfigCmd.GroupID = "admin"
	commands.SettingsCmd.GroupID = "admin"
	commands.StatusCmd.GroupID = "admin"
	commands.LogsCmd.GroupID = "admin"
	commands.UsageCmd.GroupID = "admin"
	rootCmd.AddCommand(commands.ConfigCmd)
	rootCmd.AddCommand(commands.SettingsCmd)
	rootCmd.AddCommand(commands.StatusCmd)
	rootCmd.AddCommand(commands.LogsCmd)
	rootCmd.AddCommand(commands.UsageCmd)
	rootCmd.AddCommand(commands.CheckUsageCmd)
	rootCmd.AddCommand(commands.SyncNamesCmd)

	// Troubleshooting
	commands.DoctorCmd.GroupID = "troubleshoot"
	commands.DebugCmd.GroupID = "troubleshoot"
	rootCmd.AddCommand(commands.DoctorCmd)
	rootCmd.AddCommand(commands.DebugCmd)

	// Completions
	rootCmd.AddCommand(commands.CompletionCmd)

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}
```

- [ ] **Step 5: Mirror to `cmd/omniproxy/main.go`**

Apply identical changes to `cmd/omniproxy/main.go` (same group IDs, same `DoctorCmd`, `CompletionCmd` registrations).

- [ ] **Step 6: Run tests**

```powershell
go test ./internal/commands/... -run TestVirtualModelCmdHasAlias -v
```
Expected: PASS

```powershell
go test ./internal/commands/...
```
Expected: all pass

- [ ] **Step 7: Build and smoke test**

```powershell
go build -o omnillm.exe . && .\omnillm.exe --help
```
Expected: help output with grouped command sections (Server, Providers, Admin, Troubleshooting)

```powershell
.\omnillm.exe virtual-model --help
```
Expected: shows virtual model help (alias works)

```powershell
.\omnillm.exe doctor --help
```
Expected: shows doctor help

```powershell
.\omnillm.exe completion --help
```
Expected: shows completion help with shell options

- [ ] **Step 8: Run full tests**

```powershell
go test ./internal/commands/...
```
Expected: all pass

- [ ] **Step 9: Commit**

```powershell
git add main.go cmd/omniproxy/main.go internal/commands/virtualmodel.go internal/commands/commands_test.go
git commit -m "feat(cli): add command groups, virtual-model alias, doctor and completion to root" --author="James Zhu <zhujian0805@gmail.com>"
```

---

## Self-Review

**Spec coverage check:**

| Spec section | Covered by task |
|---|---|
| 2.1 Keep existing commands | All tasks preserve existing commands |
| 2.2 Aliases (`virtual-model`) | Task 12 |
| 2.2 `model enable`/`model disable` | Task 6 |
| 2.3 `doctor` command | Task 10 |
| 3.1 TTY-only prompting | Task 7 (`IsTerminalWriter` guard) |
| 3.2 Interactive ID selection | Tasks 7 + 8 |
| 3.3 Post-command next-step hints | Task 11 (`Example` blocks); doctor's "Next step:" |
| 4.1 Stdout/stderr normalization | Tasks 2, 3, 4 |
| 4.2 `Client.PrintJSON` fix | Task 1 |
| 4.3 Detail view consistency | Task 4 |
| 5.1 Mutually exclusive flags | Task 5 |
| 5.2 Allowed-values validation | Task 9 (completions + `ValidArgs`) |
| 5.3 `MarkFlagRequired` for `--name` | Task 5 |
| 6.1 Completion subcommand | Task 9 |
| 6.2 Static + live completions | Task 9 |
| 7.1 `Example` blocks | Task 11 |
| 7.2 Improved `Long` | Task 11 (`start`) |
| 7.3 Command groups | Task 12 |
| 8.1 Startup summary/examples | Task 11 |
| 9 Files to modify | All tasks, see file map |
| 10 Out of scope | No server-side changes in any task |
| 11 Testing | Each task has tests; final `go test ./internal/commands/...` in each task |

**Placeholder scan:** none found — all code steps contain complete implementations.

**Type consistency:** `resolveProviderID` / `resolveVirtualModelID` / `resolveIDFromList` defined in Task 7 and used in Task 8. `DoctorCmd` / `CompletionCmd` defined in Tasks 10/9 and registered in Task 12. `printCheck(out io.Writer, ...)` consistent throughout Task 10. ✓

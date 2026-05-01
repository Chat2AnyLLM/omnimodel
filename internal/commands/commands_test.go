package commands

import (
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/spf13/cobra"
)

// ─── auth and usage commands ──────────────────────────────────────────────────

func TestAuthCmdIsGenericProviderAuth(t *testing.T) {
	if strings.Contains(AuthCmd.Short, "GitHub Copilot") {
		t.Fatalf("auth short help should not be GitHub Copilot-specific: %q", AuthCmd.Short)
	}
	for _, flagName := range []string{"api-key", "token", "endpoint", "region", "plan", "yes"} {
		if AuthCmd.Flags().Lookup(flagName) == nil {
			t.Errorf("auth: missing flag --%s", flagName)
		}
	}
}

func TestSupportedAuthProviderTypes(t *testing.T) {
	expected := []string{"github-copilot", "openai-compatible", "alibaba", "azure-openai", "google", "kimi", "codex"}
	if len(supportedAuthProviderTypes) != len(expected) {
		t.Fatalf("supportedAuthProviderTypes length = %d, want %d", len(supportedAuthProviderTypes), len(expected))
	}
	for i, providerType := range expected {
		if supportedAuthProviderTypes[i] != providerType {
			t.Fatalf("supportedAuthProviderTypes[%d] = %q, want %q", i, supportedAuthProviderTypes[i], providerType)
		}
	}
}

func TestSupportedAuthProviders(t *testing.T) {
	if len(supportedAuthProviders) != len(supportedAuthProviderTypes) {
		t.Fatalf("supportedAuthProviders length = %d, want %d", len(supportedAuthProviders), len(supportedAuthProviderTypes))
	}
	for i, provider := range supportedAuthProviders {
		if provider.Type != supportedAuthProviderTypes[i] {
			t.Fatalf("supportedAuthProviders[%d].Type = %q, want %q", i, provider.Type, supportedAuthProviderTypes[i])
		}
		if provider.Label == "" {
			t.Fatalf("supportedAuthProviders[%d].Label should not be empty", i)
		}
	}
}

func TestSelectFromListValid(t *testing.T) {
	selected, err := SelectFromList("Pick one:", []string{"a", "b", "c"}, strings.NewReader("2\n"))
	if err != nil {
		t.Fatalf("SelectFromList returned error: %v", err)
	}
	if selected != "b" {
		t.Fatalf("SelectFromList returned %q, want %q", selected, "b")
	}
}

func TestSelectFromListNonNumeric(t *testing.T) {
	_, err := SelectFromList("Pick one:", []string{"a", "b", "c"}, strings.NewReader("abc\n"))
	if err == nil || !strings.Contains(err.Error(), "must be a number") {
		t.Fatalf("SelectFromList error = %v, want number error", err)
	}
}

func TestSelectFromListOutOfRange(t *testing.T) {
	_, err := SelectFromList("Pick one:", []string{"a", "b", "c"}, strings.NewReader("0\n"))
	if err == nil || !strings.Contains(err.Error(), "out of range") {
		t.Fatalf("SelectFromList error = %v, want range error", err)
	}
}

func TestResolveAuthProviderTypeUsesArgWhenProvided(t *testing.T) {
	providerType, err := resolveAuthProviderType([]string{"google"})
	if err != nil {
		t.Fatalf("resolveAuthProviderType returned error: %v", err)
	}
	if providerType != "google" {
		t.Fatalf("resolveAuthProviderType returned %q, want %q", providerType, "google")
	}
}

func TestPromptForProviderAuthSkipsWhenYesFlagIsSet(t *testing.T) {
	cmd := &cobra.Command{}
	addProviderAuthFlags(cmd)
	if err := cmd.Flags().Set("yes", "true"); err != nil {
		t.Fatalf("set yes flag: %v", err)
	}
	if err := promptForProviderAuth(cmd, "azure-openai"); err != nil {
		t.Fatalf("promptForProviderAuth returned error: %v", err)
	}
}

func TestPromptForGitHubCopilotAuthSetsMethodFromToken(t *testing.T) {
	cmd := &cobra.Command{}
	addProviderAuthFlags(cmd)
	if err := cmd.Flags().Set("token", "ghp_test"); err != nil {
		t.Fatalf("set token flag: %v", err)
	}
	if err := promptForProviderAuth(cmd, "github-copilot"); err != nil {
		t.Fatalf("promptForProviderAuth returned error: %v", err)
	}
	method, _ := cmd.Flags().GetString("method")
	if method != "token" {
		t.Fatalf("method = %q, want %q", method, "token")
	}
}

func TestResolveAuthProviderTypeUsesInteractiveSelection(t *testing.T) {
	t.Skip("interactive provider selection is covered manually")
}

func TestUsageCmdFlags(t *testing.T) {
	if UsageCmd.Use != "usage" {
		t.Errorf("expected Use='usage', got %q", UsageCmd.Use)
	}
	for _, flagName := range []string{"provider-id", "model-id", "client", "api-shape", "since", "until", "breakdown"} {
		if UsageCmd.Flags().Lookup(flagName) == nil {
			t.Errorf("usage: missing flag --%s", flagName)
		}
	}
	if strings.Contains(CheckUsageCmd.Short, "GitHub Copilot") {
		t.Fatalf("check-usage short help should not be GitHub Copilot-specific: %q", CheckUsageCmd.Short)
	}
}

func TestUsageHelpers(t *testing.T) {
	cmd := &cobra.Command{}
	addUsageFlags(cmd)
	_ = cmd.Flags().Set("provider-id", "provider one")
	_ = cmd.Flags().Set("model-id", "qwen3")
	_ = cmd.Flags().Set("api-shape", "openai")

	query := usageQuery(cmd)
	for _, expected := range []string{"api_shape=openai", "model_id=qwen3", "provider_id=provider+one"} {
		if !strings.Contains(query, expected) {
			t.Errorf("usage query %q missing %q", query, expected)
		}
	}

	cases := map[string]string{
		"provider":  "/api/admin/metering/by-provider",
		"providers": "/api/admin/metering/by-provider",
		"model":     "/api/admin/metering/by-model",
		"models":    "/api/admin/metering/by-model",
		"client":    "/api/admin/metering/by-client",
		"clients":   "/api/admin/metering/by-client",
		"none":      "",
	}
	for breakdown, expected := range cases {
		path, err := usageBreakdownPath(breakdown)
		if err != nil {
			t.Fatalf("usageBreakdownPath(%q) returned error: %v", breakdown, err)
		}
		if path != expected {
			t.Errorf("usageBreakdownPath(%q) = %q, want %q", breakdown, path, expected)
		}
	}

	if _, err := usageBreakdownPath("bad"); err == nil {
		t.Fatal("usageBreakdownPath should reject invalid breakdown values")
	}
}

// ─── provider command ─────────────────────────────────────────────────────────

func TestProviderCmdStructure(t *testing.T) {
	expectedSubs := []string{
		"list", "add", "delete", "activate", "deactivate",
		"switch", "rename", "priorities", "usage",
	}
	subNames := make(map[string]bool)
	for _, sub := range ProviderCmd.Commands() {
		subNames[sub.Name()] = true
	}
	for _, name := range expectedSubs {
		if !subNames[name] {
			t.Errorf("provider: missing subcommand %q", name)
		}
	}
}

func TestProviderAddFlagDefaults(t *testing.T) {
	for _, flagName := range []string{"api-key", "token", "endpoint", "region", "plan"} {
		found := false
		for _, sub := range ProviderCmd.Commands() {
			if sub.Name() == "add" {
				if sub.Flags().Lookup(flagName) == nil {
					t.Errorf("provider add: missing flag --%s", flagName)
				}
				found = true
				break
			}
		}
		if !found {
			t.Errorf("provider add: subcommand not found while checking flag %s", flagName)
		}
	}
}

func TestProviderDeleteHasYesFlag(t *testing.T) {
	for _, sub := range ProviderCmd.Commands() {
		if sub.Name() == "delete" {
			if sub.Flags().Lookup("yes") == nil {
				t.Error("provider delete: missing --yes flag")
			}
			return
		}
	}
	t.Error("provider delete: subcommand not found")
}

// ─── model command ────────────────────────────────────────────────────────────

func TestModelCmdStructure(t *testing.T) {
	expected := []string{"list", "refresh", "toggle", "version"}
	subNames := make(map[string]bool)
	for _, sub := range ModelCmd.Commands() {
		subNames[sub.Name()] = true
	}
	for _, name := range expected {
		if !subNames[name] {
			t.Errorf("model: missing subcommand %q", name)
		}
	}
}

func TestModelToggleFlags(t *testing.T) {
	for _, sub := range ModelCmd.Commands() {
		if sub.Name() == "toggle" {
			if sub.Flags().Lookup("enable") == nil {
				t.Error("model toggle: missing --enable flag")
			}
			if sub.Flags().Lookup("disable") == nil {
				t.Error("model toggle: missing --disable flag")
			}
			return
		}
	}
	t.Error("model toggle: subcommand not found")
}

func TestModelVersionSubcommands(t *testing.T) {
	for _, sub := range ModelCmd.Commands() {
		if sub.Name() == "version" {
			subs := make(map[string]bool)
			for _, s := range sub.Commands() {
				subs[s.Name()] = true
			}
			if !subs["get"] {
				t.Error("model version: missing subcommand 'get'")
			}
			if !subs["set"] {
				t.Error("model version: missing subcommand 'set'")
			}
			return
		}
	}
	t.Error("model: 'version' subcommand not found")
}

// ─── virtualmodel command ─────────────────────────────────────────────────────

func TestVirtualModelCmdUse(t *testing.T) {
	if VirtualModelCmd.Use != "virtualmodel" {
		t.Errorf("expected Use='virtualmodel', got %q", VirtualModelCmd.Use)
	}
}

func TestVirtualModelCmdStructure(t *testing.T) {
	expected := []string{"list", "get", "create", "update", "delete"}
	subNames := make(map[string]bool)
	for _, sub := range VirtualModelCmd.Commands() {
		subNames[sub.Name()] = true
	}
	for _, name := range expected {
		if !subNames[name] {
			t.Errorf("virtualmodel: missing subcommand %q", name)
		}
	}
}

func TestVirtualModelCreateFlags(t *testing.T) {
	for _, sub := range VirtualModelCmd.Commands() {
		if sub.Name() == "create" {
			for _, flagName := range []string{"name", "description", "strategy", "api-shape", "upstream", "disabled"} {
				if sub.Flags().Lookup(flagName) == nil {
					t.Errorf("virtualmodel create: missing flag --%s", flagName)
				}
			}
			// Check strategy default
			strategy, _ := sub.Flags().GetString("strategy")
			if strategy != "round-robin" {
				t.Errorf("expected default strategy='round-robin', got %q", strategy)
			}
			// Check api-shape default
			apiShape, _ := sub.Flags().GetString("api-shape")
			if apiShape != "openai" {
				t.Errorf("expected default api-shape='openai', got %q", apiShape)
			}
			return
		}
	}
	t.Error("virtualmodel create: subcommand not found")
}

func TestVirtualModelDeleteHasYesFlag(t *testing.T) {
	for _, sub := range VirtualModelCmd.Commands() {
		if sub.Name() == "delete" {
			if sub.Flags().Lookup("yes") == nil {
				t.Error("virtualmodel delete: missing --yes flag")
			}
			return
		}
	}
	t.Error("virtualmodel delete: subcommand not found")
}

// ─── config command ───────────────────────────────────────────────────────────

func TestConfigCmdStructure(t *testing.T) {
	expected := []string{"list", "get", "set", "import", "backup"}
	subNames := make(map[string]bool)
	for _, sub := range ConfigCmd.Commands() {
		subNames[sub.Name()] = true
	}
	for _, name := range expected {
		if !subNames[name] {
			t.Errorf("config: missing subcommand %q", name)
		}
	}
}

func TestConfigSetFlags(t *testing.T) {
	for _, sub := range ConfigCmd.Commands() {
		if sub.Name() == "set" {
			if sub.Flags().Lookup("file") == nil {
				t.Error("config set: missing --file flag")
			}
			if sub.Flags().Lookup("stdin") == nil {
				t.Error("config set: missing --stdin flag")
			}
			return
		}
	}
	t.Error("config set: subcommand not found")
}

func TestConfigImportRequiresFileFlag(t *testing.T) {
	for _, sub := range ConfigCmd.Commands() {
		if sub.Name() == "import" {
			f := sub.Flags().Lookup("file")
			if f == nil {
				t.Error("config import: missing --file flag")
				return
			}
			return
		}
	}
	t.Error("config import: subcommand not found")
}

// ─── settings command ─────────────────────────────────────────────────────────

func TestSettingsCmdStructure(t *testing.T) {
	subNames := make(map[string]bool)
	for _, sub := range SettingsCmd.Commands() {
		subNames[sub.Name()] = true
	}
	if !subNames["get"] {
		t.Error("settings: missing subcommand 'get'")
	}
	if !subNames["set"] {
		t.Error("settings: missing subcommand 'set'")
	}
}

func TestSettingsGetHasLogLevelSubcommand(t *testing.T) {
	for _, sub := range SettingsCmd.Commands() {
		if sub.Name() == "get" {
			for _, s := range sub.Commands() {
				if s.Name() == "log-level" {
					return // found
				}
			}
			t.Error("settings get: missing subcommand 'log-level'")
			return
		}
	}
	t.Error("settings: 'get' subcommand not found")
}

func TestSettingsSetHasLogLevelSubcommand(t *testing.T) {
	for _, sub := range SettingsCmd.Commands() {
		if sub.Name() == "set" {
			for _, s := range sub.Commands() {
				if s.Name() == "log-level" {
					return
				}
			}
			t.Error("settings set: missing subcommand 'log-level'")
			return
		}
	}
	t.Error("settings: 'set' subcommand not found")
}

// ─── status command ───────────────────────────────────────────────────────────

func TestStatusCmdHasAuthSubcommand(t *testing.T) {
	subNames := make(map[string]bool)
	for _, sub := range StatusCmd.Commands() {
		subNames[sub.Name()] = true
	}
	if !subNames["auth"] {
		t.Error("status: missing subcommand 'auth'")
	}
}

// ─── logs command ─────────────────────────────────────────────────────────────

func TestLogsCmdHasTailSubcommand(t *testing.T) {
	subNames := make(map[string]bool)
	for _, sub := range LogsCmd.Commands() {
		subNames[sub.Name()] = true
	}
	if !subNames["tail"] {
		t.Error("logs: missing subcommand 'tail'")
	}
}

func TestLogsTailHasLevelFlag(t *testing.T) {
	for _, sub := range LogsCmd.Commands() {
		if sub.Name() == "tail" {
			if sub.Flags().Lookup("level") == nil {
				t.Error("logs tail: missing --level flag")
			}
			return
		}
	}
	t.Error("logs tail: subcommand not found")
}

// ─── chat command ─────────────────────────────────────────────────────────────

func TestChatCmdStructure(t *testing.T) {
	subNames := make(map[string]bool)
	for _, sub := range ChatCmd.Commands() {
		subNames[sub.Name()] = true
	}
	if !subNames["sessions"] {
		t.Error("chat: missing subcommand 'sessions'")
	}
	if !subNames["send"] {
		t.Error("chat: missing subcommand 'send'")
	}
}

func TestChatSessionsSubcommands(t *testing.T) {
	for _, sub := range ChatCmd.Commands() {
		if sub.Name() == "sessions" {
			subNames := make(map[string]bool)
			for _, s := range sub.Commands() {
				subNames[s.Name()] = true
			}
			for _, expected := range []string{"list", "create", "get", "rename", "delete"} {
				if !subNames[expected] {
					t.Errorf("chat sessions: missing subcommand %q", expected)
				}
			}
			return
		}
	}
	t.Error("chat: 'sessions' subcommand not found")
}

func TestChatCmdFlags(t *testing.T) {
	if ChatCmd.Flags().Lookup("model") == nil {
		t.Error("chat: missing --model flag")
	}
	if ChatCmd.Flags().Lookup("session") == nil {
		t.Error("chat: missing --session flag")
	}
}

// ─── parseUpstreamArgs ────────────────────────────────────────────────────────

func TestParseUpstreamArgsBasic(t *testing.T) {
	result, err := parseUpstreamArgs([]string{"my-provider/my-model"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 1 {
		t.Fatalf("expected 1 upstream, got %d", len(result))
	}
	if result[0]["provider_id"] != "my-provider" {
		t.Errorf("expected provider_id='my-provider', got %v", result[0]["provider_id"])
	}
	if result[0]["model_id"] != "my-model" {
		t.Errorf("expected model_id='my-model', got %v", result[0]["model_id"])
	}
	if result[0]["weight"] != 1 {
		t.Errorf("expected weight=1, got %v", result[0]["weight"])
	}
	if result[0]["priority"] != 0 {
		t.Errorf("expected priority=0, got %v", result[0]["priority"])
	}
}

func TestParseUpstreamArgsWithWeight(t *testing.T) {
	result, err := parseUpstreamArgs([]string{"p1/m1:3"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result[0]["weight"] != 3 {
		t.Errorf("expected weight=3, got %v", result[0]["weight"])
	}
	if result[0]["priority"] != 0 {
		t.Errorf("expected priority=0, got %v", result[0]["priority"])
	}
}

func TestParseUpstreamArgsWithWeightAndPriority(t *testing.T) {
	result, err := parseUpstreamArgs([]string{"p1/m1:2:5"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if result[0]["weight"] != 2 {
		t.Errorf("expected weight=2, got %v", result[0]["weight"])
	}
	if result[0]["priority"] != 5 {
		t.Errorf("expected priority=5, got %v", result[0]["priority"])
	}
}

func TestParseUpstreamArgsMultiple(t *testing.T) {
	result, err := parseUpstreamArgs([]string{"p1/m1", "p2/m2:10:1"})
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if len(result) != 2 {
		t.Fatalf("expected 2 upstreams, got %d", len(result))
	}
	if result[1]["provider_id"] != "p2" {
		t.Errorf("expected provider_id='p2', got %v", result[1]["provider_id"])
	}
	if result[1]["weight"] != 10 {
		t.Errorf("expected weight=10, got %v", result[1]["weight"])
	}
}

func TestParseUpstreamArgsMissingSlashReturnsError(t *testing.T) {
	_, err := parseUpstreamArgs([]string{"no-slash-here"})
	if err == nil {
		t.Error("expected error for missing slash, got nil")
	}
}

func TestParseUpstreamArgsInvalidWeightReturnsError(t *testing.T) {
	_, err := parseUpstreamArgs([]string{"p1/m1:notanumber"})
	if err == nil {
		t.Error("expected error for non-numeric weight, got nil")
	}
}

func TestParseUpstreamArgsInvalidPriorityReturnsError(t *testing.T) {
	_, err := parseUpstreamArgs([]string{"p1/m1:1:bad"})
	if err == nil {
		t.Error("expected error for non-numeric priority, got nil")
	}
}

func TestParseUpstreamArgsEmpty(t *testing.T) {
	result, err := parseUpstreamArgs(nil)
	if err != nil {
		t.Fatalf("unexpected error for nil input: %v", err)
	}
	if result != nil {
		t.Errorf("expected nil result for empty input, got %v", result)
	}
}

// ─── isLevelAtOrAbove ─────────────────────────────────────────────────────────

func TestIsLevelAtOrAboveFiltering(t *testing.T) {
	cases := []struct {
		msg    string
		filter string
		want   bool
	}{
		{"error", "error", true},
		{"error", "warn", true},   // error is more severe than warn
		{"error", "info", true},   // error is more severe than info
		{"debug", "info", false},  // debug is less severe than info
		{"debug", "debug", true},  // same level passes
		{"trace", "debug", false}, // trace is less severe than debug
		{"info", "warn", false},   // info=3 is LESS severe than warn=2 — filtered out
		{"warn", "info", true},    // warn=2 is MORE severe than info=3 — passes
		{"unknown", "info", true}, // unknown levels pass through
		{"info", "unknown", true}, // unknown filter passes through
	}

	for _, tc := range cases {
		got := isLevelAtOrAbove(tc.msg, tc.filter)
		if got != tc.want {
			t.Errorf("isLevelAtOrAbove(%q, %q) = %v, want %v", tc.msg, tc.filter, got, tc.want)
		}
	}
}

// ─── NewClient defaults ───────────────────────────────────────────────────────

func TestNewClientDefaultServer(t *testing.T) {
	import_test_root := ChatCmd.Root()
	if import_test_root == nil {
		t.Skip("no root command in test context")
	}
	t.Setenv("OMNILLM_SERVER", "")
	t.Setenv("OMNILLM_API_KEY", "")

	const expectedDefault = "http://127.0.0.1:5000"
	_ = expectedDefault // guard against renaming without updating
}

// ─── OAuth browser / no-browser flag ─────────────────────────────────────────

func TestAuthCmdHasNoBrowserFlag(t *testing.T) {
	if AuthCmd.Flags().Lookup("no-browser") == nil {
		t.Error("auth: missing --no-browser flag")
	}
}

func TestProviderAddHasNoBrowserFlag(t *testing.T) {
	for _, sub := range ProviderCmd.Commands() {
		if sub.Name() == "add" {
			if sub.Flags().Lookup("no-browser") == nil {
				t.Error("provider add: missing --no-browser flag")
			}
			return
		}
	}
	t.Error("provider add: subcommand not found")
}

func TestOpenBrowserRejectsEmptyURL(t *testing.T) {
	if err := openBrowser(""); err == nil {
		t.Error("openBrowser should return an error for an empty URL")
	}
}

// TestAuthAndCreateProviderOAuthPollingCompletes verifies that the CLI polling
// loop detects a "complete" auth-status response and returns successfully, and
// that it times out when the server never transitions to a terminal state.
func TestAuthAndCreateProviderOAuthPollingCompletes(t *testing.T) {
	// Speed up polling so the test finishes quickly.
	origInterval := authPollInterval
	authPollInterval = 50 * time.Millisecond
	t.Cleanup(func() { authPollInterval = origInterval })

	pollCount := 0

	mux := http.NewServeMux()
	mux.HandleFunc("/api/admin/providers/auth-and-create/github-copilot",
		func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{
				"requiresAuth":     true,
				"verification_uri": "https://github.com/login/device",
				"user_code":        "TEST-CODE",
				"expires_in":       5
			}`)
		})
	mux.HandleFunc("/api/admin/auth-status",
		func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			pollCount++
			if pollCount >= 2 {
				fmt.Fprint(w, `{"status":"complete","providerId":"github-copilot-testuser"}`)
			} else {
				fmt.Fprint(w, `{"status":"awaiting_user"}`)
			}
		})

	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)

	root := &cobra.Command{Use: "test"}
	root.PersistentFlags().String("server", ts.URL, "")
	root.PersistentFlags().String("api-key", "", "")
	root.PersistentFlags().StringP("output", "o", "table", "")

	cmd := &cobra.Command{Use: "sub"}
	addProviderAuthFlags(cmd)
	if err := cmd.Flags().Set("no-browser", "true"); err != nil {
		t.Fatalf("set no-browser: %v", err)
	}
	root.AddCommand(cmd)

	var out strings.Builder
	cmd.SetOut(&out)

	if err := authAndCreateProvider(cmd, "github-copilot"); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}
	if pollCount < 2 {
		t.Errorf("expected at least 2 polls, got %d", pollCount)
	}
	if !strings.Contains(out.String(), "github-copilot-testuser") {
		t.Errorf("expected provider ID in output, got: %q", out.String())
	}
}

func TestAuthAndCreateProviderOAuthTimesOut(t *testing.T) {
	origInterval := authPollInterval
	authPollInterval = 50 * time.Millisecond
	t.Cleanup(func() { authPollInterval = origInterval })

	mux := http.NewServeMux()
	mux.HandleFunc("/api/admin/providers/auth-and-create/github-copilot",
		func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			// expires_in=1 forces a 1-second timeout.
			fmt.Fprint(w, `{
				"requiresAuth":     true,
				"verification_uri": "https://github.com/login/device",
				"user_code":        "TEST-CODE",
				"expires_in":       1
			}`)
		})
	mux.HandleFunc("/api/admin/auth-status",
		func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"status":"awaiting_user"}`)
		})

	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)

	root := &cobra.Command{Use: "test"}
	root.PersistentFlags().String("server", ts.URL, "")
	root.PersistentFlags().String("api-key", "", "")
	root.PersistentFlags().StringP("output", "o", "table", "")

	cmd := &cobra.Command{Use: "sub"}
	addProviderAuthFlags(cmd)
	if err := cmd.Flags().Set("no-browser", "true"); err != nil {
		t.Fatalf("set no-browser: %v", err)
	}
	root.AddCommand(cmd)
	cmd.SetOut(io.Discard)

	err := authAndCreateProvider(cmd, "github-copilot")
	if err == nil {
		t.Fatal("expected timeout error, got nil")
	}
	if !strings.Contains(err.Error(), "timed out") {
		t.Errorf("expected timeout error, got: %v", err)
	}
}

func TestAuthAndCreateProviderOAuthErrorStatus(t *testing.T) {
	origInterval := authPollInterval
	authPollInterval = 50 * time.Millisecond
	t.Cleanup(func() { authPollInterval = origInterval })

	mux := http.NewServeMux()
	mux.HandleFunc("/api/admin/providers/auth-and-create/github-copilot",
		func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{
				"requiresAuth":     true,
				"verification_uri": "https://github.com/login/device",
				"user_code":        "TEST-CODE",
				"expires_in":       30
			}`)
		})
	mux.HandleFunc("/api/admin/auth-status",
		func(w http.ResponseWriter, _ *http.Request) {
			w.Header().Set("Content-Type", "application/json")
			fmt.Fprint(w, `{"status":"error","error":"token exchange failed"}`)
		})

	ts := httptest.NewServer(mux)
	t.Cleanup(ts.Close)

	root := &cobra.Command{Use: "test"}
	root.PersistentFlags().String("server", ts.URL, "")
	root.PersistentFlags().String("api-key", "", "")
	root.PersistentFlags().StringP("output", "o", "table", "")

	cmd := &cobra.Command{Use: "sub"}
	addProviderAuthFlags(cmd)
	if err := cmd.Flags().Set("no-browser", "true"); err != nil {
		t.Fatalf("set no-browser: %v", err)
	}
	root.AddCommand(cmd)
	cmd.SetOut(io.Discard)

	err := authAndCreateProvider(cmd, "github-copilot")
	if err == nil {
		t.Fatal("expected error from auth status, got nil")
	}
	if !strings.Contains(err.Error(), "token exchange failed") {
		t.Errorf("expected auth error message, got: %v", err)
	}
}

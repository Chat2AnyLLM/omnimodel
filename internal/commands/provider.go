package commands

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

type authProviderOption struct {
	Type  string
	Label string
}

type providerPromptField struct {
	FlagName   string
	Label      string
	Secret     bool
	Required   bool
	Options    []string
	AllowEmpty bool
	Default    string
}

var supportedAuthProviders = []authProviderOption{
	{Type: "github-copilot", Label: "GitHub Copilot"},
	{Type: "openai-compatible", Label: "OpenAI-Compatible"},
	{Type: "alibaba", Label: "Alibaba DashScope"},
	{Type: "azure-openai", Label: "Azure OpenAI"},
	{Type: "google", Label: "Google AI"},
	{Type: "kimi", Label: "Kimi"},
	{Type: "codex", Label: "OpenAI Codex"},
}

var supportedAuthProviderTypes = []string{
	"github-copilot",
	"openai-compatible",
	"alibaba",
	"azure-openai",
	"google",
	"kimi",
	"codex",
}

const supportedAuthProviderTypesSummary = "github-copilot, openai-compatible, alibaba, azure-openai, google, kimi, and codex"

var ProviderCmd = &cobra.Command{
	Use:   "provider",
	Short: "Manage LLM providers",
	Long:  "List, add, remove, and configure LLM provider instances.",
}

func init() {
	// provider list
	ProviderCmd.AddCommand(providerListCmd)

	// provider add
	addProviderAuthFlags(providerAddCmd)
	ProviderCmd.AddCommand(providerAddCmd)

	// provider delete
	providerDeleteCmd.Flags().BoolP("yes", "y", false, "Skip confirmation")
	ProviderCmd.AddCommand(providerDeleteCmd)

	// provider activate / deactivate / switch
	ProviderCmd.AddCommand(providerActivateCmd)
	ProviderCmd.AddCommand(providerDeactivateCmd)
	ProviderCmd.AddCommand(providerSwitchCmd)

	// provider rename
	providerRenameCmd.Flags().String("name", "", "New display name")
	providerRenameCmd.Flags().String("subtitle", "", "New subtitle")
	ProviderCmd.AddCommand(providerRenameCmd)

	// provider priorities
	providerPrioritiesCmd.Flags().StringSlice("set", nil, "Set priorities: id:N,... (repeatable)")
	ProviderCmd.AddCommand(providerPrioritiesCmd)

	// provider usage
	ProviderCmd.AddCommand(providerUsageCmd)

	for _, sub := range []*cobra.Command{
		providerDeleteCmd, providerActivateCmd, providerDeactivateCmd,
		providerSwitchCmd, providerRenameCmd, providerUsageCmd,
	} {
		sub.ValidArgsFunction = providerIDCompletionFunc
	}
	providerAddCmd.ValidArgs = supportedAuthProviderTypes
}

// ─── Helpers ──────────────────────────────────────────────────────────────────

// parseProviders unmarshals a provider list from a JSON response.
func parseProviders(data []byte) ([]map[string]interface{}, error) {
	var providers []map[string]interface{}
	if err := json.Unmarshal(data, &providers); err != nil {
		return nil, fmt.Errorf("parse response: %w", err)
	}
	return providers, nil
}

// getStringFlag returns a string flag value, silently ignoring errors.
func getStringFlag(cmd *cobra.Command, name string) string {
	v, _ := cmd.Flags().GetString(name)
	return v
}

// getStringSliceFlag returns a string slice flag value, silently ignoring errors.
func getStringSliceFlag(cmd *cobra.Command, name string) []string {
	v, _ := cmd.Flags().GetStringSlice(name)
	return v
}

// getBoolFlag returns a bool flag value, silently ignoring errors.
func getBoolFlag(cmd *cobra.Command, name string) bool {
	v, _ := cmd.Flags().GetBool(name)
	return v
}

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

// ─── list ─────────────────────────────────────────────────────────────────────

var providerListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all configured providers",
	RunE: func(cmd *cobra.Command, args []string) error {
		c := NewClient(cmd)
		data, err := c.Get("/api/admin/providers")
		if err != nil {
			return err
		}
		if c.IsJSON() {
			c.PrintJSON(data)
			return nil
		}

		providers, err := parseProviders(data)
		if err != nil {
			return err
		}

		if len(providers) == 0 {
			return PrintEmpty(cmd.OutOrStdout(), "providers configured")
		}

		table := NewTable("ID", "TYPE", "NAME", "AUTH", "ACTIVE", "MODELS")
		table.SetMaxWidth(0, 32) // ID
		table.SetMaxWidth(2, 36) // NAME
		for _, p := range providers {
			id, _ := p["id"].(string)
			pType, _ := p["type"].(string)
			name, _ := p["name"].(string)
			auth, _ := p["authStatus"].(string)
			active := "no"
			if v, ok := p["isActive"].(bool); ok && v {
				active = "yes"
			}
			total, _ := p["totalModelCount"].(float64)
			enabled, _ := p["enabledModelCount"].(float64)
			models := fmt.Sprintf("%d/%d", int(enabled), int(total))
			table.AddRow(id, pType, name, auth, active, models)
		}
		return table.Render(cmd.OutOrStdout())
	},
}

// ─── add ──────────────────────────────────────────────────────────────────────

var providerAddCmd = &cobra.Command{
	Use:   "add <type>",
	Short: "Add and authenticate a new provider instance",
	Long: `Add a new provider instance. Supported types:
  github-copilot    GitHub Copilot (device-code OAuth or --token)
  openai-compatible Any OpenAI-compatible API (requires --endpoint and --api-key)
  alibaba           Alibaba DashScope (requires --api-key; optional --region, --plan)
  azure-openai      Azure OpenAI (requires --api-key)
  google            Google AI (requires --api-key)
  kimi              Kimi AI (requires --api-key)
  codex             OpenAI Codex (requires --api-key)`,
	Args: cobra.ExactArgs(1),
		Example: `  # Interactive (prompts for missing fields)
  omnillm provider add github-copilot

  # OpenAI-compatible with all flags
  omnillm provider add openai-compatible --endpoint https://api.openai.com/v1 --api-key sk-...

  # Alibaba DashScope
  omnillm provider add alibaba --api-key my-key --region global --plan standard`,
	RunE: func(cmd *cobra.Command, args []string) error {
		return authAndCreateProvider(cmd, args[0])
	},
}

func selectProviderTypeInteractive() (string, error) {
	return SelectAuthProvider("Select a provider to authenticate:", supportedAuthProviders)
}

func promptForProviderAuth(cmd *cobra.Command, providerType string) error {
	yes, _ := cmd.Flags().GetBool("yes")
	if yes {
		return nil
	}

	switch providerType {
	case "github-copilot":
		return promptForGitHubCopilotAuth(cmd)
	case "openai-compatible":
		return promptForMissingFields(cmd,
			providerPromptField{FlagName: "endpoint", Label: "Endpoint", Required: true},
			providerPromptField{FlagName: "api-key", Label: "API key", Secret: true},
		)
	case "alibaba":
		return promptForMissingFields(cmd,
			providerPromptField{FlagName: "api-key", Label: "API key", Secret: true, Required: true},
			providerPromptField{FlagName: "region", Label: "Region", Options: []string{"global", "china"}, Default: "global", AllowEmpty: true},
			providerPromptField{FlagName: "plan", Label: "Plan", Options: []string{"standard", "coding-plan"}, Default: "standard", AllowEmpty: true},
		)
	case "azure-openai":
		return promptForMissingFields(cmd,
			providerPromptField{FlagName: "api-key", Label: "API key", Secret: true, Required: true},
		)
	case "google", "kimi", "codex":
		return promptForMissingFields(cmd,
			providerPromptField{FlagName: "api-key", Label: "API key", Secret: true, Required: true},
		)
	default:
		return nil
	}
}

func promptForGitHubCopilotAuth(cmd *cobra.Command) error {
	token, _ := cmd.Flags().GetString("token")
	method, _ := cmd.Flags().GetString("method")
	if strings.TrimSpace(token) != "" {
		if strings.TrimSpace(method) == "" {
			return cmd.Flags().Set("method", "token")
		}
		return nil
	}

	if strings.TrimSpace(method) == "" {
		const (
			optOAuth = "Browser login"
			optToken = "Personal token"
		)
		selectedMethod, err := SelectFromOptions("Authenticate with GitHub Copilot using:", []string{optOAuth, optToken})
		if err != nil {
			return err
		}
		switch selectedMethod {
		case optToken:
			method = "token"
		default: // optOAuth
			method = "oauth"
		}
		if err := cmd.Flags().Set("method", method); err != nil {
			return err
		}
	}

	if method == "token" {
		return promptForMissingFields(cmd, providerPromptField{FlagName: "token", Label: "GitHub token", Secret: true, Required: true})
	}
	return nil
}

func promptForMissingFields(cmd *cobra.Command, fields ...providerPromptField) error {
	for _, field := range fields {
		currentValue, _ := cmd.Flags().GetString(field.FlagName)
		if strings.TrimSpace(currentValue) != "" {
			continue
		}

		var value string
		var err error
		if len(field.Options) > 0 {
			value, err = SelectFromOptions(field.Label, field.Options)
			if err != nil {
				return err
			}
		} else if field.Secret {
			value, err = PromptSecret(field.Label, field.Required)
			if err != nil {
				return err
			}
		} else {
			value, err = PromptText(field.Label, field.Required, field.Default)
			if err != nil {
				return err
			}
		}

		value = strings.TrimSpace(value)
		if value == "" && field.Default != "" && field.AllowEmpty {
			value = field.Default
		}
		if value == "" && field.Required {
			return fmt.Errorf("%s is required", strings.ToLower(field.Label))
		}
		if value == "" {
			continue
		}
		if err := cmd.Flags().Set(field.FlagName, value); err != nil {
			return err
		}
	}
	return nil
}

func addProviderAuthFlags(cmd *cobra.Command) {
	cmd.Flags().String("api-key", "", "API key for the provider")
	cmd.Flags().String("token", "", "Provider token (for providers that support token auth)")
	cmd.Flags().String("method", "", "Authentication method (for providers that support multiple methods)")
	cmd.Flags().String("endpoint", "", "Base URL endpoint (openai-compatible)")
	cmd.Flags().String("region", "", "Region (alibaba, azure-openai)")
	cmd.Flags().String("plan", "", "Plan (alibaba: standard|coding-plan)")
	cmd.Flags().BoolP("yes", "y", false, "Skip confirmations")
	cmd.Flags().Bool("device", false, "Use device code flow without opening a browser automatically")
}

func authAndCreateProvider(cmd *cobra.Command, providerType string) error {
	c := NewClient(cmd)

	apiKey, _ := cmd.Flags().GetString("api-key")
	token, _ := cmd.Flags().GetString("token")
	method, _ := cmd.Flags().GetString("method")
	endpoint, _ := cmd.Flags().GetString("endpoint")
	region, _ := cmd.Flags().GetString("region")
	plan, _ := cmd.Flags().GetString("plan")

	body := map[string]interface{}{
		"api_key":  apiKey,
		"apiKey":   apiKey,
		"token":    token,
		"method":   method,
		"endpoint": endpoint,
		"region":   region,
		"plan":     plan,
	}

	data, err := c.Post("/api/admin/providers/auth-and-create/"+providerType, body)
	if err != nil {
		return err
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(data, &resp); err != nil {
		return fmt.Errorf("parse response: %w", err)
	}

	if requiresAuth, ok := resp["requiresAuth"].(bool); ok && requiresAuth {
		verifyURI, _ := resp["verification_uri"].(string)
		userCode, _ := resp["user_code"].(string)

		deviceOnly := getBoolFlag(cmd, "device")

		if !deviceOnly {
			opened := tryOpenBrowser(verifyURI)
			if opened {
				fmt.Fprintf(cmd.OutOrStdout(), "\n  Browser opened for authorization.\n  Code:  %s\n\nWaiting for authorization", userCode)
			} else {
				fmt.Fprintf(cmd.OutOrStdout(), "\n  Could not open browser automatically.\n  Visit: %s\n  Code:  %s\n\nWaiting for authorization", verifyURI, userCode)
			}
		} else {
			fmt.Fprintf(cmd.OutOrStdout(), "\n  Visit: %s\n  Code:  %s\n\nWaiting for authorization", verifyURI, userCode)
		}

		for {
			time.Sleep(3 * time.Second)
			fmt.Fprint(cmd.OutOrStdout(), ".")

			statusData, err := c.Get("/api/admin/auth-status")
			if err != nil {
				continue
			}
			var statusResp map[string]interface{}
			if err := json.Unmarshal(statusData, &statusResp); err != nil {
				continue
			}
			status, _ := statusResp["status"].(string)
			switch status {
			case "complete":
				fmt.Fprintln(cmd.OutOrStdout())
				providerID, _ := statusResp["providerId"].(string)
				SuccessMsg(cmd, "Provider '%s' authenticated successfully.", providerID)
				return nil
			case "error":
				fmt.Fprintln(cmd.OutOrStdout())
				errMsg, _ := statusResp["error"].(string)
				return fmt.Errorf("authentication failed: %s", errMsg)
			}
		}
	}

	if c.IsJSON() {
		c.PrintJSON(data)
		return nil
	}

	success, _ := resp["success"].(bool)
	if !success {
		msg, _ := resp["message"].(string)
		return fmt.Errorf("failed: %s", msg)
	}
	if prov, ok := resp["provider"].(map[string]interface{}); ok {
		id, _ := prov["id"].(string)
		name, _ := prov["name"].(string)
		SuccessMsg(cmd, "Provider '%s' (%s) added successfully.", id, name)
	}
	return nil
}

// ─── delete ───────────────────────────────────────────────────────────────────

var providerDeleteCmd = &cobra.Command{
	Use:   "delete <id>",
	Short: "Delete a provider instance",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		c := NewClient(cmd)
		id, err := resolveProviderID(cmd, c, args)
		if err != nil {
			return err
		}
		if !getBoolFlag(cmd, "yes") && !Confirm(cmd, fmt.Sprintf("Delete provider '%s'?", id)) {
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Cancelled.")
			return nil
		}
		data, err := c.Delete("/api/admin/providers/" + id)
		if err != nil {
			return err
		}
		if c.IsJSON() {
			c.PrintJSON(data)
			return nil
		}
		SuccessMsg(cmd, "Provider '%s' deleted.", id)
		return nil
	},
}

// ─── activate ─────────────────────────────────────────────────────────────────

var providerActivateCmd = &cobra.Command{
	Use:   "activate <id>",
	Short: "Activate a provider (add to active set)",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		c := NewClient(cmd)
		id, err := resolveProviderID(cmd, c, args)
		if err != nil {
			return err
		}
		data, err := c.Post("/api/admin/providers/"+id+"/activate", nil)
		if err != nil {
			return err
		}
		if c.IsJSON() {
			c.PrintJSON(data)
			return nil
		}
		SuccessMsg(cmd, "Provider '%s' activated.", id)
		return nil
	},
}

// ─── deactivate ───────────────────────────────────────────────────────────────

var providerDeactivateCmd = &cobra.Command{
	Use:   "deactivate <id>",
	Short: "Deactivate a provider",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		c := NewClient(cmd)
		id, err := resolveProviderID(cmd, c, args)
		if err != nil {
			return err
		}
		data, err := c.Post("/api/admin/providers/"+id+"/deactivate", nil)
		if err != nil {
			return err
		}
		if c.IsJSON() {
			c.PrintJSON(data)
			return nil
		}
		SuccessMsg(cmd, "Provider '%s' deactivated.", id)
		return nil
	},
}

// ─── switch ───────────────────────────────────────────────────────────────────

var providerSwitchCmd = &cobra.Command{
	Use:   "switch <id>",
	Short: "Switch the primary active provider",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		c := NewClient(cmd)
		id, err := resolveProviderID(cmd, c, args)
		if err != nil {
			return err
		}
		body := map[string]string{"providerId": id}
		data, err := c.Post("/api/admin/providers/switch", body)
		if err != nil {
			return err
		}
		if c.IsJSON() {
			c.PrintJSON(data)
			return nil
		}
		SuccessMsg(cmd, "Switched active provider to '%s'.", id)
		return nil
	},
}

// ─── rename ───────────────────────────────────────────────────────────────────

var providerRenameCmd = &cobra.Command{
	Use:   "rename <id>",
	Short: "Rename a provider or update its subtitle",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		c := NewClient(cmd)
		id, err := resolveProviderID(cmd, c, args)
		if err != nil {
			return err
		}
		name := getStringFlag(cmd, "name")
		subtitle := getStringFlag(cmd, "subtitle")
		if name == "" && subtitle == "" {
			return fmt.Errorf("at least one of --name or --subtitle is required")
		}
		body := map[string]interface{}{}
		if name != "" {
			body["name"] = name
		}
		if subtitle != "" {
			body["subtitle"] = subtitle
		}
		data, err := c.Patch("/api/admin/providers/"+id+"/name", body)
		if err != nil {
			return err
		}
		if c.IsJSON() {
			c.PrintJSON(data)
			return nil
		}
		SuccessMsg(cmd, "Provider '%s' renamed.", id)
		return nil
	},
}

// ─── priorities ───────────────────────────────────────────────────────────────

var providerPrioritiesCmd = &cobra.Command{
	Use:   "priorities",
	Short: "Get or set provider priorities",
	RunE: func(cmd *cobra.Command, args []string) error {
		c := NewClient(cmd)
		sets := getStringSliceFlag(cmd, "set")

		if len(sets) == 0 {
			// GET
			data, err := c.Get("/api/admin/providers/priorities")
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
			priorities, _ := resp["priorities"].(map[string]interface{})
			table := NewTable("PROVIDER ID", "PRIORITY")
			for id, p := range priorities {
				table.AddRow(id, fmt.Sprintf("%.0f", p))
			}
			return table.Render(cmd.OutOrStdout())
		}

		// POST — parse "id:N" pairs
		priorities := map[string]int{}
		for _, s := range sets {
			parts := strings.SplitN(s, ":", 2)
			if len(parts) != 2 {
				return fmt.Errorf("invalid --set value %q: expected id:N", s)
			}
			var n int
			if _, err := fmt.Sscanf(parts[1], "%d", &n); err != nil {
				return fmt.Errorf("invalid priority %q: %w", parts[1], err)
			}
			priorities[parts[0]] = n
		}
		body := map[string]interface{}{"priorities": priorities}
		data, err := c.Post("/api/admin/providers/priorities", body)
		if err != nil {
			return err
		}
		if c.IsJSON() {
			c.PrintJSON(data)
			return nil
		}
		SuccessMsg(cmd, "Provider priorities updated.")
		return nil
	},
}

// ─── usage ────────────────────────────────────────────────────────────────────

var providerUsageCmd = &cobra.Command{
	Use:   "usage <id>",
	Short: "Show usage/quota for a provider",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		c := NewClient(cmd)
		id, err := resolveProviderID(cmd, c, args)
		if err != nil {
			return err
		}
		data, err := c.Get("/api/admin/providers/" + id + "/usage")
		if err != nil {
			return err
		}
		if c.IsJSON() {
			c.PrintJSON(data)
			return nil
		}
		var usage map[string]interface{}
		if err := json.Unmarshal(data, &usage); err != nil {
			fmt.Fprintln(cmd.OutOrStdout(), string(data))
			return nil
		}
		out := cmd.OutOrStdout()
		if err := PrintSection(out, "Usage for "+id); err != nil {
			return err
		}
		table := NewTable("METRIC", "VALUE")
		for k, v := range usage {
			table.AddRow(k, fmt.Sprintf("%v", v))
		}
		return table.Render(out)
	},
}
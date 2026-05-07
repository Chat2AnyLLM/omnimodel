package commands

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"
)

var ModelCmd = &cobra.Command{
	Use:   "model",
	Short: "Manage models for a provider",
}

func init() {
	ModelCmd.AddCommand(modelListCmd)
	ModelCmd.AddCommand(modelRefreshCmd)
	ModelCmd.AddCommand(modelMetadataCmd)
	modelMetadataCmd.Flags().Bool("refresh", false, "Refresh metadata from models.dev instead of using cache")

	modelToggleCmd.Flags().Bool("enable", false, "Enable the model")
	modelToggleCmd.Flags().Bool("disable", false, "Disable the model")
	ModelCmd.AddCommand(modelToggleCmd)

	modelVersionCmd := &cobra.Command{
		Use:   "version",
		Short: "Get or set a model version",
	}
	modelVersionCmd.AddCommand(modelVersionGetCmd)
	modelVersionCmd.AddCommand(modelVersionSetCmd)
	ModelCmd.AddCommand(modelVersionCmd)
}

func formatFloat(v *float64) string {
	if v == nil {
		return "-"
	}
	return fmt.Sprintf("%.4g", *v)
}

func formatInt(v *int) string {
	if v == nil {
		return "-"
	}
	return fmt.Sprintf("%d", *v)
}

type modelMetadataEntry struct {
	ID                        string   `json:"id"`
	Name                      string   `json:"name"`
	ProviderID                string   `json:"provider_id"`
	InputPriceUSDPer1MTokens  *float64 `json:"input_price_usd_per_1m_tokens"`
	OutputPriceUSDPer1MTokens *float64 `json:"output_price_usd_per_1m_tokens"`
	ContextLimitTokens        *int     `json:"context_limit_tokens"`
	InputLimitTokens          *int     `json:"input_limit_tokens"`
	OutputLimitTokens         *int     `json:"output_limit_tokens"`
}

type modelMetadataResponse struct {
	Data []modelMetadataEntry `json:"data"`
	// Count exists on server response but is not required for rendering.
	Count int `json:"count"`
}

var modelMetadataCmd = &cobra.Command{
	Use:   "metadata [filter]",
	Short: "Show normalized model pricing and token limits from backend metadata cache",
	Args:  cobra.MaximumNArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		c := NewClient(cmd)
		path := "/models/metadata"
		if refresh, _ := cmd.Flags().GetBool("refresh"); refresh {
			path += "?refresh=1"
		}

		data, err := c.Get(path)
		if err != nil {
			return err
		}

		if c.IsJSON() {
			c.PrintJSON(data)
			return nil
		}

		var resp modelMetadataResponse
		if err := json.Unmarshal(data, &resp); err != nil {
			return err
		}

		filter := ""
		if len(args) > 0 {
			filter = strings.ToLower(strings.TrimSpace(args[0]))
		}

		table := NewTable("MODEL", "PROVIDER", "INPUT $/1M", "OUTPUT $/1M", "CTX", "IN", "OUT")
		count := 0
		for _, item := range resp.Data {
			if filter != "" {
				candidate := strings.ToLower(item.ID + " " + item.Name + " " + item.ProviderID)
				if !strings.Contains(candidate, filter) {
					continue
				}
			}

			table.AddRow(
				item.ID,
				item.ProviderID,
				formatFloat(item.InputPriceUSDPer1MTokens),
				formatFloat(item.OutputPriceUSDPer1MTokens),
				formatInt(item.ContextLimitTokens),
				formatInt(item.InputLimitTokens),
				formatInt(item.OutputLimitTokens),
			)
			count++
		}

		if count == 0 {
			return PrintEmpty(cmd.OutOrStdout(), "model metadata")
		}

		if err := table.Render(cmd.OutOrStdout()); err != nil {
			return err
		}
		_, err = fmt.Fprintf(cmd.OutOrStdout(), "\n%d model metadata row(s)\n", count)
		return err
	},
}

// ─── list ─────────────────────────────────────────────────────────────────────

var modelListCmd = &cobra.Command{
	Use:   "list <provider-id>",
	Short: "List models for a provider",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		c := NewClient(cmd)
		data, err := c.Get("/api/admin/providers/" + args[0] + "/models")
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

		models, _ := resp["models"].([]interface{})
		if len(models) == 0 {
			return PrintEmpty(cmd.OutOrStdout(), "models found")
		}

		table := NewTable("MODEL ID", "NAME", "ENABLED")
		for _, m := range models {
			model, _ := m.(map[string]interface{})
			id, _ := model["id"].(string)
			name, _ := model["name"].(string)
			enabled := "no"
			if v, ok := model["enabled"].(bool); ok && v {
				enabled = "yes"
			}
			table.AddRow(id, name, enabled)
		}
		if err := table.Render(cmd.OutOrStdout()); err != nil {
			return err
		}
		total, _ := resp["total"].(float64)
		_, err = fmt.Fprintf(cmd.OutOrStdout(), "\n%d model(s)\n", int(total))
		return err
	},
}

// ─── refresh ──────────────────────────────────────────────────────────────────

var modelRefreshCmd = &cobra.Command{
	Use:   "refresh <provider-id>",
	Short: "Refresh the model list for a provider from the upstream API",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		c := NewClient(cmd)
		data, err := c.Post("/api/admin/providers/"+args[0]+"/models/refresh", nil)
		if err != nil {
			return err
		}
		if c.IsJSON() {
			c.PrintJSON(data)
			return nil
		}
		var resp map[string]interface{}
		_ = json.Unmarshal(data, &resp)
		total, _ := resp["total"].(float64)
		SuccessMsg(cmd, "Model list refreshed. %d model(s) available.", int(total))
		return nil
	},
}

// ─── toggle ───────────────────────────────────────────────────────────────────

var modelToggleCmd = &cobra.Command{
	Use:   "toggle <provider-id> <model-id>",
	Short: "Enable or disable a model",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		enable, _ := cmd.Flags().GetBool("enable")
		disable, _ := cmd.Flags().GetBool("disable")
		if !enable && !disable {
			return fmt.Errorf("specify --enable or --disable")
		}
		enabled := enable && !disable

		c := NewClient(cmd)
		body := map[string]interface{}{
			"modelId": args[1],
			"enabled": enabled,
		}
		data, err := c.Post("/api/admin/providers/"+args[0]+"/models/toggle", body)
		if err != nil {
			return err
		}
		if c.IsJSON() {
			c.PrintJSON(data)
			return nil
		}
		state := "enabled"
		if !enabled {
			state = "disabled"
		}
		SuccessMsg(cmd, "Model '%s' %s.", args[1], state)
		return nil
	},
}

// ─── version get ──────────────────────────────────────────────────────────────

var modelVersionGetCmd = &cobra.Command{
	Use:   "get <provider-id> <model-id>",
	Short: "Get the pinned version for a model",
	Args:  cobra.ExactArgs(2),
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
		version, _ := resp["version"].(string)
		if version == "" {
			fmt.Println("No version pinned (using provider default).")
		} else {
			fmt.Printf("Version: %s\n", version)
		}
		return nil
	},
}

// ─── version set ──────────────────────────────────────────────────────────────

var modelVersionSetCmd = &cobra.Command{
	Use:   "set <provider-id> <model-id> <version>",
	Short: "Pin a model to a specific version",
	Args:  cobra.ExactArgs(3),
	RunE: func(cmd *cobra.Command, args []string) error {
		c := NewClient(cmd)
		path := fmt.Sprintf("/api/admin/providers/%s/models/%s/version", args[0], args[1])
		body := map[string]string{"version": args[2]}
		data, err := c.Put(path, body)
		if err != nil {
			return err
		}
		if c.IsJSON() {
			c.PrintJSON(data)
			return nil
		}
		SuccessMsg(cmd, "Model '%s' pinned to version '%s'.", args[1], args[2])
		return nil
	},
}

package commands

import (
	"encoding/json"
	"fmt"
	"strconv"
	"strings"

	"github.com/spf13/cobra"
)

var VirtualModelCmd = &cobra.Command{
	Use:   "virtualmodel",
	Short: "Manage virtual models (model aliases with load-balancing)",
	Long: `Virtual models are stable model aliases that route requests to one or
more upstream provider/model pairs with configurable load-balancing strategies.`,
}

func init() {
	VirtualModelCmd.AddCommand(vmListCmd)
	VirtualModelCmd.AddCommand(vmGetCmd)

	vmCreateCmd.Flags().String("name", "", "Display name (required)")
	vmCreateCmd.Flags().String("description", "", "Optional description")
	vmCreateCmd.Flags().StringP("strategy", "s", "round-robin", "Load-balancing strategy: round-robin|random|priority|weighted")
	vmCreateCmd.Flags().String("api-shape", "openai", "API shape: openai|anthropic")
	vmCreateCmd.Flags().StringArrayP("upstream", "u", nil, "Upstream in format provider-id/model-id or provider-id/model-id:weight:priority (repeatable)")
	vmCreateCmd.Flags().Bool("disabled", false, "Create in disabled state")
	VirtualModelCmd.AddCommand(vmCreateCmd)

	vmUpdateCmd.Flags().String("name", "", "New display name")
	vmUpdateCmd.Flags().String("description", "", "New description")
	vmUpdateCmd.Flags().StringP("strategy", "s", "", "Load-balancing strategy")
	vmUpdateCmd.Flags().String("api-shape", "", "API shape")
	vmUpdateCmd.Flags().StringArrayP("upstream", "u", nil, "Upstream (repeatable, replaces all existing)")
	vmUpdateCmd.Flags().Bool("disabled", false, "Disable the virtual model")
	vmUpdateCmd.Flags().Bool("enabled", false, "Enable the virtual model")
	VirtualModelCmd.AddCommand(vmUpdateCmd)

	vmDeleteCmd.Flags().BoolP("yes", "y", false, "Skip confirmation")
	VirtualModelCmd.AddCommand(vmDeleteCmd)

	VirtualModelCmd.AddCommand(vmRenameCmd)
}

// ─── list ─────────────────────────────────────────────────────────────────────

var vmListCmd = &cobra.Command{
	Use:   "list",
	Short: "List all virtual models",
	RunE: func(cmd *cobra.Command, args []string) error {
		c := NewClient(cmd)
		data, err := c.Get("/api/admin/virtualmodels")
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
		items, _ := resp["data"].([]interface{})
		if len(items) == 0 {
			return PrintEmpty(cmd.OutOrStdout(), "virtual models configured")
		}

		table := NewTable("ID", "NAME", "STRATEGY", "API SHAPE", "UPSTREAMS", "ENABLED")
		for _, item := range items {
			vm, _ := item.(map[string]interface{})
			id, _ := vm["virtual_model_id"].(string)
			name, _ := vm["name"].(string)
			strategy, _ := vm["lb_strategy"].(string)
			apiShape, _ := vm["api_shape"].(string)
			upstreams, _ := vm["upstreams"].([]interface{})
			enabled := "no"
			if v, ok := vm["enabled"].(bool); ok && v {
				enabled = "yes"
			}
			table.AddRow(id, name, strategy, apiShape, fmt.Sprintf("%d", len(upstreams)), enabled)
		}
		return table.Render(cmd.OutOrStdout())
	},
}

// ─── get ──────────────────────────────────────────────────────────────────────

var vmGetCmd = &cobra.Command{
	Use:   "get <id>",
	Short: "Get details of a virtual model",
	Args:  cobra.ExactArgs(1),
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
}

// ─── create ───────────────────────────────────────────────────────────────────

var vmCreateCmd = &cobra.Command{
	Use:   "create <id>",
	Short: "Create a new virtual model",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		name, _ := cmd.Flags().GetString("name")
		if name == "" {
			return fmt.Errorf("--name is required")
		}
		desc, _ := cmd.Flags().GetString("description")
		strategy, _ := cmd.Flags().GetString("strategy")
		apiShape, _ := cmd.Flags().GetString("api-shape")
		upstreamArgs, _ := cmd.Flags().GetStringArray("upstream")
		disabled, _ := cmd.Flags().GetBool("disabled")

		upstreams, err := parseUpstreamArgs(upstreamArgs)
		if err != nil {
			return err
		}
		if len(upstreams) == 0 {
			return fmt.Errorf("at least one --upstream is required")
		}

		body := map[string]interface{}{
			"virtual_model_id": args[0],
			"name":             name,
			"description":      desc,
			"lb_strategy":      strategy,
			"api_shape":        apiShape,
			"enabled":          !disabled,
			"upstreams":        upstreams,
		}
		c := NewClient(cmd)
		data, err := c.Post("/api/admin/virtualmodels", body)
		if err != nil {
			return err
		}
		if c.IsJSON() {
			c.PrintJSON(data)
			return nil
		}
		SuccessMsg(cmd, "Virtual model '%s' created.", args[0])
		return nil
	},
}

// ─── update ───────────────────────────────────────────────────────────────────

var vmUpdateCmd = &cobra.Command{
	Use:   "update <id>",
	Short: "Update a virtual model",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		c := NewClient(cmd)

		// Fetch current state first
		existing, err := c.Get("/api/admin/virtualmodels/" + args[0])
		if err != nil {
			return err
		}
		var vm map[string]interface{}
		if err := json.Unmarshal(existing, &vm); err != nil {
			return err
		}

		if v, _ := cmd.Flags().GetString("name"); v != "" {
			vm["name"] = v
		}
		if v, _ := cmd.Flags().GetString("description"); v != "" {
			vm["description"] = v
		}
		if v, _ := cmd.Flags().GetString("strategy"); v != "" {
			vm["lb_strategy"] = v
		}
		if v, _ := cmd.Flags().GetString("api-shape"); v != "" {
			vm["api_shape"] = v
		}
		if disabled, _ := cmd.Flags().GetBool("disabled"); disabled {
			vm["enabled"] = false
		}
		if enabled, _ := cmd.Flags().GetBool("enabled"); enabled {
			vm["enabled"] = true
		}
		if upstreamArgs, _ := cmd.Flags().GetStringArray("upstream"); len(upstreamArgs) > 0 {
			upstreams, err := parseUpstreamArgs(upstreamArgs)
			if err != nil {
				return err
			}
			vm["upstreams"] = upstreams
		}
		// Ensure virtual_model_id is set (required by server)
		vm["virtual_model_id"] = args[0]

		data, err := c.Put("/api/admin/virtualmodels/"+args[0], vm)
		if err != nil {
			return err
		}
		if c.IsJSON() {
			c.PrintJSON(data)
			return nil
		}
		SuccessMsg(cmd, "Virtual model '%s' updated.", args[0])
		return nil
	},
}

// ─── delete ───────────────────────────────────────────────────────────────────

var vmDeleteCmd = &cobra.Command{
	Use:   "delete <id>",
	Short: "Delete a virtual model",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		yes, _ := cmd.Flags().GetBool("yes")
		if !yes && !Confirm(cmd, fmt.Sprintf("Delete virtual model '%s'?", args[0])) {
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Cancelled.")
			return nil
		}
		c := NewClient(cmd)
		data, err := c.Delete("/api/admin/virtualmodels/" + args[0])
		if err != nil {
			return err
		}
		if c.IsJSON() {
			c.PrintJSON(data)
			return nil
		}
		SuccessMsg(cmd, "Virtual model '%s' deleted.", args[0])
		return nil
	},
}

// ─── rename ───────────────────────────────────────────────────────────────────

var vmRenameCmd = &cobra.Command{
	Use:   "rename <old-id> <new-id>",
	Short: "Rename a virtual model",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		oldID := args[0]
		newID := args[1]

		c := NewClient(cmd)
		body := map[string]string{"new_id": newID}
		data, err := c.Post("/api/admin/virtualmodels/"+oldID+"/rename", body)
		if err != nil {
			return err
		}
		if c.IsJSON() {
			c.PrintJSON(data)
			return nil
		}
		SuccessMsg(cmd, "Virtual model '%s' renamed to '%s'.", oldID, newID)
		return nil
	},
}

// ─── helper ───────────────────────────────────────────────────────────────────

// parseUpstreamArgs parses strings of the form:
//
//	"provider-id/model-id"
//	"provider-id/model-id:weight"
//	"provider-id/model-id:weight:priority"
func parseUpstreamArgs(args []string) ([]map[string]interface{}, error) {
	var result []map[string]interface{}
	for _, arg := range args {
		// Split provider/model from optional :weight:priority
		colonParts := strings.SplitN(arg, ":", 3)
		providerModel := colonParts[0]

		slashIdx := strings.Index(providerModel, "/")
		if slashIdx < 0 {
			return nil, fmt.Errorf("invalid upstream %q: expected provider-id/model-id", arg)
		}
		providerID := providerModel[:slashIdx]
		modelID := providerModel[slashIdx+1:]

		weight := 1
		priority := 0
		var err error
		if len(colonParts) >= 2 && colonParts[1] != "" {
			if weight, err = strconv.Atoi(colonParts[1]); err != nil {
				return nil, fmt.Errorf("invalid weight in %q: %w", arg, err)
			}
		}
		if len(colonParts) >= 3 && colonParts[2] != "" {
			if priority, err = strconv.Atoi(colonParts[2]); err != nil {
				return nil, fmt.Errorf("invalid priority in %q: %w", arg, err)
			}
		}

		result = append(result, map[string]interface{}{
			"provider_id": providerID,
			"model_id":    modelID,
			"weight":      weight,
			"priority":    priority,
		})
	}
	return result, nil
}
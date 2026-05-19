package commands

import (
	"encoding/json"
	"fmt"
	"net/url"
	"strings"

	"github.com/spf13/cobra"
)

var UsageCmd = &cobra.Command{
	Use:   "usage",
	Short: "Show usage metrics for all providers",
	Long:  "Show backend metering usage metrics across providers, models, clients, and API shapes.",
	RunE:  runUsage,
}

var CheckUsageCmd = &cobra.Command{
	Use:        "check-usage",
	Short:      "Show usage metrics for all providers",
	Deprecated: "use 'usage' instead",
	RunE:       runUsage,
}

func init() {
	addUsageFlags(UsageCmd)
	addUsageFlags(CheckUsageCmd)
}

func addUsageFlags(cmd *cobra.Command) {
	cmd.Flags().String("provider-id", "", "Filter by provider instance ID")
	cmd.Flags().String("model-id", "", "Filter by model ID")
	cmd.Flags().String("client", "", "Filter by client")
	cmd.Flags().String("api-shape", "", "Filter by API shape (openai or anthropic)")
	cmd.Flags().String("since", "", "Filter from RFC3339 timestamp")
	cmd.Flags().String("until", "", "Filter until RFC3339 timestamp")
	cmd.Flags().String("breakdown", "provider", "Breakdown to show when not using JSON: provider/providers, model/models, client/clients, or none")
}

func runUsage(cmd *cobra.Command, args []string) error {
	c := NewClient(cmd)
	query := usageQuery(cmd)
	out := cmd.OutOrStdout()

	statsData, err := c.Get("/api/admin/metering/stats" + query)
	if err != nil {
		return err
	}
	if c.IsJSON() {
		c.PrintJSON(statsData)
		return nil
	}

	var stats map[string]any
	if err := json.Unmarshal(statsData, &stats); err != nil {
		return fmt.Errorf("parse stats response: %w", err)
	}

	if err := PrintSection(out, "Usage summary"); err != nil {
		return err
	}
	if filters := activeUsageFilters(cmd); len(filters) > 0 {
		filterTable := NewTable("FILTER", "VALUE")
		for _, filter := range filters {
			filterTable.AddRow(filter[0], filter[1])
		}
		if err := filterTable.Render(out); err != nil {
			return err
		}
		if _, err := fmt.Fprintln(out); err != nil {
			return err
		}
	}
	summary := NewTable("METRIC", "VALUE")
	summary.AddRow("Requests", fmt.Sprint(stats["total_requests"]))
	summary.AddRow("Input tokens", fmt.Sprint(stats["total_input_tokens"]))
	summary.AddRow("Output tokens", fmt.Sprint(stats["total_output_tokens"]))
	summary.AddRow("Total tokens", fmt.Sprint(stats["total_tokens"]))
	summary.AddRow("Average latency ms", fmt.Sprint(stats["avg_latency_ms"]))
	summary.AddRow("Errors", fmt.Sprint(stats["error_count"]))
	if err := summary.Render(out); err != nil {
		return err
	}

	breakdown, _ := cmd.Flags().GetString("breakdown")
	path, err := usageBreakdownPath(breakdown)
	if err != nil {
		return err
	}
	if path == "" {
		return nil
	}

	breakdownData, err := c.Get(path + query)
	if err != nil {
		return err
	}
	var resp map[string]any
	if err := json.Unmarshal(breakdownData, &resp); err != nil {
		return fmt.Errorf("parse breakdown response: %w", err)
	}
	items, _ := resp["items"].([]any)
	if len(items) == 0 {
		return nil
	}

	if _, err := fmt.Fprintln(out); err != nil {
		return err
	}
	if err := PrintSection(out, fmt.Sprintf("Usage by %s", usageBreakdownTitle(breakdown))); err != nil {
		return err
	}
	table := usageBreakdownTable(breakdown)
	for _, item := range items {
		row, ok := item.(map[string]any)
		if !ok {
			continue
		}
		addUsageBreakdownRow(table, breakdown, row)
	}
	return table.Render(out)
}

func usageQuery(cmd *cobra.Command) string {
	params := url.Values{}
	for _, name := range []string{"provider-id", "model-id", "client", "api-shape", "since", "until"} {
		value, _ := cmd.Flags().GetString(name)
		if value != "" {
			params.Set(strings.ReplaceAll(name, "-", "_"), value)
		}
	}
	if len(params) == 0 {
		return ""
	}
	return "?" + params.Encode()
}

func activeUsageFilters(cmd *cobra.Command) [][2]string {
	filters := make([][2]string, 0, 6)
	for _, entry := range []struct {
		flag  string
		label string
	}{
		{flag: "provider-id", label: "Provider"},
		{flag: "model-id", label: "Model"},
		{flag: "client", label: "Client"},
		{flag: "api-shape", label: "API shape"},
		{flag: "since", label: "Since"},
		{flag: "until", label: "Until"},
	} {
		value, _ := cmd.Flags().GetString(entry.flag)
		if value != "" {
			filters = append(filters, [2]string{entry.label, value})
		}
	}
	return filters
}

func usageBreakdownTitle(breakdown string) string {
	switch breakdown {
	case "providers":
		return "provider"
	case "models":
		return "model"
	case "clients":
		return "client"
	default:
		return breakdown
	}
}

func usageBreakdownPath(breakdown string) (string, error) {
	switch breakdown {
	case "provider", "providers":
		return "/api/admin/metering/by-provider", nil
	case "model", "models":
		return "/api/admin/metering/by-model", nil
	case "client", "clients":
		return "/api/admin/metering/by-client", nil
	case "", "none":
		return "", nil
	default:
		return "", fmt.Errorf("invalid breakdown %q: expected provider, model, client, or none", breakdown)
	}
}

func usageBreakdownTable(breakdown string) *Table {
	labelHeader := map[string]string{
		"provider":  "PROVIDER",
		"providers": "PROVIDER",
		"model":     "MODEL",
		"models":    "MODEL",
		"client":    "CLIENT",
		"clients":   "CLIENT",
	}[breakdown]
	t := NewTable(labelHeader, "REQUESTS", "TOKENS", "AVG LATENCY MS")
	t.SetMaxWidth(0, 40) // label column
	return t
}

func addUsageBreakdownRow(table *Table, breakdown string, row map[string]any) {
	labelKey := map[string]string{
		"provider":  "provider_id",
		"providers": "provider_id",
		"model":     "model_id",
		"models":    "model_id",
		"client":    "client",
		"clients":   "client",
	}[breakdown]
	label, _ := row[labelKey].(string)
	if label == "" {
		label = "(unknown)"
	}
	table.AddRow(
		label,
		fmt.Sprint(row["requests"]),
		fmt.Sprint(row["total_tokens"]),
		fmt.Sprint(row["avg_latency_ms"]),
	)
}

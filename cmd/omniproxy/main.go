package main

import (
	"fmt"
	"omnillm/internal/commands"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:           "omniproxy",
	Short:         "Universal LLM proxy with OpenAI-compatible endpoints",
	Long:          "OmniProxy is the OmniLLM proxy binary that provides OpenAI-compatible endpoints for multiple upstream providers.",
	SilenceUsage:  true,
	SilenceErrors: true,
}

func main() {
	rootCmd.PersistentFlags().String("server", "http://127.0.0.1:5000",
		"OmniLLM server address (or set OMNILLM_SERVER)")
	rootCmd.PersistentFlags().String("api-key", "",
		"Admin API key for the server (or set OMNILLM_API_KEY)")
	rootCmd.PersistentFlags().StringP("output", "o", "table",
		"Output format: table or json")

	rootCmd.AddCommand(commands.StartCmd)
	rootCmd.AddCommand(commands.AuthCmd)
	rootCmd.AddCommand(commands.UsageCmd)
	rootCmd.AddCommand(commands.CheckUsageCmd)
	rootCmd.AddCommand(commands.SyncNamesCmd)
	rootCmd.AddCommand(commands.DebugCmd)
	rootCmd.AddCommand(commands.ProviderCmd)
	rootCmd.AddCommand(commands.ModelCmd)
	rootCmd.AddCommand(commands.VirtualModelCmd)
	rootCmd.AddCommand(commands.ConfigCmd)
	rootCmd.AddCommand(commands.SettingsCmd)
	rootCmd.AddCommand(commands.StatusCmd)
	rootCmd.AddCommand(commands.ChatCmd)
	rootCmd.AddCommand(commands.LogsCmd)

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

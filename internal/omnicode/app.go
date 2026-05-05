package omnicode

import (
	"fmt"
	"omnillm/internal/chat"
	"omnillm/internal/commands"
	"os"

	"github.com/spf13/cobra"
)

func NewRootCmd() *cobra.Command {
	var cfg *Config

	rootCmd := &cobra.Command{
		Use:           "omnicode",
		Short:         "Coding-focused chat and agent CLI",
		Long:          "OmniCode is a coding-focused interactive chat and agent CLI built on OmniLLM components.",
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			var err error
			cfg, err = LoadConfig()
			if err != nil {
				return fmt.Errorf("load config: %w", err)
			}

			if cfg.Model != "" {
				modelFlag := cmd.Flags().Lookup("model")
				if modelFlag != nil && !modelFlag.Changed {
					modelFlag.Value.Set(cfg.Model)
				}
			}

			chat.InitialConfig.Mode = cfg.Mode
			chat.InitialConfig.APIShape = cfg.APIShape
			chat.InitialConfig.Autopilot = cfg.Autopilot
			if cfg.MaxTurns > 0 {
				chat.InitialConfig.MaxTurns = cfg.MaxTurns
			}

			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			saveCb := func(model, mode, apiShape, agentBackend string, autopilot bool, maxTurns int) {
				if cfg == nil {
					return
				}
				cfg.Model = model
				cfg.Mode = mode
				cfg.APIShape = apiShape
				cfg.AgentBackend = agentBackend
				cfg.Autopilot = autopilot
				cfg.MaxTurns = maxTurns
				SaveConfig(cfg)
			}
			chat.SetConfigSaveCallback(saveCb)
			return commands.ChatCmd.RunE(cmd, args)
		},
	}

	rootCmd.PersistentFlags().String("server", "http://127.0.0.1:5000", "OmniLLM/OmniCode server address (or set OMNILLM_SERVER)")
	rootCmd.PersistentFlags().String("api-key", "", "Admin API key for the server (or set OMNILLM_API_KEY)")
	rootCmd.PersistentFlags().StringP("output", "o", "table", "Output format: table or json")
	rootCmd.Flags().String("model", "", "Model to use for the chat session")
	rootCmd.Flags().String("session", "", "Resume an existing session by ID")

	rootCmd.AddCommand(commands.ChatCmd)

	return rootCmd
}

func Run() {
	if err := NewRootCmd().Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

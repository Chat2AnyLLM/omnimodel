package omnicode

import (
	"fmt"
	"omnillm/internal/chat"
	"omnillm/internal/commands"
	"os"

	"github.com/spf13/cobra"
)

func runOneShot(cmd *cobra.Command, prompt string) error {
	c := commands.NewClient(cmd)
	requestedModel, _ := cmd.Flags().GetString("model")
	existingSession, _ := cmd.Flags().GetString("session")

	session, err := chat.EnsureSession(cmd, c, existingSession, requestedModel)
	if err != nil {
		return err
	}

	if session.Mode == "agent" {
		result, err := chat.RunAgentTurn(c, session.ID, session.Model, session.AgentBackend, session.APIShape, prompt, cmd)
		if err != nil {
			return err
		}
		fmt.Println(result)
		return nil
	}

	if err := chat.PostMessage(c, session.ID, "user", prompt); err != nil {
		return fmt.Errorf("store message: %w", err)
	}

	_, messages, err := chat.LoadSessionMessages(c, session.ID)
	if err != nil {
		return fmt.Errorf("load messages: %w", err)
	}

	assistantContent, err := chat.StreamCompletion(c, session.Model, messages, cmd.OutOrStdout(), false)
	if err != nil {
		return fmt.Errorf("completion: %w", err)
	}

	if assistantContent != "" {
		if err := chat.PostMessage(c, session.ID, "assistant", assistantContent); err != nil {
			return fmt.Errorf("store assistant message: %w", err)
		}
	}
	fmt.Println()
	return nil
}

func NewRootCmd() *cobra.Command {
	var cfg *Config

	rootCmd := &cobra.Command{
		Use:           "omnicode",
		Short:         "Coding-focused chat and agent CLI",
		Long:          "OmniCode is a coding-focused interactive chat and agent CLI built on OmniLLM components.",
		SilenceUsage:  true,
		SilenceErrors: true,
		PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
			verbose, _ := cmd.Flags().GetBool("verbose")
			logFile, _ := cmd.Flags().GetString("log-file")
			if err := setupLogging(verbose, logFile); err != nil {
				return fmt.Errorf("setup logging: %w", err)
			}

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
			chat.InitialConfig.SpecMode = cfg.SpecMode
			if cfg.MaxTurns > 0 {
				chat.InitialConfig.MaxTurns = cfg.MaxTurns
			}

			return nil
		},
		RunE: func(cmd *cobra.Command, args []string) error {
			prompt, _ := cmd.Flags().GetString("prompt")
			if prompt != "" {
				return runOneShot(cmd, prompt)
			}

			saveCb := func(model, mode, apiShape, agentBackend, specMode string, autopilot bool, maxTurns int) {
				if cfg == nil {
					return
				}
				cfg.Model = model
				cfg.Mode = mode
				cfg.APIShape = apiShape
				cfg.AgentBackend = agentBackend
				cfg.SpecMode = specMode
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
	rootCmd.PersistentFlags().BoolP("verbose", "v", false, "Enable debug-level logging")
	rootCmd.PersistentFlags().String("log-file", "", "Write logs to file (e.g. ~/.config/omnicode/omnicode.log)")
	rootCmd.Flags().String("model", "", "Model to use for the chat session")
	rootCmd.Flags().String("session", "", "Resume an existing session by ID")
	rootCmd.Flags().StringP("prompt", "p", "", "Send a single prompt and print the response (non-interactive)")

	rootCmd.AddCommand(commands.ChatCmd)

	return rootCmd
}

func Run() {
	if err := NewRootCmd().Execute(); err != nil {
		fmt.Fprintf(os.Stderr, "Error: %v\n", err)
		os.Exit(1)
	}
}

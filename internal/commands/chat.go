package commands

import (
	"encoding/json"
	"fmt"
	"strings"

	chatpkg "omnillm/internal/chat"

	"github.com/spf13/cobra"
)

var ChatCmd = &cobra.Command{
	Use:   "chat",
	Short: "Interactive chat or manage chat sessions",
	Long: `Start an interactive chat session or manage saved sessions.

Running 'omnillm chat' without subcommands launches an interactive REPL
that sends messages to the active provider via the proxy.`,
	RunE: runInteractiveChat,
}

var chatSessionsCmd = &cobra.Command{
	Use:   "sessions",
	Short: "Manage saved chat sessions",
}

func init() {
	chatSessionsCmd.AddCommand(chatSessionsListCmd)

	chatSessionsCreateCmd.Flags().String("title", "", "Session title")
	chatSessionsCreateCmd.Flags().String("model", "", "Model ID to use for the session")
	chatSessionsCreateCmd.Flags().String("api-shape", "openai", "API shape (openai|anthropic)")
	chatSessionsCmd.AddCommand(chatSessionsCreateCmd)

	chatSessionsCmd.AddCommand(chatSessionsGetCmd)

	chatSessionsRenameCmd.Flags().String("title", "", "New title (required)")
	chatSessionsCmd.AddCommand(chatSessionsRenameCmd)

	chatSessionsDeleteCmd.Flags().BoolP("all", "a", false, "Delete all sessions")
	chatSessionsDeleteCmd.Flags().BoolP("yes", "y", false, "Skip confirmation")
	chatSessionsCmd.AddCommand(chatSessionsDeleteCmd)

	ChatCmd.AddCommand(chatSessionsCmd)

	chatSendCmd.Flags().String("role", "user", "Message role: user|assistant|system")
	ChatCmd.AddCommand(chatSendCmd)

	ChatCmd.Flags().String("model", "", "Model to use for the chat session")
	ChatCmd.Flags().String("session", "", "Resume an existing session by ID")
}

func runInteractiveChat(cmd *cobra.Command, args []string) error {
	c := NewClient(cmd)
	requestedModel, _ := cmd.Flags().GetString("model")
	existingSession, _ := cmd.Flags().GetString("session")

	picker := func(prompt string, models []chatpkg.ModelInfo) (string, error) {
		return chatpkg.PromptModelPicker(prompt, models, SelectFromOptions)
	}

	if IsTerminalWriter(cmd.OutOrStdout()) {
		session, err := chatpkg.EnsureSession(cmd, c, existingSession, requestedModel)
		if err != nil {
			return err
		}
		_, history, err := chatpkg.LoadSessionMessages(c, session.ID)
		if err != nil {
			return err
		}
		return chatpkg.RunTUI(c, session.ID, session.Model, session.Mode, session.AgentBackend, history)
	}

	return chatpkg.RunREPL(cmd, c, requestedModel, existingSession, picker)
}

// ─── sessions list ────────────────────────────────────────────────────────────

var chatSessionsListCmd = &cobra.Command{
	Use:   "list",
	Short: "List chat sessions",
	RunE: func(cmd *cobra.Command, args []string) error {
		c := NewClient(cmd)
		data, err := c.Get("/api/admin/chat/sessions")
		if err != nil {
			return err
		}
		if c.IsJSON() {
			c.PrintJSON(data)
			return nil
		}

		var resp map[string]any
		if err := json.Unmarshal(data, &resp); err != nil {
			return err
		}
		sessions, _ := resp["sessions"].([]any)
		if len(sessions) == 0 {
			return PrintEmpty(cmd.OutOrStdout(), "chat sessions")
		}

		table := NewTable("SESSION ID", "TITLE", "MODEL", "MESSAGES")
		for _, s := range sessions {
			session, _ := s.(map[string]any)
			id, _ := session["id"].(string)
			title, _ := session["title"].(string)
			model, _ := session["model_id"].(string)
			msgs, _ := session["message_count"].(float64)
			table.AddRow(id, title, model, fmt.Sprintf("%.0f", msgs))
		}
		return table.Render(cmd.OutOrStdout())
	},
}

// ─── sessions create ──────────────────────────────────────────────────────────

var chatSessionsCreateCmd = &cobra.Command{
	Use:   "create",
	Short: "Create a new chat session",
	RunE: func(cmd *cobra.Command, args []string) error {
		title, _ := cmd.Flags().GetString("title")
		model, _ := cmd.Flags().GetString("model")
		apiShape, _ := cmd.Flags().GetString("api-shape")

		body := map[string]any{
			"title":     title,
			"model_id":  model,
			"api_shape": apiShape,
		}
		c := NewClient(cmd)
		data, err := c.Post("/api/admin/chat/sessions", body)
		if err != nil {
			return err
		}
		if c.IsJSON() {
			c.PrintJSON(data)
			return nil
		}
		var resp map[string]any
		if err := json.Unmarshal(data, &resp); err == nil {
			if sid, ok := resp["session_id"].(string); ok {
				SuccessMsg(cmd, "Session created: %s", sid)
				return nil
			}
		}
		SuccessMsg(cmd, "Session created.")
		return nil
	},
}

// ─── sessions get ─────────────────────────────────────────────────────────────

var chatSessionsGetCmd = &cobra.Command{
	Use:   "get <session-id>",
	Short: "Show a chat session and its messages",
	Args:  cobra.ExactArgs(1),
	RunE: func(cmd *cobra.Command, args []string) error {
		c := NewClient(cmd)
		data, err := c.Get("/api/admin/chat/sessions/" + args[0])
		if err != nil {
			return err
		}
		if c.IsJSON() {
			c.PrintJSON(data)
			return nil
		}

		var session map[string]any
		if err := json.Unmarshal(data, &session); err != nil {
			return err
		}
		title, _ := session["title"].(string)
		model, _ := session["model_id"].(string)
		fmt.Printf("Session: %s\n", args[0])
		fmt.Printf("  Title: %s\n", title)
		fmt.Printf("  Model: %s\n", model)

		if msgs, ok := session["messages"].([]any); ok && len(msgs) > 0 {
			fmt.Printf("\nMessages (%d):\n", len(msgs))
			fmt.Println(strings.Repeat("─", 60))
			for _, m := range msgs {
				msg, _ := m.(map[string]any)
				role, _ := msg["role"].(string)
				content, _ := msg["content"].(string)
				ts, _ := msg["created_at"].(string)
				if len(ts) > 10 {
					ts = ts[:10]
				}
				fmt.Printf("[%s] %s: %s\n", ts, strings.ToUpper(role), content)
			}
		} else {
			fmt.Println("\nNo messages.")
		}
		return nil
	},
}

// ─── sessions rename ──────────────────────────────────────────────────────────

var chatSessionsRenameCmd = &cobra.Command{
	Use:   "rename <session-id> <new-title>",
	Short: "Rename a chat session",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		c := NewClient(cmd)
		body := map[string]string{"title": args[1]}
		data, err := c.Put("/api/admin/chat/sessions/"+args[0], body)
		if err != nil {
			return err
		}
		if c.IsJSON() {
			c.PrintJSON(data)
			return nil
		}
		SuccessMsg(cmd, "Session '%s' renamed.", args[0])
		return nil
	},
}

// ─── sessions delete ──────────────────────────────────────────────────────────

var chatSessionsDeleteCmd = &cobra.Command{
	Use:   "delete [session-id]",
	Short: "Delete a chat session (or all with --all)",
	RunE: func(cmd *cobra.Command, args []string) error {
		all, _ := cmd.Flags().GetBool("all")
		yes, _ := cmd.Flags().GetBool("yes")
		c := NewClient(cmd)

		if all {
			if !yes && !Confirm(cmd, "Delete all chat sessions?") {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Cancelled.")
				return nil
			}
			data, err := c.Delete("/api/admin/chat/sessions")
			if err != nil {
				return err
			}
			if c.IsJSON() {
				c.PrintJSON(data)
				return nil
			}
			SuccessMsg(cmd, "All sessions deleted.")
			return nil
		}

		if len(args) == 0 {
			return fmt.Errorf("provide a session-id or use --all")
		}
		if !yes && !Confirm(cmd, fmt.Sprintf("Delete session '%s'?", args[0])) {
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Cancelled.")
			return nil
		}
		data, err := c.Delete("/api/admin/chat/sessions/" + args[0])
		if err != nil {
			return err
		}
		if c.IsJSON() {
			c.PrintJSON(data)
			return nil
		}
		SuccessMsg(cmd, "Session '%s' deleted.", args[0])
		return nil
	},
}

// ─── send ─────────────────────────────────────────────────────────────────────

var chatSendCmd = &cobra.Command{
	Use:   "send <session-id> <message>",
	Short: "Append a message to a chat session",
	Args:  cobra.ExactArgs(2),
	RunE: func(cmd *cobra.Command, args []string) error {
		role, _ := cmd.Flags().GetString("role")
		c := NewClient(cmd)
		body := map[string]string{
			"role":    role,
			"content": args[1],
		}
		data, err := c.Post("/api/admin/chat/sessions/"+args[0]+"/messages", body)
		if err != nil {
			return err
		}
		if c.IsJSON() {
			c.PrintJSON(data)
			return nil
		}
		var resp map[string]any
		if err := json.Unmarshal(data, &resp); err == nil {
			if msgID, ok := resp["message_id"].(string); ok {
				SuccessMsg(cmd, "Message added: %s", msgID)
				return nil
			}
		}
		SuccessMsg(cmd, "Message added.")
		return nil
	},
}

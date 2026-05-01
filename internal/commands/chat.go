package commands

import (
	"bufio"
	"encoding/json"
	"fmt"
	"sort"
	"strings"

	"github.com/spf13/cobra"
)

type replCommandResult struct {
	handled bool
	exit    bool
	model   string
}

type chatSessionState struct {
	ID     string
	Model  string
	IsTTY  bool
	Picker modelPickerFunc
}

type modelPickerFunc func(string, []chatModelInfo) (string, error)

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
	// sessions subcommands
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

	// send subcommand
	chatSendCmd.Flags().String("role", "user", "Message role: user|assistant|system")
	ChatCmd.AddCommand(chatSendCmd)

	// interactive flags
	ChatCmd.Flags().String("model", "", "Model to use for the chat session")
	ChatCmd.Flags().String("session", "", "Resume an existing session by ID")
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

		var resp map[string]interface{}
		if err := json.Unmarshal(data, &resp); err != nil {
			return err
		}
		sessions, _ := resp["sessions"].([]interface{})
		if len(sessions) == 0 {
			return PrintEmpty(cmd.OutOrStdout(), "chat sessions")
		}

		table := NewTable("SESSION ID", "TITLE", "MODEL", "MESSAGES")
		for _, s := range sessions {
			session, _ := s.(map[string]interface{})
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

		body := map[string]interface{}{
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
		var resp map[string]interface{}
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

		var session map[string]interface{}
		if err := json.Unmarshal(data, &session); err != nil {
			return err
		}
		title, _ := session["title"].(string)
		model, _ := session["model_id"].(string)
		fmt.Printf("Session: %s\n", args[0])
		fmt.Printf("  Title: %s\n", title)
		fmt.Printf("  Model: %s\n", model)

		if msgs, ok := session["messages"].([]interface{}); ok && len(msgs) > 0 {
			fmt.Printf("\nMessages (%d):\n", len(msgs))
			fmt.Println(strings.Repeat("─", 60))
			for _, m := range msgs {
				msg, _ := m.(map[string]interface{})
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
		var resp map[string]interface{}
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

// ─── interactive REPL ────────────────────────────────────────────────────────

func runInteractiveChat(cmd *cobra.Command, args []string) error {
	c := NewClient(cmd)
	requestedModel, _ := cmd.Flags().GetString("model")
	existingSession, _ := cmd.Flags().GetString("session")

	session, err := ensureChatSession(cmd, c, existingSession, requestedModel)
	if err != nil {
		return err
	}
	session.IsTTY = IsTerminalWriter(cmd.OutOrStdout())
	session.Picker = promptModelPicker

	_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Type your message and press Enter.")
	_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Use /help to see interactive commands. Use /models to browse and switch models.")
	_, _ = fmt.Fprintln(cmd.OutOrStdout())

	scanner := bufio.NewScanner(cmd.InOrStdin())
	for {
		_, _ = fmt.Fprint(cmd.OutOrStdout(), FormatChatPrompt("You", session.IsTTY))
		if !scanner.Scan() {
			break
		}
		line := strings.TrimSpace(scanner.Text())
		if line == "" {
			continue
		}

		if strings.HasPrefix(line, "/") {
			result, err := handleChatSlashCommand(cmd, c, &session, line)
			if err != nil {
				ErrorMsg(cmd, "%v", err)
				continue
			}
			if result.model != "" {
				session.Model = result.model
			}
			if result.exit {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Goodbye.")
				break
			}
			if result.handled {
				continue
			}
		}

		if err := postChatSessionMessage(c, session.ID, "user", line); err != nil {
			ErrorMsg(cmd, "store message: %v", err)
			continue
		}

		loadedSession, messages, err := loadChatSessionMessages(c, session.ID)
		if err != nil {
			ErrorMsg(cmd, "%v", err)
			continue
		}
		if loadedSession.Model != "" {
			session.Model = loadedSession.Model
		}

		assistantContent, err := runChatCompletion(c, session.Model, messages)
		if err != nil {
			ErrorMsg(cmd, "completion: %v", err)
			continue
		}
		if assistantContent == "" {
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "(no response)")
			continue
		}

		_, _ = fmt.Fprintln(cmd.OutOrStdout(), FormatChatHeader("Assistant", session.IsTTY))
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "%s\n\n", assistantContent)
		if err := postChatSessionMessage(c, session.ID, "assistant", assistantContent); err != nil {
			ErrorMsg(cmd, "store assistant message: %v", err)
		}
	}

	return scanner.Err()
}

func ensureChatSession(cmd *cobra.Command, c *Client, existingSession string, requestedModel string) (chatSessionState, error) {
	if existingSession != "" {
		session, _, err := loadChatSessionMessages(c, existingSession)
		if err != nil {
			return chatSessionState{}, err
		}
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Resuming session: %s\n", existingSession)
		if requestedModel != "" && requestedModel != session.Model {
			if err := updateChatSessionModel(c, existingSession, requestedModel); err != nil {
				return chatSessionState{}, err
			}
			session.Model = requestedModel
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Using model: %s\n", requestedModel)
		}
		return session, nil
	}

	body := map[string]interface{}{
		"title":     "CLI session",
		"model_id":  requestedModel,
		"api_shape": "openai",
	}
	data, err := c.Post("/api/admin/chat/sessions", body)
	if err != nil {
		return chatSessionState{}, fmt.Errorf("create session: %w", err)
	}
	var resp map[string]interface{}
	if err := json.Unmarshal(data, &resp); err != nil {
		return chatSessionState{}, err
	}
	sid, ok := resp["session_id"].(string)
	if !ok {
		return chatSessionState{}, fmt.Errorf("server did not return a session_id")
	}
	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Started session: %s\n", sid)
	if requestedModel != "" {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Using model: %s\n", requestedModel)
	}
	return chatSessionState{ID: sid, Model: requestedModel}, nil
}

func handleChatSlashCommand(cmd *cobra.Command, c *Client, session *chatSessionState, line string) (replCommandResult, error) {
	fields := strings.Fields(line)
	if len(fields) == 0 {
		return replCommandResult{handled: true}, nil
	}

	switch fields[0] {
	case "/quit", "/exit":
		return replCommandResult{handled: true, exit: true}, nil
	case "/help":
		printChatHelp(cmd)
		return replCommandResult{handled: true}, nil
	case "/model":
		if len(fields) == 1 {
			currentModel, err := currentChatModel(c, session.ID, session.Model)
			if err != nil {
				return replCommandResult{}, err
			}
			if currentModel == "" {
				_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Current model: (server default)")
			} else {
				_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Current model: %s\n", currentModel)
			}
			return replCommandResult{handled: true, model: currentModel}, nil
		}
		newModel := fields[1]
		if err := updateChatSessionModel(c, session.ID, newModel); err != nil {
			return replCommandResult{}, err
		}
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Switched model to %s\n", newModel)
		return replCommandResult{handled: true, model: newModel}, nil
	case "/models":
		models, err := listChatModels(c)
		if err != nil {
			return replCommandResult{}, err
		}
		if len(models) == 0 {
			return replCommandResult{handled: true}, PrintEmpty(cmd.OutOrStdout(), "models")
		}
		filter := ""
		if len(fields) > 1 {
			filter = strings.Join(fields[1:], " ")
		}
		filtered := filterChatModels(models, filter)
		if session.IsTTY && filter == "" && session.Picker != nil {
			selected, err := session.Picker("Select a model", models)
			if err != nil {
				return replCommandResult{handled: true}, nil
			}
			if selected == "" {
				return replCommandResult{handled: true}, nil
			}
			if err := updateChatSessionModel(c, session.ID, selected); err != nil {
				return replCommandResult{}, err
			}
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Switched model to %s\n", selected)
			return replCommandResult{handled: true, model: selected}, nil
		}
		if len(filtered) == 0 {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "No models match %q\n", filter)
			return replCommandResult{handled: true}, nil
		}
		table := NewTable("SELECTOR", "NAME")
		for _, model := range filtered {
			table.AddRow(model.Selector, model.Name)
		}
		if err := table.Render(cmd.OutOrStdout()); err != nil {
			return replCommandResult{}, err
		}
		return replCommandResult{handled: true}, nil
	case "/session":
		currentModel, err := currentChatModel(c, session.ID, session.Model)
		if err != nil {
			return replCommandResult{}, err
		}
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Session: %s\n", session.ID)
		if currentModel == "" {
			_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Model:   (server default)")
		} else {
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Model:   %s\n", currentModel)
		}
		return replCommandResult{handled: true, model: currentModel}, nil
	default:
		return replCommandResult{}, fmt.Errorf("unknown command %q; use /help", fields[0])
	}
}

func printChatHelp(cmd *cobra.Command) {
	_, _ = fmt.Fprintln(cmd.OutOrStdout(), "Interactive commands:")
	_, _ = fmt.Fprintln(cmd.OutOrStdout(), "  /help              Show this help")
	_, _ = fmt.Fprintln(cmd.OutOrStdout(), "  /session           Show current session details")
	_, _ = fmt.Fprintln(cmd.OutOrStdout(), "  /model             Show the current model")
	_, _ = fmt.Fprintln(cmd.OutOrStdout(), "  /model <id>        Switch to a different model")
	_, _ = fmt.Fprintln(cmd.OutOrStdout(), "  /models            Open the model selector in a terminal")
	_, _ = fmt.Fprintln(cmd.OutOrStdout(), "  /models <filter>   List model selectors matching a filter")
	_, _ = fmt.Fprintln(cmd.OutOrStdout(), "  /quit, /exit       Leave the chat shell")
}


type chatMessage struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type chatModelInfo struct {
	ID       string
	Owner    string
	Name     string
	Selector string
}

func chatModelSelector(model chatModelInfo) string {
	if model.Owner == "" || model.Owner == "virtual" || strings.HasPrefix(model.ID, model.Owner+"/") {
		return model.ID
	}
	return model.Owner + "/" + model.ID
}

func promptModelPicker(prompt string, models []chatModelInfo) (string, error) {
	items := make([]string, 0, len(models))
	for _, model := range models {
		label := model.Selector
		if model.Name != "" && model.Name != model.ID {
			label = fmt.Sprintf("%s — %s", model.Selector, model.Name)
		}
		items = append(items, label)
	}

	selected, err := SelectFromOptions(prompt, items)
	if err != nil {
		return "", err
	}
	for i, item := range items {
		if item == selected {
			return models[i].Selector, nil
		}
	}
	return "", nil
}

func filterChatModels(models []chatModelInfo, filter string) []chatModelInfo {
	filter = strings.TrimSpace(strings.ToLower(filter))
	if filter == "" {
		return models
	}

	filtered := make([]chatModelInfo, 0, len(models))
	for _, model := range models {
		if strings.Contains(strings.ToLower(model.ID), filter) || strings.Contains(strings.ToLower(model.Name), filter) || strings.Contains(strings.ToLower(model.Owner), filter) || strings.Contains(strings.ToLower(model.Selector), filter) {
			filtered = append(filtered, model)
		}
	}
	return filtered
}

func currentChatModel(c *Client, sessionID string, fallback string) (string, error) {
	session, _, err := loadChatSessionMessages(c, sessionID)
	if err != nil {
		return "", err
	}
	if session.Model != "" {
		return session.Model, nil
	}
	return fallback, nil
}

func loadChatSessionMessages(c *Client, sessionID string) (chatSessionState, []chatMessage, error) {
	sessionData, err := c.Get("/api/admin/chat/sessions/" + sessionID)
	if err != nil {
		return chatSessionState{}, nil, fmt.Errorf("load session: %w", err)
	}
	var session map[string]interface{}
	if err := json.Unmarshal(sessionData, &session); err != nil {
		return chatSessionState{}, nil, fmt.Errorf("parse session: %w", err)
	}

	state := chatSessionState{ID: sessionID}
	if mid, ok := session["model_id"].(string); ok {
		state.Model = mid
	}

	var messages []chatMessage
	if msgs, ok := session["messages"].([]interface{}); ok {
		for _, m := range msgs {
			msg, _ := m.(map[string]interface{})
			role, _ := msg["role"].(string)
			content, _ := msg["content"].(string)
			messages = append(messages, chatMessage{Role: role, Content: content})
		}
	}

	return state, messages, nil
}

func postChatSessionMessage(c *Client, sessionID string, role string, content string) error {
	_, err := c.Post("/api/admin/chat/sessions/"+sessionID+"/messages", map[string]string{
		"role":    role,
		"content": content,
	})
	return err
}

func runChatCompletion(c *Client, model string, messages []chatMessage) (string, error) {
	reqModel := model
	if reqModel == "" {
		reqModel = "gpt-4"
	}

	completionBody := map[string]interface{}{
		"model":    reqModel,
		"messages": messages,
		"stream":   false,
	}

	respData, err := c.Post("/v1/chat/completions", completionBody)
	if err != nil {
		return "", err
	}

	var completion map[string]interface{}
	if err := json.Unmarshal(respData, &completion); err != nil {
		return string(respData), nil
	}

	if choices, ok := completion["choices"].([]interface{}); ok && len(choices) > 0 {
		choice, _ := choices[0].(map[string]interface{})
		if message, ok := choice["message"].(map[string]interface{}); ok {
			assistantContent, _ := message["content"].(string)
			return assistantContent, nil
		}
	}

	return "", nil
}

func updateChatSessionModel(c *Client, sessionID string, modelID string) error {
	_, err := c.Put("/api/admin/chat/sessions/"+sessionID, map[string]string{
		"model_id": modelID,
	})
	return err
}

func listChatModels(c *Client) ([]chatModelInfo, error) {
	data, err := c.Get("/models")
	if err != nil {
		return nil, err
	}

	var resp map[string]interface{}
	if err := json.Unmarshal(data, &resp); err != nil {
		return nil, err
	}

	items, _ := resp["data"].([]interface{})
	models := make([]chatModelInfo, 0, len(items))
	for _, item := range items {
		entry, _ := item.(map[string]interface{})
		id, _ := entry["id"].(string)
		owner, _ := entry["owned_by"].(string)
		name, _ := entry["display_name"].(string)
		selector := chatModelSelector(chatModelInfo{ID: id, Owner: owner, Name: name})
		models = append(models, chatModelInfo{ID: id, Owner: owner, Name: name, Selector: selector})
	}

	sort.Slice(models, func(i, j int) bool {
		return models[i].ID < models[j].ID
	})
	return models, nil
}

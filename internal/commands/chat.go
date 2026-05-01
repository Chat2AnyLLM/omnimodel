package commands

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sort"
	"strings"
	"time"

	"github.com/charmbracelet/glamour"
	"github.com/chzyer/readline"
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

	// Use full-screen TUI when stdout is a terminal; fall back to readline REPL.
	if session.IsTTY {
		_, history, err := loadChatSessionMessages(c, session.ID)
		if err != nil {
			return err
		}
		return runChatTUI(c, session.ID, session.Model, history)
	}

	out := cmd.OutOrStdout()

	_, _ = fmt.Fprintln(out, "Type your message and press Enter. Use ↑↓ for history, Ctrl+R to search.")
	_, _ = fmt.Fprintln(out, "Use /help for commands, /models to browse models.")
	_, _ = fmt.Fprintln(out)

	rl, err := readline.NewEx(&readline.Config{
		Prompt:      FormatChatPrompt("You", session.IsTTY),
		HistoryFile: os.TempDir() + "/omnillm-chat-history",
	})
	if err != nil {
		return fmt.Errorf("init readline: %w", err)
	}
	defer rl.Close()

	for {
		line, err := rl.Readline()
		if err != nil {
			if err == readline.ErrInterrupt || err == io.EOF {
				_, _ = fmt.Fprintln(out, "Goodbye.")
				break
			}
			return fmt.Errorf("read input: %w", err)
		}
		line = strings.TrimSpace(line)
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
				_, _ = fmt.Fprintln(out, "Goodbye.")
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

		_, _ = fmt.Fprintln(out, FormatChatHeader("Assistant", session.IsTTY))

		assistantContent, err := runChatCompletionStreaming(c, session.Model, messages, out, session.IsTTY)
		if err != nil {
			ErrorMsg(cmd, "completion: %v", err)
			continue
		}
		if assistantContent == "" {
			_, _ = fmt.Fprintln(out, "(no response)")
			continue
		}

		if err := postChatSessionMessage(c, session.ID, "assistant", assistantContent); err != nil {
			ErrorMsg(cmd, "store assistant message: %v", err)
		}
	}

	return nil
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
	ID           string
	Owner        string
	OwnerName    string
	Name         string
	Selector     string
	ProviderID   string
	ProviderName string
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

func showLoading(w io.Writer) func() {
	phrases := []string{"thinking", "processing", "reasoning", "computing", "generating"}
	stop := make(chan struct{})
	go func() {
		i := 0
		for {
			select {
			case <-stop:
				return
			default:
				fmt.Fprintf(w, "\r  %s... ", phrases[i%len(phrases)])
				i++
				time.Sleep(400 * time.Millisecond)
			}
		}
	}()
	return func() {
		close(stop)
		fmt.Fprint(w, "\r"+strings.Repeat(" ", 20)+"\r")
	}
}

// sseEvent represents a single Server-Sent Events chunk.
type sseEvent struct {
	data []byte
}

// parseSSEStreams reads an SSE body and yields events.
func parseSSEStreams(body io.Reader, onEvent func(sseEvent) error) error {
	scanner := bufio.NewScanner(body)
	scanner.Buffer(make([]byte, 0, 64*1024), 256*1024)
	var buf bytes.Buffer
	for scanner.Scan() {
		line := scanner.Bytes()
		if len(line) == 0 {
			if buf.Len() > 0 {
				if err := onEvent(sseEvent{data: buf.Bytes()}); err != nil {
					return err
				}
				buf.Reset()
			}
			continue
		}
		if bytes.HasPrefix(line, []byte("data: ")) {
			buf.Write(bytes.TrimPrefix(line, []byte("data: ")))
		} else if bytes.HasPrefix(line, []byte("data:")) {
			buf.Write(bytes.TrimPrefix(line, []byte("data:")))
		}
	}
	if buf.Len() > 0 {
		if err := onEvent(sseEvent{data: buf.Bytes()}); err != nil {
			return err
		}
	}
	return scanner.Err()
}

// runChatCompletionStreaming sends a streaming chat completion request and
// prints chunks as they arrive. Returns the full accumulated content.
func runChatCompletionStreaming(c *Client, model string, messages []chatMessage, w io.Writer, tty bool) (string, error) {
	reqModel := model
	if reqModel == "" {
		reqModel = "gpt-4"
	}

	completionBody := map[string]interface{}{
		"model":    reqModel,
		"messages": messages,
		"stream":   true,
	}

	resp, err := c.PostStream("/v1/chat/completions", completionBody)
	if err != nil {
		return "", err
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		data, _ := io.ReadAll(resp.Body)
		return "", fmt.Errorf("server error (%d): %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}

	stop := showLoading(w)
	var fullContent strings.Builder
	firstChunk := true

	err = parseSSEStreams(resp.Body, func(ev sseEvent) error {
		trimmed := bytes.TrimSpace(ev.data)
		if len(trimmed) == 0 || string(trimmed) == "[DONE]" {
			return nil
		}

		var chunk map[string]interface{}
		if err := json.Unmarshal(trimmed, &chunk); err != nil {
			return nil
		}

		choices, _ := chunk["choices"].([]interface{})
		if len(choices) == 0 {
			return nil
		}
		choice, _ := choices[0].(map[string]interface{})
		delta, _ := choice["delta"].(map[string]interface{})
		content, _ := delta["content"].(string)
		if content == "" {
			return nil
		}

		if firstChunk {
			stop()
			firstChunk = false
		}

		fullContent.WriteString(content)
		fmt.Fprint(w, content)
		return nil
	})
	if err != nil {
		if !firstChunk {
			fmt.Fprintln(w)
		}
		return fullContent.String(), fmt.Errorf("stream error: %w", err)
	}

	if !firstChunk {
		fmt.Fprint(w, "\n\n")
	} else {
		stop()
	}

	// Re-render with markdown/syntax highlighting after stream completes.
	if tty && fullContent.Len() > 0 {
		rendered := renderMarkdown(fullContent.String())
		if rendered != "" {
			// Clear the raw streamed text and show rendered output.
			clearLinesUp(w, fullContent.String())
			fmt.Fprint(w, rendered)
		}
	}

	return fullContent.String(), nil
}

// clearLinesUp moves the cursor up to overwrite previously printed lines.
func clearLinesUp(w io.Writer, content string) {
	lines := strings.Count(content, "\n") + 2
	for i := 0; i < lines; i++ {
		fmt.Fprint(w, "\033[1A\033[2K")
	}
}

// renderMarkdown renders markdown to ANSI terminal output using glamour.
func renderMarkdown(md string) string {
	r, err := glamour.NewTermRenderer(
		glamour.WithAutoStyle(),
		glamour.WithWordWrap(0),
	)
	if err != nil {
		return ""
	}
	out, err := r.Render(md)
	if err != nil {
		return ""
	}
	return out
}

func updateChatSessionModel(c *Client, sessionID string, modelID string) error {
	_, err := c.Put("/api/admin/chat/sessions/"+sessionID, map[string]string{
		"model_id": modelID,
	})
	return err
}

func listChatModels(c *Client) ([]chatModelInfo, error) {
	statusData, err := c.Get("/api/admin/status")
	if err != nil {
		return nil, err
	}

	var statusResp map[string]interface{}
	if err := json.Unmarshal(statusData, &statusResp); err != nil {
		return nil, err
	}

	type providerInfo struct {
		id   string
		name string
	}
	providers := make([]providerInfo, 0)
	if items, ok := statusResp["activeProviders"].([]interface{}); ok {
		for _, item := range items {
			entry, _ := item.(map[string]interface{})
			id, _ := entry["id"].(string)
			name, _ := entry["name"].(string)
			if id == "" {
				continue
			}
			providers = append(providers, providerInfo{id: id, name: name})
		}
	}

	models := make([]chatModelInfo, 0)
	for _, provider := range providers {
		data, err := c.Get("/api/admin/providers/" + provider.id + "/models")
		if err != nil {
			continue
		}

		var resp map[string]interface{}
		if err := json.Unmarshal(data, &resp); err != nil {
			continue
		}
		items, _ := resp["models"].([]interface{})
		for _, item := range items {
			entry, _ := item.(map[string]interface{})
			enabled, _ := entry["enabled"].(bool)
			if !enabled {
				continue
			}
			id, _ := entry["id"].(string)
			name, _ := entry["name"].(string)
			selector := chatModelSelector(chatModelInfo{ID: id, Owner: provider.id, Name: name})
			models = append(models, chatModelInfo{
				ID:           id,
				Owner:        provider.id,
				OwnerName:    provider.name,
				Name:         name,
				Selector:     selector,
				ProviderID:   provider.id,
				ProviderName: provider.name,
			})
		}
	}

	// Keep virtual models from /models because they don't live under a provider-specific admin list.
	allModelsData, err := c.Get("/models")
	if err == nil {
		var allResp map[string]interface{}
		if json.Unmarshal(allModelsData, &allResp) == nil {
			items, _ := allResp["data"].([]interface{})
			for _, item := range items {
				entry, _ := item.(map[string]interface{})
				owner, _ := entry["owned_by"].(string)
				if owner != "virtual" {
					continue
				}
				id, _ := entry["id"].(string)
				name, _ := entry["display_name"].(string)
				selector := chatModelSelector(chatModelInfo{ID: id, Owner: owner, Name: name})
				models = append(models, chatModelInfo{
					ID:           id,
					Owner:        owner,
					OwnerName:    "Virtual",
					Name:         name,
					Selector:     selector,
					ProviderID:   owner,
					ProviderName: "Virtual",
				})
			}
		}
	}

	sort.Slice(models, func(i, j int) bool {
		if models[i].ProviderName != models[j].ProviderName {
			return models[i].ProviderName < models[j].ProviderName
		}
		return models[i].Name < models[j].Name
	})
	return models, nil
}

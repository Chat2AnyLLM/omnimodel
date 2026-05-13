package chat

import (
	"encoding/json"
	"fmt"
	"time"
)

const DefaultAgentBackend = "omnicode"

func normalizeAgentBackend(_ string) string {
	return DefaultAgentBackend
}

// SessionSummary holds the metadata for a single chat session returned by the list endpoint.
type SessionSummary struct {
	ID           string    `json:"session_id"`
	Title        string    `json:"title"`
	Model        string    `json:"model_id"`
	APIShape     string    `json:"api_shape"`
	AgentBackend string    `json:"agent_backend"`
	CreatedAt    time.Time `json:"created_at"`
	UpdatedAt    time.Time `json:"updated_at"`
	MessageCount int       `json:"message_count"`
}

func EnsureSession(cmd CommandContext, c Client, existingSession string, requestedModel string) (SessionState, error) {
	if existingSession != "" {
		session, _, err := LoadSessionMessages(c, existingSession)
		if err != nil {
			return SessionState{}, err
		}
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Resuming session: %s\n", existingSession)
		if requestedModel != "" && requestedModel != session.Model {
			if err := UpdateSessionModel(c, existingSession, requestedModel); err != nil {
				return SessionState{}, err
			}
			session.Model = requestedModel
			_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Using model: %s\n", requestedModel)
		}
		return session, nil
	}

	body := map[string]any{
		"title":         "CLI session",
		"model_id":      requestedModel,
		"api_shape":     DefaultAPIShape,
		"agent_backend": DefaultAgentBackend,
	}
	data, err := c.Post("/api/admin/chat/sessions", body)
	if err != nil {
		return SessionState{}, fmt.Errorf("create session: %w", err)
	}
	var resp map[string]any
	if err := json.Unmarshal(data, &resp); err != nil {
		return SessionState{}, err
	}
	sid, ok := resp["session_id"].(string)
	if !ok {
		return SessionState{}, fmt.Errorf("server did not return a session_id")
	}
	_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Started session: %s\n", sid)
	if requestedModel != "" {
		_, _ = fmt.Fprintf(cmd.OutOrStdout(), "Using model: %s\n", requestedModel)
	}
	return SessionState{ID: sid, Model: requestedModel, Mode: "chat", APIShape: DefaultAPIShape, AgentBackend: DefaultAgentBackend}, nil
}

func LoadSessionMessages(c Client, sessionID string) (SessionState, []Message, error) {
	sessionData, err := c.Get("/api/admin/chat/sessions/" + sessionID)
	if err != nil {
		return SessionState{}, nil, fmt.Errorf("load session: %w", err)
	}
	var session map[string]any
	if err := json.Unmarshal(sessionData, &session); err != nil {
		return SessionState{}, nil, fmt.Errorf("parse session: %w", err)
	}

	state := SessionState{ID: sessionID, Mode: "chat", APIShape: DefaultAPIShape, AgentBackend: DefaultAgentBackend}
	if mid, ok := session["model_id"].(string); ok {
		state.Model = mid
	}
	if apiShape, ok := session["api_shape"].(string); ok && apiShape != "" {
		state.APIShape = canonicalAPIShape(apiShape)
	}
	if backend, ok := session["agent_backend"].(string); ok && backend != "" {
		state.AgentBackend = normalizeAgentBackend(backend)
	}

	var messages []Message
	if msgs, ok := session["messages"].([]any); ok {
		for _, m := range msgs {
			msg, _ := m.(map[string]any)
			role, _ := msg["role"].(string)
			content, _ := msg["content"].(string)
			messages = append(messages, Message{Role: role, Content: content})
		}
	}
	return state, messages, nil
}

func PostMessage(c Client, sessionID string, role string, content string) error {
	_, err := c.Post("/api/admin/chat/sessions/"+sessionID+"/messages", map[string]string{
		"role":    role,
		"content": content,
	})
	return err
}

func UpdateSessionModel(c Client, sessionID string, modelID string) error {
	_, err := c.Put("/api/admin/chat/sessions/"+sessionID, map[string]string{
		"model_id": modelID,
	})
	return err
}

func UpdateSessionAgentBackend(c Client, sessionID string, agentBackend string) error {
	_, err := c.Put("/api/admin/chat/sessions/"+sessionID, map[string]string{
		"agent_backend": normalizeAgentBackend(agentBackend),
	})
	return err
}

func UpdateSessionAPIShape(c Client, sessionID string, apiShape string) error {
	_, err := c.Put("/api/admin/chat/sessions/"+sessionID, map[string]string{
		"api_shape": canonicalAPIShape(apiShape),
	})
	return err
}

func CurrentModel(c Client, sessionID string, fallback string) (string, error) {
	session, _, err := LoadSessionMessages(c, sessionID)
	if err != nil {
		return "", err
	}
	if session.Model != "" {
		return session.Model, nil
	}
	return fallback, nil
}

func CurrentAgentBackend(c Client, sessionID string, fallback string) (string, error) {
	session, _, err := LoadSessionMessages(c, sessionID)
	if err != nil {
		return "", err
	}
	if session.AgentBackend != "" {
		return normalizeAgentBackend(session.AgentBackend), nil
	}
	return normalizeAgentBackend(fallback), nil
}

func CurrentAPIShape(c Client, sessionID string, fallback string) (string, error) {
	session, _, err := LoadSessionMessages(c, sessionID)
	if err != nil {
		return "", err
	}
	if session.APIShape != "" {
		return canonicalAPIShape(session.APIShape), nil
	}
	return canonicalAPIShape(fallback), nil
}

// ListSessions fetches the list of all chat sessions from the server.
func ListSessions(c Client) ([]SessionSummary, error) {
	data, err := c.Get("/api/admin/chat/sessions")
	if err != nil {
		return nil, fmt.Errorf("list sessions: %w", err)
	}
	// Accept either a bare array or {"sessions":[...]}
	var sessions []SessionSummary
	if err := json.Unmarshal(data, &sessions); err == nil {
		return sessions, nil
	}
	var wrapped struct {
		Sessions []SessionSummary `json:"sessions"`
	}
	if err := json.Unmarshal(data, &wrapped); err != nil {
		return nil, fmt.Errorf("parse sessions: %w", err)
	}
	return wrapped.Sessions, nil
}

// CreateSession creates a new chat session and returns its ID.
func CreateSession(c Client, title, model, apiShape, agentBackend string) (string, error) {
	if title == "" {
		title = "New session"
	}
	apiShape = canonicalAPIShape(apiShape)
	agentBackend = normalizeAgentBackend(agentBackend)
	body := map[string]any{
		"title":         title,
		"model_id":      model,
		"api_shape":     apiShape,
		"agent_backend": agentBackend,
	}
	data, err := c.Post("/api/admin/chat/sessions", body)
	if err != nil {
		return "", fmt.Errorf("create session: %w", err)
	}
	var resp map[string]any
	if err := json.Unmarshal(data, &resp); err != nil {
		return "", fmt.Errorf("parse create response: %w", err)
	}
	sid, ok := resp["session_id"].(string)
	if !ok {
		return "", fmt.Errorf("server did not return a session_id")
	}
	return sid, nil
}

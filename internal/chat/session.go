package chat

import (
	"encoding/json"
	"fmt"
)

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
		"api_shape":     "openai",
		"agent_backend": "agent-sdk-go",
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
	return SessionState{ID: sid, Model: requestedModel, Mode: "chat", AgentBackend: "agent-sdk-go"}, nil
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

	state := SessionState{ID: sessionID, Mode: "chat", AgentBackend: "agent-sdk-go"}
	if mid, ok := session["model_id"].(string); ok {
		state.Model = mid
	}
	if backend, ok := session["agent_backend"].(string); ok && backend != "" {
		state.AgentBackend = backend
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
		"agent_backend": agentBackend,
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
		return session.AgentBackend, nil
	}
	return fallback, nil
}

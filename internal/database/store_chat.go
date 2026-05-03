package database

import "database/sql"

// Chat operations
type ChatStore struct {
	db *Database
}

func NewChatStore() *ChatStore {
	return &ChatStore{db: GetDatabase()}
}

func (cs *ChatStore) CreateSession(sessionID, title, modelID, apiShape, agentBackend string) error {
	_, err := cs.db.db.Exec(`
		INSERT INTO chat_sessions (session_id, title, model_id, api_shape, agent_backend)
		VALUES (?, ?, ?, ?, ?)
	`, sessionID, title, modelID, apiShape, agentBackend)
	return err
}

func (cs *ChatStore) UpdateSessionTitle(sessionID, title string) error {
	_, err := cs.db.db.Exec(`
		UPDATE chat_sessions SET title = ?, updated_at = datetime('now')
		WHERE session_id = ?
	`, title, sessionID)
	return err
}

func (cs *ChatStore) UpdateSession(sessionID, title, modelID, agentBackend string) error {
	_, err := cs.db.db.Exec(`
		UPDATE chat_sessions
		SET title = COALESCE(NULLIF(?, ''), title),
		    model_id = COALESCE(NULLIF(?, ''), model_id),
		    agent_backend = COALESCE(NULLIF(?, ''), agent_backend),
		    updated_at = datetime('now')
		WHERE session_id = ?
	`, title, modelID, agentBackend, sessionID)
	return err
}

func (cs *ChatStore) TouchSession(sessionID string) error {
	_, err := cs.db.db.Exec(`
		UPDATE chat_sessions SET updated_at = datetime('now') WHERE session_id = ?
	`, sessionID)
	return err
}

func (cs *ChatStore) ListSessions() ([]ChatSessionRecord, error) {
	rows, err := cs.db.db.Query(`
		SELECT session_id, title, model_id, api_shape, agent_backend, created_at, updated_at
		FROM chat_sessions ORDER BY updated_at DESC, session_id ASC
	`)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var sessions []ChatSessionRecord
	for rows.Next() {
		var session ChatSessionRecord
		var createdAtStr, updatedAtStr string
		if err := rows.Scan(&session.SessionID, &session.Title, &session.ModelID, &session.APIShape, &session.AgentBackend, &createdAtStr, &updatedAtStr); err != nil {
			return nil, err
		}
		session.CreatedAt = parseTime(createdAtStr)
		session.UpdatedAt = parseTime(updatedAtStr)
		sessions = append(sessions, session)
	}
	return sessions, nil
}

func (cs *ChatStore) GetSession(sessionID string) (*ChatSessionRecord, error) {
	var session ChatSessionRecord
	var createdAtStr, updatedAtStr string
	err := cs.db.db.QueryRow(`
		SELECT session_id, title, model_id, api_shape, agent_backend, created_at, updated_at
		FROM chat_sessions WHERE session_id = ?
	`, sessionID).Scan(&session.SessionID, &session.Title, &session.ModelID, &session.APIShape, &session.AgentBackend, &createdAtStr, &updatedAtStr)

	if err == sql.ErrNoRows {
		return nil, nil
	}
	if err != nil {
		return nil, err
	}

	session.CreatedAt = parseTime(createdAtStr)
	session.UpdatedAt = parseTime(updatedAtStr)
	return &session, nil
}

func (cs *ChatStore) DeleteSession(sessionID string) error {
	_, err := cs.db.db.Exec("DELETE FROM chat_sessions WHERE session_id = ?", sessionID)
	return err
}

func (cs *ChatStore) DeleteAllSessions() error {
	_, err := cs.db.db.Exec("DELETE FROM chat_sessions")
	return err
}

func (cs *ChatStore) AddMessage(messageID, sessionID, role, content string) error {
	_, err := cs.db.db.Exec(`
		INSERT INTO chat_messages (message_id, session_id, role, content)
		VALUES (?, ?, ?, ?)
	`, messageID, sessionID, role, content)
	return err
}

func (cs *ChatStore) GetMessages(sessionID string) ([]ChatMessageRecord, error) {
	rows, err := cs.db.db.Query(`
		SELECT message_id, session_id, role, content, created_at
		FROM chat_messages WHERE session_id = ? ORDER BY created_at ASC, message_id ASC
	`, sessionID)
	if err != nil {
		return nil, err
	}
	defer rows.Close()

	var messages []ChatMessageRecord
	for rows.Next() {
		var message ChatMessageRecord
		var createdAtStr string
		if err := rows.Scan(&message.MessageID, &message.SessionID, &message.Role, &message.Content, &createdAtStr); err != nil {
			return nil, err
		}
		message.CreatedAt = parseTime(createdAtStr)
		messages = append(messages, message)
	}
	return messages, nil
}

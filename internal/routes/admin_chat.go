package routes

import (
	"crypto/rand"
	"encoding/hex"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/gin-gonic/gin"
	"github.com/rs/zerolog/log"

	"omnillm/internal/database"
)

// ─── Model version handlers ───────────────────────────────────────────────────

func handleGetModelVersion(c *gin.Context) {
	instanceID := c.Param("id")
	modelID := c.Param("modelId")

	configStore := database.NewModelConfigStore()
	record, err := configStore.Get(instanceID, modelID)
	if err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to get model version"})
		return
	}

	version := ""
	if record != nil {
		version = fmt.Sprintf("%d", record.Version)
	}
	c.JSON(http.StatusOK, gin.H{"version": version})
}

func handleSetModelVersion(c *gin.Context) {
	instanceID := c.Param("id")
	modelID := c.Param("modelId")

	var req struct {
		Version string `json:"version"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	configStore := database.NewModelConfigStore()
	if err := configStore.SetVersion(instanceID, modelID, req.Version); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to set model version"})
		return
	}
	c.JSON(http.StatusOK, gin.H{"success": true, "version": req.Version})
}

// ─── Chat session handlers ────────────────────────────────────────────────────

func handleGetChatSessions(c *gin.Context) {
	chatStore := database.NewChatStore()
	sessions, err := chatStore.ListSessions()
	if err != nil {
		log.Error().Err(err).Msg("Failed to get chat sessions")
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to retrieve chat sessions"})
		return
	}

	var sessionList []map[string]any
	for _, session := range sessions {
		sessionInfo := map[string]any{
			"session_id":    session.SessionID,
			"title":         session.Title,
			"model_id":      session.ModelID,
			"api_shape":     session.APIShape,
			"agent_backend": session.AgentBackend,
			"created_at":    session.CreatedAt.Format(time.RFC3339),
			"updated_at":    session.UpdatedAt.Format(time.RFC3339),
		}

		messages, err := chatStore.GetMessages(session.SessionID)
		if err == nil {
			sessionInfo["message_count"] = len(messages)
		} else {
			sessionInfo["message_count"] = 0
		}

		sessionList = append(sessionList, sessionInfo)
	}

	c.JSON(http.StatusOK, gin.H{
		"sessions": sessionList,
		"total":    len(sessionList),
	})
}

func generateSessionID() string {
	b := make([]byte, 16)
	if _, err := rand.Read(b); err != nil {
		return fmt.Sprintf("session-%d", time.Now().UnixNano())
	}
	return fmt.Sprintf("session-%s", hex.EncodeToString(b))
}

func handleCreateChatSession(c *gin.Context) {
	var req struct {
		SessionID    string `json:"session_id"`
		Title        string `json:"title"`
		ModelID      string `json:"model_id"`
		APIShape     string `json:"api_shape"`
		AgentBackend string `json:"agent_backend"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	if req.APIShape == "" {
		req.APIShape = "anthropic"
	}

	if req.AgentBackend == "" {
		req.AgentBackend = "google-adk"
	}

	sessionID := req.SessionID
	if sessionID == "" {
		sessionID = generateSessionID()
	}

	chatStore := database.NewChatStore()
	if err := chatStore.CreateSession(sessionID, req.Title, req.ModelID, req.APIShape, req.AgentBackend); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to create session"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success":    true,
		"session_id": sessionID,
	})
}

func handleDeleteAllChatSessions(c *gin.Context) {
	chatStore := database.NewChatStore()
	if err := chatStore.DeleteAllSessions(); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete sessions"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "All sessions deleted",
	})
}

func handleGetChatSession(c *gin.Context) {
	sessionID := c.Param("id")
	chatStore := database.NewChatStore()

	session, err := chatStore.GetSession(sessionID)
	if err != nil || session == nil {
		c.JSON(http.StatusNotFound, gin.H{"error": "Session not found"})
		return
	}

	messages, err := chatStore.GetMessages(sessionID)
	if err != nil {
		messages = nil
	}

	var messageList []map[string]any
	for _, msg := range messages {
		messageList = append(messageList, map[string]any{
			"id":         msg.MessageID,
			"role":       msg.Role,
			"content":    msg.Content,
			"created_at": msg.CreatedAt.Format(time.RFC3339),
		})
	}

	c.JSON(http.StatusOK, gin.H{
		"id":            session.SessionID,
		"title":         session.Title,
		"model_id":      session.ModelID,
		"api_shape":     session.APIShape,
		"agent_backend": session.AgentBackend,
		"created_at":    session.CreatedAt.Format(time.RFC3339),
		"updated_at":    session.UpdatedAt.Format(time.RFC3339),
		"messages":      messageList,
	})
}

func handleUpdateChatSession(c *gin.Context) {
	sessionID := c.Param("id")
	var req struct {
		Title        string `json:"title"`
		ModelID      string `json:"model_id"`
		APIShape     string `json:"api_shape"`
		AgentBackend string `json:"agent_backend"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	chatStore := database.NewChatStore()
	if err := chatStore.UpdateSession(sessionID, req.Title, req.ModelID, req.APIShape, req.AgentBackend); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to update session"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Session updated",
	})
}

func handleAddChatMessage(c *gin.Context) {
	sessionID := c.Param("id")
	var req struct {
		Role    string `json:"role"`
		Content string `json:"content"`
	}
	if err := c.ShouldBindJSON(&req); err != nil {
		c.JSON(http.StatusBadRequest, gin.H{"error": "Invalid request body"})
		return
	}

	messageID := fmt.Sprintf("msg-%d", time.Now().UnixNano())
	chatStore := database.NewChatStore()

	if err := chatStore.AddMessage(messageID, sessionID, req.Role, req.Content); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to add message"})
		return
	}

	// Touch session updated_at
	chatStore.TouchSession(sessionID)

	c.JSON(http.StatusOK, gin.H{
		"success":    true,
		"message_id": messageID,
	})
}

func handleDeleteChatSession(c *gin.Context) {
	sessionID := c.Param("id")
	chatStore := database.NewChatStore()

	if err := chatStore.DeleteSession(sessionID); err != nil {
		c.JSON(http.StatusInternalServerError, gin.H{"error": "Failed to delete session"})
		return
	}

	c.JSON(http.StatusOK, gin.H{
		"success": true,
		"message": "Session deleted",
	})
}

// ─── Log streaming (SSE) ──────────────────────────────────────────────────────

func handleLogsStream(c *gin.Context) {
	c.Header("Content-Type", "text/event-stream")
	c.Header("Cache-Control", "no-cache")
	c.Header("Connection", "keep-alive")
	c.Header("X-Accel-Buffering", "no")

	sub := &logSubscriber{
		ch:   make(chan string, 64),
		done: make(chan struct{}),
	}

	logSubscribersMu.Lock()
	logSubscribers[sub] = struct{}{}
	logSubscribersMu.Unlock()

	defer func() {
		logSubscribersMu.Lock()
		delete(logSubscribers, sub)
		logSubscribersMu.Unlock()
		close(sub.done)
	}()

	flusher, ok := c.Writer.(http.Flusher)
	if !ok {
		return
	}
	heartbeat := time.NewTicker(5 * time.Second)
	defer heartbeat.Stop()

	if _, err := io.WriteString(c.Writer, ": connected\n\n"); err != nil {
		return
	}
	flusher.Flush()

	for {
		select {
		case data := <-sub.ch:
			if _, err := io.WriteString(c.Writer, data); err != nil {
				return
			}
			flusher.Flush()
		case <-heartbeat.C:
			if _, err := io.WriteString(c.Writer, ": heartbeat\n\n"); err != nil {
				return
			}
			flusher.Flush()
		case <-c.Request.Context().Done():
			return
		}
	}
}

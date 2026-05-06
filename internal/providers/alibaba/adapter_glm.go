package alibaba

import (
	"omnillm/internal/providers/openaicompat"
	"strings"
)

func normalizeGLM51Messages(chatReq *openaicompat.ChatRequest) {
	chatReq.Messages = mergeLeadingSystemIntoFirstUser(chatReq.Messages)
	ensureToolAssistantContent(chatReq.Messages)
	ensureDashScopeToolCallAlias(chatReq.Messages)
}

func mergeLeadingSystemIntoFirstUser(messages []openaicompat.Message) []openaicompat.Message {
	if len(messages) == 0 || messages[0].Role != "system" {
		return messages
	}

	var systemParts []string
	firstNonSystem := 0
	for firstNonSystem < len(messages) && messages[firstNonSystem].Role == "system" {
		if systemText := strings.TrimSpace(messageTextContent(messages[firstNonSystem])); systemText != "" {
			systemParts = append(systemParts, systemText)
		}
		firstNonSystem++
	}
	if firstNonSystem == 0 {
		return messages
	}

	systemText := strings.Join(systemParts, "\n\n")
	remaining := append([]openaicompat.Message{}, messages[firstNonSystem:]...)
	if systemText == "" {
		return remaining
	}

	for i := range remaining {
		if remaining[i].Role != "user" {
			continue
		}
		mergeSystemTextIntoUserMessage(&remaining[i], systemText)
		return remaining
	}

	return append([]openaicompat.Message{{Role: "user", Content: systemText}}, remaining...)
}

func messageTextContent(message openaicompat.Message) string {
	switch content := message.Content.(type) {
	case string:
		return content
	case []openaicompat.ContentPart:
		var parts []string
		for _, part := range content {
			if part.Type == "text" && strings.TrimSpace(part.Text) != "" {
				parts = append(parts, strings.TrimSpace(part.Text))
			}
		}
		return strings.Join(parts, "\n\n")
	default:
		return ""
	}
}

func mergeSystemTextIntoUserMessage(message *openaicompat.Message, systemText string) {
	systemText = strings.TrimSpace(systemText)
	if systemText == "" {
		return
	}

	switch content := message.Content.(type) {
	case string:
		content = strings.TrimSpace(content)
		if content == "" {
			message.Content = systemText
			return
		}
		message.Content = systemText + "\n\n" + content
	case []openaicompat.ContentPart:
		for i := range content {
			if content[i].Type != "text" {
				continue
			}
			text := strings.TrimSpace(content[i].Text)
			if text == "" {
				content[i].Text = systemText
			} else {
				content[i].Text = systemText + "\n\n" + text
			}
			message.Content = content
			return
		}
		message.Content = append([]openaicompat.ContentPart{{Type: "text", Text: systemText}}, content...)
	default:
		message.Content = systemText
	}
}

func ensureToolAssistantContent(messages []openaicompat.Message) {
	for i := range messages {
		if messages[i].Role == "assistant" && messages[i].Content == nil && len(messages[i].ToolCalls) > 0 {
			messages[i].Content = ""
		}
	}
}

func ensureDashScopeToolCallAlias(messages []openaicompat.Message) {
	for i := range messages {
		if messages[i].Role != "assistant" {
			continue
		}
		for j := range messages[i].ToolCalls {
			if messages[i].ToolCalls[j].ID != "" && messages[i].ToolCalls[j].CallID == "" {
				messages[i].ToolCalls[j].CallID = messages[i].ToolCalls[j].ID
			}
		}
	}
}

package agent

import "strings"

const DefaultAPIShape = "anthropic"

func normalizeAPIShape(shape string) string {
	switch strings.ToLower(strings.TrimSpace(shape)) {
	case "", "anthropic", "messages", "message", "v1/messages", "/v1/messages":
		return DefaultAPIShape
	case "openai", "chat", "chat-completions", "chat_completions", "v1/chat/completions", "/v1/chat/completions":
		return "openai"
	case "responses", "response", "v1/responses", "/v1/responses":
		return "responses"
	default:
		return DefaultAPIShape
	}
}

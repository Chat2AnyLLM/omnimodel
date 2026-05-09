package chat

import (
	"fmt"
	"strings"
)

const DefaultAPIShape = "anthropic"

func normalizeAPIShape(shape string) (string, bool) {
	switch strings.ToLower(strings.TrimSpace(shape)) {
	case "", "anthropic", "messages", "message", "v1/messages", "/v1/messages":
		return "anthropic", true
	case "openai", "chat", "chat-completions", "chat_completions", "v1/chat/completions", "/v1/chat/completions":
		return "openai", true
	case "responses", "response", "v1/responses", "/v1/responses":
		return "responses", true
	default:
		return "", false
	}
}

func canonicalAPIShape(shape string) string {
	if normalized, ok := normalizeAPIShape(shape); ok {
		return normalized
	}
	return DefaultAPIShape
}

func apiShapeEndpoint(shape string) string {
	switch canonicalAPIShape(shape) {
	case "openai":
		return "/v1/chat/completions"
	case "responses":
		return "/v1/responses"
	default:
		return "/v1/messages"
	}
}

func formatAPIShape(shape string) string {
	canonical := canonicalAPIShape(shape)
	return fmt.Sprintf("%s (%s)", canonical, apiShapeEndpoint(canonical))
}

func supportedAPIShapesText() string {
	return "anthropic (/v1/messages), openai (/v1/chat/completions)"
}

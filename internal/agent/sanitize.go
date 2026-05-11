package agent

import (
	"fmt"
	"strings"
)

const maxToolResultChars = 16000

// injectionPatterns are lower-cased substrings that suggest a prompt-injection
// attempt in externally-supplied text (user prompts, tool results, file contents).
var injectionPatterns = []string{
	"ignore previous instructions",
	"ignore all previous",
	"disregard previous",
	"forget your instructions",
	"forget all previous",
	"you are now",
	"act as if you are",
	"pretend you are",
	"your new instructions",
	"your true instructions",
	"system prompt:",
	"<system>",
	"[system]",
	"### system",
	"## system",
	"-- system",
}

// SanitizeUserInput scans user-supplied text for common prompt-injection
// patterns. When patterns are detected the text is returned unchanged but
// prefixed with an advisory comment so the model treats it as untrusted.
// The second return value is true when injection was detected.
func SanitizeUserInput(text string) (string, bool) {
	if hasPromptInjectionPattern(text) {
		return "<!-- WARNING: potential prompt injection detected; treating as untrusted user content -->\n" + text, true
	}
	return text, false
}

func hasPromptInjectionPattern(text string) bool {
	lower := strings.ToLower(text)
	for _, p := range injectionPatterns {
		if strings.Contains(lower, p) {
			return true
		}
	}
	return false
}

func sanitizeToolResultForModel(toolName, content string, isError bool) string {
	cleaned := strings.ReplaceAll(content, "\x00", "")
	cleaned = strings.ReplaceAll(cleaned, "\r\n", "\n")
	cleaned = strings.TrimSpace(cleaned)
	if cleaned == "" {
		cleaned = "(no output)"
	}

	if len(cleaned) > maxToolResultChars {
		cleaned = cleaned[:maxToolResultChars] + "\n[TRUNCATED]"
	}

	status := "ok"
	if isError {
		status = "error"
	}
	warning := ""
	if hasPromptInjectionPattern(cleaned) {
		warning = "\nwarning: potential_prompt_injection_detected"
	}

	return fmt.Sprintf(
		"<tool_result>\ntool: %s\nstatus: %s\nuntrusted: true%s\n---\n%s\n</tool_result>",
		toolName,
		status,
		warning,
		cleaned,
	)
}

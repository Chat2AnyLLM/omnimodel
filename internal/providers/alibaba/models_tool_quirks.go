package alibaba

import (
	"omnillm/internal/providers/openaicompat"
	"strings"
)

// needsToolChoiceNil reports whether this DashScope model requires tool_choice
// to be omitted (nil) when tools are present. All Qwen and GLM models on
// DashScope reject explicit tool_choice values; DeepSeek models do not.
func needsToolChoiceNil(modelID string) bool {
	lower := strings.ToLower(RemapModel(modelID))
	return strings.HasPrefix(lower, "qwen") || strings.HasPrefix(lower, "glm")
}

// dashScopeToolCallAliasModels: DashScope models that require both "id" and
// "call_id" on assistant tool_calls in message history.
var dashScopeToolCallAliasModels = map[string]struct{}{
	"deepseek-v3":       {},
	"deepseek-v4-flash": {},
	"qwen3.5-plus":      {},
	"qwen3.6-plus":      {},
	"glm-5.1":           {},
}

// omitToolsAfterToolResultModels: DashScope models that reject the tools list
// being repeated in requests that already contain a tool-role message.
var omitToolsAfterToolResultModels = map[string]struct{}{
	"deepseek-v3":       {},
	"deepseek-v4-flash": {},
}

func needsDashScopeToolCallAlias(modelID string) bool {
	_, ok := dashScopeToolCallAliasModels[strings.ToLower(RemapModel(modelID))]
	return ok
}

func omitToolsAfterToolResult(modelID string, messages []openaicompat.Message) bool {
	_, ok := omitToolsAfterToolResultModels[strings.ToLower(RemapModel(modelID))]
	if !ok {
		return false
	}
	for _, message := range messages {
		if message.Role == "tool" {
			return true
		}
	}
	return false
}

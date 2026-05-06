package alibaba

import (
	"omnillm/internal/providers/openaicompat"
	"strings"
)

func needsDashScopeToolCallAlias(modelID string) bool {
	_, ok := dashScopeToolCallAliasModelIDs[strings.ToLower(RemapModel(modelID))]
	return ok
}

func omitToolsAfterToolResult(modelID string, messages []openaicompat.Message) bool {
	_, ok := omitToolsAfterToolResultModelIDs[strings.ToLower(RemapModel(modelID))]
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

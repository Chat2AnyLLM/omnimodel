package alibaba

import "strings"

// nonReasoningToolModelIDs are models that are NOT reasoning models but require
// special tool handling when tools are present on DashScope. These are typically
// third-party models (GLM, Qwen3.5-Plus) that reject tool_choice and require
// explicit empty content for tool-only assistant messages.
var nonReasoningToolModelIDs = map[string]struct{}{
	"qwen3.5-plus": {},
	"glm-5.1":      {},
}

func isNonReasoningToolModel(modelID string) bool {
	_, ok := nonReasoningToolModelIDs[strings.ToLower(RemapModel(modelID))]
	return ok
}

package alibaba

import (
	"omnillm/internal/providers/types"
	"strings"
)

func RemapModel(modelID string) string {
	trimmed := strings.TrimSpace(modelID)
	if idx := strings.LastIndex(trimmed, "/"); idx >= 0 {
		return strings.TrimSpace(trimmed[idx+1:])
	}
	return trimmed
}

func IsChatCompletionsModel(modelID string) bool {
	return !strings.Contains(strings.ToLower(modelID), "realtime")
}

func IsReasoningModel(modelID string) bool {
	_, ok := reasoningModelIDs[strings.ToLower(RemapModel(modelID))]
	return ok
}

func ModelMetadata(modelID string) (types.Model, bool) {
	remapped := strings.ToLower(RemapModel(modelID))
	for _, m := range Models {
		if m.ID == remapped {
			return m, true
		}
	}
	return types.Model{}, false
}

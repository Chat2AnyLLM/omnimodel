package alibaba

import "strings"

func isQwenReasoningModel(modelID string) bool {
	_, ok := qwenReasoningModelIDs[strings.ToLower(RemapModel(modelID))]
	return ok
}

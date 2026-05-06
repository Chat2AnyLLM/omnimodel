package alibaba

import "strings"

func isNonReasoningToolModel(modelID string) bool {
	_, ok := nonReasoningToolModelIDs[strings.ToLower(RemapModel(modelID))]
	return ok
}

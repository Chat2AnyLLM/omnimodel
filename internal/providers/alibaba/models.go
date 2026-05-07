package alibaba

import (
	"context"
	"omnillm/internal/services/modelsmeta"
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

// IsReasoningModel reports whether the model supports extended reasoning output.
// It consults models.dev (via the shared DefaultService cache); returns false if
// the model is not found or the service is unavailable.
func IsReasoningModel(ctx context.Context, modelID string) bool {
	return isReasoningModelWith(
		func(id string) *modelsmeta.ModelMetadata {
			return modelsmeta.DefaultService.LookupModel(ctx, id)
		},
		modelID,
	)
}

// isReasoningModelWith is the testable core: the caller supplies the lookup so
// tests can inject a stub without needing a live models.dev connection.
func isReasoningModelWith(lookup func(string) *modelsmeta.ModelMetadata, modelID string) bool {
	meta := lookup(strings.ToLower(RemapModel(modelID)))
	if meta != nil && meta.SupportsReasoning != nil {
		return *meta.SupportsReasoning
	}
	return false
}

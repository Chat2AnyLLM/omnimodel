package alibaba

import "strings"

var qwenReasoningModelIDs = map[string]struct{}{
	"qwen3.6-max-preview":      {},
	"qwen3.6-plus":             {},
	"qwen3.6-flash":            {},
	"qwen3-coder-next":         {},
	"qwen3-coder-plus":         {},
	"qwen3-coder-flash":        {},
	"qwen3-max":                {},
	"qwen3-max-preview":        {},
	"qwen3-32b":                {},
	"qwen3-235b-a22b-instruct": {},
	"qwen-plus":                {},
}

func isQwenReasoningModel(modelID string) bool {
	_, ok := qwenReasoningModelIDs[strings.ToLower(RemapModel(modelID))]
	return ok
}

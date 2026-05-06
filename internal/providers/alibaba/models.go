package alibaba

import (
	"omnillm/internal/providers/types"
	"strings"
)

var Models = []types.Model{
	{ID: "qwen3.6-max-preview", Name: "Qwen3.6 Max Preview", MaxTokens: 32768, Provider: "alibaba"},
	{ID: "qwen3.6-plus", Name: "Qwen3.6 Plus", MaxTokens: 131072, Provider: "alibaba"},
	{ID: "qwen3.6-flash", Name: "Qwen3.6 Flash", MaxTokens: 131072, Provider: "alibaba"},
	{ID: "qwen3.5-plus", Name: "Qwen3.5 Plus", MaxTokens: 131072, Provider: "alibaba"},
	{ID: "qwen3.5-omni-flash", Name: "Qwen3.5 Omni Flash", MaxTokens: 131072, Provider: "alibaba"},
	{ID: "qwen3-coder-next", Name: "Qwen3 Coder Next", MaxTokens: 131072, Provider: "alibaba"},
	{ID: "qwen3-coder-plus", Name: "Qwen3 Coder Plus", MaxTokens: 131072, Provider: "alibaba"},
	{ID: "qwen3-coder-flash", Name: "Qwen3 Coder Flash", MaxTokens: 131072, Provider: "alibaba"},
	{ID: "qwen3-max", Name: "Qwen3 Max", MaxTokens: 32768, Provider: "alibaba"},
	{ID: "qwen3-max-preview", Name: "Qwen3 Max Preview", MaxTokens: 32768, Provider: "alibaba"},
	{ID: "qwen3-32b", Name: "Qwen3-32B", MaxTokens: 131072, Provider: "alibaba"},
	{ID: "qwen3-235b-a22b-instruct", Name: "Qwen3-235B-A22B Instruct", MaxTokens: 131072, Provider: "alibaba"},
	{ID: "qwen-plus", Name: "Qwen Plus", MaxTokens: 131072, Provider: "alibaba"},
	{ID: "qwen-turbo", Name: "Qwen Turbo", MaxTokens: 1000000, Provider: "alibaba"},
	{ID: "glm-5.1", Name: "GLM 5.1", MaxTokens: 131072, Provider: "alibaba"},
	{ID: "deepseek-v3", Name: "DeepSeek V3", MaxTokens: 65536, Provider: "alibaba"},
	{ID: "deepseek-v4-flash", Name: "DeepSeek V4 Flash", MaxTokens: 65536, Provider: "alibaba"},
	{ID: "deepseek-r1", Name: "DeepSeek R1", MaxTokens: 65536, Provider: "alibaba"},
	{ID: "deepseek-r1-0528", Name: "DeepSeek R1 0528", MaxTokens: 65536, Provider: "alibaba"},
}

var reasoningModelIDs = map[string]struct{}{
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
	"deepseek-r1":              {},
	"deepseek-r1-0528":         {},
}

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

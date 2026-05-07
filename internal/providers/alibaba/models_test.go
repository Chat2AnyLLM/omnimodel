package alibaba

import (
	"omnillm/internal/services/modelsmeta"
	"testing"
)

func TestIsChatCompletionsModel(t *testing.T) {
	cases := []struct {
		modelID string
		want    bool
	}{
		{"qwen3-max", true},
		{"qwen3-coder-plus", true},
		{"qwen-turbo", true},
		{"qwen-realtime-v1", false},
		{"REALTIME-model", false},
	}
	for _, tc := range cases {
		got := IsChatCompletionsModel(tc.modelID)
		if got != tc.want {
			t.Errorf("IsChatCompletionsModel(%q) = %v, want %v", tc.modelID, got, tc.want)
		}
	}
}

func boolPtr(b bool) *bool { return &b }

func TestIsReasoningModelWith(t *testing.T) {
	cases := []struct {
		name    string
		meta    *modelsmeta.ModelMetadata
		modelID string
		want    bool
	}{
		{"reasoning=true from models.dev", &modelsmeta.ModelMetadata{SupportsReasoning: boolPtr(true)}, "qwen3-max", true},
		{"reasoning=false from models.dev", &modelsmeta.ModelMetadata{SupportsReasoning: boolPtr(false)}, "glm-5.1", false},
		{"no models.dev entry (nil)", nil, "unknown-model", false},
		{"models.dev entry but SupportsReasoning nil", &modelsmeta.ModelMetadata{}, "some-model", false},
		{"model ID case folded", &modelsmeta.ModelMetadata{SupportsReasoning: boolPtr(true)}, "QWEN3-MAX", true},
		{"provider-prefixed ID stripped", &modelsmeta.ModelMetadata{SupportsReasoning: boolPtr(true)}, "alibaba-sk/qwen3-max", true},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			got := isReasoningModelWith(func(_ string) *modelsmeta.ModelMetadata {
				return tc.meta
			}, tc.modelID)
			if got != tc.want {
				t.Errorf("isReasoningModelWith() = %v, want %v", got, tc.want)
			}
		})
	}
}

func TestRemapModel(t *testing.T) {
	cases := []struct{ in, want string }{
		{"qwen3-max", "qwen3-max"},
		{"  qwen3-coder-plus  ", "qwen3-coder-plus"},
		{"alibaba-sk-ab2c5/deepseek-v4-flash", "deepseek-v4-flash"},
		{"  alibaba-sk-ab2c5/deepseek-v4-flash  ", "deepseek-v4-flash"},
	}
	for _, tc := range cases {
		if got := RemapModel(tc.in); got != tc.want {
			t.Errorf("RemapModel(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

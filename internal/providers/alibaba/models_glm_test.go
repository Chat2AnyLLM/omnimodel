package alibaba

import "testing"

func TestIsNonReasoningToolModel(t *testing.T) {
	cases := []struct {
		modelID string
		want    bool
	}{
		{"qwen3.5-plus", true},
		{"glm-5.1", true},
		{"QWEN3.5-PLUS", true},
		{"alibaba-sk-ab2c5/qwen3.5-plus", true},
		{"qwen3-max", false},
		{"qwen-turbo", false},
		{"deepseek-v4-flash", false},
		{"gpt-4o", false},
		{"", false},
	}
	for _, tc := range cases {
		got := isNonReasoningToolModel(tc.modelID)
		if got != tc.want {
			t.Errorf("isNonReasoningToolModel(%q) = %v, want %v", tc.modelID, got, tc.want)
		}
	}
}

package alibaba

import "testing"

func TestNeedsToolChoiceNil(t *testing.T) {
	cases := []struct {
		modelID string
		want    bool
	}{
		{"qwen3-max", true},
		{"qwen3.5-plus", true},
		{"qwen3.6-plus", true},
		{"qwen-turbo", true},
		{"QWEN3-MAX", true},
		{"alibaba-sk/qwen3.6-plus", true},
		{"glm-5.1", true},
		{"GLM-5.1", true},
		{"deepseek-v3", false},
		{"deepseek-v4-flash", false},
		{"deepseek-r1", false},
		{"gpt-4o", false},
		{"", false},
	}
	for _, tc := range cases {
		got := needsToolChoiceNil(tc.modelID)
		if got != tc.want {
			t.Errorf("needsToolChoiceNil(%q) = %v, want %v", tc.modelID, got, tc.want)
		}
	}
}

package alibaba

import "testing"

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

func TestIsReasoningModel(t *testing.T) {
	cases := []struct {
		modelID string
		want    bool
	}{
		{"qwen3-max", true},
		{"qwen3-coder-plus", true},
		{"qwen3-235b-a22b-instruct", true},
		{"qwen-plus", true},
		{"qwen3.6-plus", true},
		{"qwen3.6-flash", true},
		{"deepseek-r1", true},
		{"deepseek-r1-0528", true},
		{"deepseek-v4-flash", false},
		{"QWEN3-MAX", true},
		{"glm-5.1", false},
		{"qwen2-5-72b-instruct", false},
		{"qwen-turbo", false},
		{"gpt-4o", false},
		{"", false},
	}
	for _, tc := range cases {
		got := IsReasoningModel(tc.modelID)
		if got != tc.want {
			t.Errorf("IsReasoningModel(%q) = %v, want %v", tc.modelID, got, tc.want)
		}
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

package agent

import "testing"

func TestNormalizeAPIShape(t *testing.T) {
	cases := []struct {
		in   string
		want string
	}{
		{"", "anthropic"},
		{"anthropic", "anthropic"},
		{"messages", "anthropic"},
		{"/v1/messages", "anthropic"},
		{"openai", "openai"},
		{"chat", "openai"},
		{"/v1/chat/completions", "openai"},
		{"responses", "responses"},
		{"/v1/responses", "responses"},
		{"unknown", "anthropic"},
	}

	for _, tc := range cases {
		if got := normalizeAPIShape(tc.in); got != tc.want {
			t.Fatalf("normalizeAPIShape(%q) = %q, want %q", tc.in, got, tc.want)
		}
	}
}

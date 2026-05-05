package chat

import "testing"

func TestNormalizeAPIShape(t *testing.T) {
	cases := []struct {
		in   string
		want string
		ok   bool
	}{
		{"", "anthropic", true},
		{"anthropic", "anthropic", true},
		{"messages", "anthropic", true},
		{"/v1/messages", "anthropic", true},
		{"openai", "openai", true},
		{"chat", "openai", true},
		{"/v1/chat/completions", "openai", true},
		{"responses", "responses", true},
		{"/v1/responses", "responses", true},
		{"unknown", "", false},
	}

	for _, tc := range cases {
		got, ok := normalizeAPIShape(tc.in)
		if got != tc.want || ok != tc.ok {
			t.Fatalf("normalizeAPIShape(%q) = (%q, %v), want (%q, %v)", tc.in, got, ok, tc.want, tc.ok)
		}
	}
}

func TestFormatAPIShape(t *testing.T) {
	if got := formatAPIShape("openai"); got != "openai (/v1/chat/completions)" {
		t.Fatalf("formatAPIShape(openai) = %q", got)
	}
	if got := formatAPIShape("bogus"); got != "anthropic (/v1/messages)" {
		t.Fatalf("formatAPIShape(bogus) = %q", got)
	}
}

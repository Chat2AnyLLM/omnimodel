package agent

import (
	"strings"
	"testing"
)

func TestSanitizeToolResultForModelWrapsAsUntrusted(t *testing.T) {
	out := sanitizeToolResultForModel("read_file", "hello", false)
	if !strings.Contains(out, "<tool_result>") || !strings.Contains(out, "</tool_result>") {
		t.Fatalf("expected tool_result wrapper, got %q", out)
	}
	if !strings.Contains(out, "untrusted: true") {
		t.Fatalf("expected untrusted marker, got %q", out)
	}
	if !strings.Contains(out, "status: ok") {
		t.Fatalf("expected status ok, got %q", out)
	}
}

func TestSanitizeToolResultForModelTruncatesLongContent(t *testing.T) {
	long := strings.Repeat("a", maxToolResultChars+100)
	out := sanitizeToolResultForModel("bash", long, true)
	if !strings.Contains(out, "[TRUNCATED]") {
		t.Fatalf("expected truncation marker, got %q", out)
	}
	if !strings.Contains(out, "status: error") {
		t.Fatalf("expected status error, got %q", out)
	}
}

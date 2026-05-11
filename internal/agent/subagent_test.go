package agent

import (
	"testing"

	"omnillm/internal/tools"
)

func TestRegisterSubAgentToolsExcludesRecursiveAndWorktreeTools(t *testing.T) {
	r := tools.NewRegistry()
	registerSubAgentTools(r)

	forbidden := []string{"agent", "send_message", "enter_worktree", "exit_worktree"}
	for _, name := range forbidden {
		if tool := r.Get(name); tool != nil {
			t.Fatalf("expected %q to be excluded from sub-agent tools", name)
		}
	}

	if r.Get("bash") == nil {
		t.Fatal("expected core tool bash to be present in sub-agent tools")
	}
}

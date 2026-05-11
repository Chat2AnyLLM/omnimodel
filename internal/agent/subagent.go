package agent

import (
	"context"
	"fmt"
	"sync/atomic"

	"omnillm/internal/tools"
)

// SubAgentOptions configures how spawned sub-agents are created.
type SubAgentOptions struct {
	Client   Client
	Model    string
	Backend  string
	APIShape string
	MaxTurns int
	Checker  tools.PermissionChecker
	AskUser  func(ctx context.Context, question string, options []string) (string, error)
}

var subAgentCounter uint64

// MakeSubAgentFn returns a SendMessageFn that spawns a fully isolated sub-agent
// run for each call. Each sub-agent gets its own Registry and Memory so state
// cannot leak between parent and child.
//
// Recursive spawning is prevented by omitting the "agent" and "send_message"
// tools from the sub-agent's registry.
func MakeSubAgentFn(opts SubAgentOptions) func(ctx context.Context, to, message string) (string, error) {
	if opts.MaxTurns <= 0 {
		opts.MaxTurns = 10
	}
	return func(ctx context.Context, to, message string) (string, error) {
		id := atomic.AddUint64(&subAgentCounter, 1)
		sessionID := fmt.Sprintf("subagent-%s-%d", sanitizeSubAgentID(to), id)

		// Isolated registry — no SendMessageFn wired to prevent unbounded recursion.
		registry := tools.NewRegistry()
		registerSubAgentTools(registry)
		if opts.Checker != nil {
			registry.SetPermissionChecker(opts.Checker)
		}
		if opts.AskUser != nil {
			registry.SetAskUserCallback(opts.AskUser)
		}
		tools.InitRegistryStores(registry)
		tools.InitSkillMembership(registry)

		// Fresh memory + persistent context for the sub-agent.
		memory := NewBufferMemory(64)
		sysPrompt := buildSystemPrompt()
		memory.Append(Message{Role: "system", Content: []ContentBlock{TextBlock(sysPrompt)}})
		wsDir := workspaceDir()
		if pc := LoadWorkspaceContext(wsDir); !pc.IsEmpty() {
			injectPersistentContext(memory, pc)
		}

		dispatch := selectDispatch(opts.Client, opts.Model, opts.Backend, opts.APIShape)
		ag := NewAgent(registry, memory, opts.MaxTurns, dispatch)

		_ = AppendDailyLog(wsDir, fmt.Sprintf("[%s] subagent_start parent_target=%q", sessionID, to))

		result, err := ag.Run(ctx, sessionID, message)
		if err != nil {
			_ = AppendDailyLog(wsDir, fmt.Sprintf("[%s] subagent_error err=%q", sessionID, trimForLog(err.Error(), 200)))
			return "", fmt.Errorf("sub-agent %q failed: %w", to, err)
		}

		_ = AppendDailyLog(wsDir, fmt.Sprintf("[%s] subagent_done steps=%d", sessionID, result.Steps))
		return result.Output, nil
	}
}

// registerSubAgentTools registers core tools for sub-agents, excluding
// recursive and workspace-global mutating tools.
func registerSubAgentTools(registry *tools.Registry) {
	m := tools.NewManager()
	tools.RegisterCoreTools(m)
	for _, t := range m.Registry().List() {
		switch t.Name() {
		case "agent", "send_message", "enter_worktree", "exit_worktree":
			// Intentionally excluded: prevent recursive spawning and worktree
			// interference between parent/child sessions.
			continue
		}
		registry.Register(t)
	}
}

// sanitizeSubAgentID converts an arbitrary string to a safe filename/ID component.
func sanitizeSubAgentID(s string) string {
	if s == "" {
		return "worker"
	}
	out := make([]byte, 0, 32)
	for i := 0; i < len(s) && len(out) < 32; i++ {
		c := s[i]
		switch {
		case c >= 'a' && c <= 'z', c >= 'A' && c <= 'Z', c >= '0' && c <= '9', c == '-', c == '_':
			out = append(out, c)
		default:
			out = append(out, '_')
		}
	}
	return string(out)
}

package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
)

// ─── WorktreeState ────────────────────────────────────────────────────────────

// WorktreeState tracks an active git worktree for a session.
type WorktreeState struct {
	mu      sync.Mutex
	active  bool
	path    string
	branch  string
	origDir string
}

func NewWorktreeState() *WorktreeState { return &WorktreeState{} }

func (s *WorktreeState) IsActive() bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.active
}

func (s *WorktreeState) Get() (path, branch, origDir string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.path, s.branch, s.origDir
}

func (s *WorktreeState) Enter(path, branch, origDir string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.active = true
	s.path = path
	s.branch = branch
	s.origDir = origDir
}

func (s *WorktreeState) Exit() (path, branch string) {
	s.mu.Lock()
	defer s.mu.Unlock()
	path, branch = s.path, s.branch
	s.active = false
	s.path = ""
	s.branch = ""
	s.origDir = ""
	return
}

// ─── enter_worktree ───────────────────────────────────────────────────────────

type enterWorktreeTool struct{}

func EnterWorktree() Tool { return &enterWorktreeTool{} }

func (t *enterWorktreeTool) Name() string { return "enter_worktree" }
func (t *enterWorktreeTool) Description() string {
	return "Create and switch into a new git worktree for isolated experimentation. " +
		"All subsequent file operations will happen inside the worktree. " +
		"Call exit_worktree to return and optionally remove the worktree."
}
func (t *enterWorktreeTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"name":   map[string]any{"type": "string", "description": "Name for the new worktree (used as branch name suffix). Generated automatically if omitted."},
			"branch": map[string]any{"type": "string", "description": "New branch name. Defaults to 'worktree/<name>'."},
		},
	}
}
func (t *enterWorktreeTool) Execute(ctx context.Context, call Context, input json.RawMessage) Result {
	var p struct {
		Name   string `json:"name"`
		Branch string `json:"branch"`
	}
	_ = json.Unmarshal(input, &p)

	ws := call.WorktreeState
	if ws == nil {
		return Result{Output: "error: worktree state not available", IsError: true}
	}
	if ws.IsActive() {
		path, _, _ := ws.Get()
		return Result{Output: fmt.Sprintf("error: already in worktree at %s — call exit_worktree first", path), IsError: true}
	}

	origDir, err := os.Getwd()
	if err != nil {
		return Result{Output: "error: cannot determine working directory: " + err.Error(), IsError: true}
	}

	// Find git root
	gitRoot, err := gitRoot(origDir)
	if err != nil {
		return Result{Output: "error: not in a git repository: " + err.Error(), IsError: true}
	}

	name := strings.TrimSpace(p.Name)
	if name == "" {
		name = fmt.Sprintf("wt-%d", uniqueID())
	}
	branch := strings.TrimSpace(p.Branch)
	if branch == "" {
		branch = "worktree/" + name
	}

	wtPath := filepath.Join(gitRoot, ".git", "worktrees-agent", name)
	if err := os.MkdirAll(filepath.Dir(wtPath), 0o755); err != nil {
		return Result{Output: "error: create worktree dir: " + err.Error(), IsError: true}
	}

	cmd := exec.CommandContext(ctx, "git", "worktree", "add", "-b", branch, wtPath, "HEAD")
	cmd.Dir = gitRoot
	out, err := cmd.CombinedOutput()
	if err != nil {
		return Result{Output: "error: git worktree add: " + strings.TrimSpace(string(out)), IsError: true}
	}

	if err := os.Chdir(wtPath); err != nil {
		return Result{Output: "error: chdir to worktree: " + err.Error(), IsError: true}
	}

	ws.Enter(wtPath, branch, origDir)
	return Result{
		Title:  "Entered worktree",
		Output: fmt.Sprintf("Worktree created at: %s\nBranch: %s\nCall exit_worktree to return.", wtPath, branch),
	}
}

// ─── exit_worktree ────────────────────────────────────────────────────────────

type exitWorktreeTool struct{}

func ExitWorktree() Tool { return &exitWorktreeTool{} }

func (t *exitWorktreeTool) Name() string { return "exit_worktree" }
func (t *exitWorktreeTool) Description() string {
	return "Exit the current git worktree and return to the original working directory. " +
		"Optionally remove the worktree and its branch."
}
func (t *exitWorktreeTool) InputSchema() map[string]any {
	return map[string]any{
		"type": "object",
		"properties": map[string]any{
			"action": map[string]any{
				"type":        "string",
				"enum":        []string{"keep", "remove"},
				"description": "'keep' preserves the worktree on disk; 'remove' deletes it. Defaults to 'keep'.",
			},
		},
	}
}
func (t *exitWorktreeTool) Execute(ctx context.Context, call Context, input json.RawMessage) Result {
	var p struct {
		Action string `json:"action"`
	}
	_ = json.Unmarshal(input, &p)
	if p.Action == "" {
		p.Action = "keep"
	}

	ws := call.WorktreeState
	if ws == nil {
		return Result{Output: "error: worktree state not available", IsError: true}
	}
	if !ws.IsActive() {
		return Result{Output: "Not in a worktree."}
	}

	wtPath, branch, origDir := ws.Get()
	ws.Exit()

	if err := os.Chdir(origDir); err != nil {
		return Result{Output: fmt.Sprintf("warning: could not chdir back to %s: %v", origDir, err)}
	}

	if p.Action == "remove" {
		cmd := exec.CommandContext(ctx, "git", "worktree", "remove", "--force", wtPath)
		cmd.Dir = origDir
		out, err := cmd.CombinedOutput()
		if err != nil {
			return Result{
				Output: fmt.Sprintf("Returned to %s but failed to remove worktree: %s", origDir, strings.TrimSpace(string(out))),
				IsError: true,
			}
		}
		// Also delete the branch
		exec.CommandContext(ctx, "git", "branch", "-D", branch).Run() //nolint
		return Result{
			Title:  "Exited and removed worktree",
			Output: fmt.Sprintf("Returned to: %s\nWorktree %s removed, branch %s deleted.", origDir, wtPath, branch),
		}
	}

	return Result{
		Title:  "Exited worktree",
		Output: fmt.Sprintf("Returned to: %s\nWorktree kept at: %s (branch: %s)", origDir, wtPath, branch),
	}
}

// ─── helpers ──────────────────────────────────────────────────────────────────

func gitRoot(dir string) (string, error) {
	cmd := exec.Command("git", "rev-parse", "--show-toplevel")
	cmd.Dir = dir
	out, err := cmd.Output()
	if err != nil {
		return "", err
	}
	return strings.TrimSpace(string(out)), nil
}

var wtSeq uint64
var wtMu sync.Mutex

func uniqueID() uint64 {
	wtMu.Lock()
	defer wtMu.Unlock()
	wtSeq++
	return wtSeq
}

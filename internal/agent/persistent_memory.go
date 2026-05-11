package agent

import (
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"
)

const (
	// workspaceDirEnv overrides the workspace root used for persistent memory.
	workspaceDirEnv = "OMNILLM_WORKSPACE_DIR"

	// memoryFile is the primary persistent memory file read at session startup.
	memoryFile = "MEMORY.md"

	// agentsFile carries behavior rules that are injected as a high-priority system message.
	agentsFile = "AGENTS.md"

	// memoryLogDir is the directory for daily session logs.
	memoryLogDir = "memory"

	// maxPersistentMemoryBytes caps how much content is injected to avoid blowing the context budget.
	maxPersistentMemoryBytes = 24_000
)

// workspaceDir returns the directory to search for persistent memory files.
// Priority: OMNILLM_WORKSPACE_DIR env → current working directory.
func workspaceDir() string {
	if d := os.Getenv(workspaceDirEnv); d != "" {
		return d
	}
	wd, err := os.Getwd()
	if err != nil {
		return "."
	}
	return wd
}

// PersistentContext holds the content loaded from workspace memory files.
type PersistentContext struct {
	// AgentsRules is the content of AGENTS.md, injected at highest priority.
	AgentsRules string
	// Memory is the content of MEMORY.md.
	Memory string
	// TodayLog is today's daily log content.
	TodayLog string
	// YesterdayLog is yesterday's daily log content (for continuity).
	YesterdayLog string
}

// IsEmpty returns true when no persistent context was found.
func (p PersistentContext) IsEmpty() bool {
	return p.AgentsRules == "" && p.Memory == "" && p.TodayLog == "" && p.YesterdayLog == ""
}

// LoadWorkspaceContext reads AGENTS.md, MEMORY.md, and today's/yesterday's daily
// log from the workspace root. Missing files are silently skipped.
func LoadWorkspaceContext(dir string) PersistentContext {
	var pc PersistentContext
	pc.AgentsRules = readFileCapped(filepath.Join(dir, agentsFile), maxPersistentMemoryBytes)
	pc.Memory = readFileCapped(filepath.Join(dir, memoryFile), maxPersistentMemoryBytes)

	now := time.Now()
	todayPath := dailyLogPath(dir, now)
	yesterdayPath := dailyLogPath(dir, now.AddDate(0, 0, -1))
	pc.TodayLog = readFileCapped(todayPath, maxPersistentMemoryBytes/2)
	pc.YesterdayLog = readFileCapped(yesterdayPath, maxPersistentMemoryBytes/2)
	return pc
}

// AppendDailyLog appends a one-line entry to today's daily log file.
// The file is created if it does not exist.
func AppendDailyLog(dir, entry string) error {
	if entry == "" {
		return nil
	}
	logPath := dailyLogPath(dir, time.Now())
	logDir := filepath.Dir(logPath)
	if err := os.MkdirAll(logDir, 0700); err != nil {
		return fmt.Errorf("daily log mkdir: %w", err)
	}
	f, err := os.OpenFile(logPath, os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0600)
	if err != nil {
		return fmt.Errorf("daily log open: %w", err)
	}
	defer f.Close()
	line := strings.TrimRight(entry, "\n") + "\n"
	_, err = f.WriteString(line)
	return err
}

// dailyLogPath returns the path for the daily log for the given date.
func dailyLogPath(dir string, t time.Time) string {
	filename := t.Format("2006-01-02") + ".md"
	return filepath.Join(dir, memoryLogDir, filename)
}

// readFileCapped reads a file and returns its content capped at maxBytes.
// Returns "" if the file does not exist or cannot be read.
func readFileCapped(path string, maxBytes int) string {
	data, err := os.ReadFile(path)
	if err != nil {
		return ""
	}
	if len(data) > maxBytes {
		data = data[:maxBytes]
		// Back up to the last newline so we don't cut mid-line.
		if idx := strings.LastIndexByte(string(data), '\n'); idx > 0 {
			data = data[:idx]
		}
		return string(data) + "\n… [truncated]"
	}
	return string(data)
}

// injectPersistentContext appends PersistentContext sections to memory as
// system messages in priority order:
//
//	Priority 0: AGENTS.md behavior rules (highest — injected first)
//	Priority 1: MEMORY.md persistent notes
//	Priority 2: Yesterday's daily log (context from prior session)
//	Priority 3: Today's daily log (most recent activity)
func injectPersistentContext(memory Memory, pc PersistentContext) {
	if pc.AgentsRules != "" {
		memory.Append(Message{
			Role: "system",
			Content: []ContentBlock{TextBlock(
				"=== AGENTS.md — Behavior Rules (follow strictly) ===\n" + pc.AgentsRules,
			)},
		})
	}
	if pc.Memory != "" {
		memory.Append(Message{
			Role: "system",
			Content: []ContentBlock{TextBlock(
				"=== MEMORY.md — Persistent Notes ===\n" + pc.Memory,
			)},
		})
	}
	if pc.YesterdayLog != "" {
		memory.Append(Message{
			Role: "system",
			Content: []ContentBlock{TextBlock(
				"=== Yesterday's Session Log ===\n" + pc.YesterdayLog,
			)},
		})
	}
	if pc.TodayLog != "" {
		memory.Append(Message{
			Role: "system",
			Content: []ContentBlock{TextBlock(
				"=== Today's Session Log ===\n" + pc.TodayLog,
			)},
		})
	}
}

package agent

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"
)

func TestLoadWorkspaceContextEmpty(t *testing.T) {
	dir := t.TempDir()
	pc := LoadWorkspaceContext(dir)
	if !pc.IsEmpty() {
		t.Errorf("expected empty context in blank dir, got %+v", pc)
	}
}

func TestLoadWorkspaceContextReadsMemoryAndAgents(t *testing.T) {
	dir := t.TempDir()

	agentsContent := "## Rules\n- Always be helpful\n"
	memoryContent := "# Memory\nUser prefers Go.\n"

	if err := os.WriteFile(filepath.Join(dir, agentsFile), []byte(agentsContent), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(dir, memoryFile), []byte(memoryContent), 0600); err != nil {
		t.Fatal(err)
	}

	pc := LoadWorkspaceContext(dir)

	if pc.AgentsRules != agentsContent {
		t.Errorf("AgentsRules: want %q, got %q", agentsContent, pc.AgentsRules)
	}
	if pc.Memory != memoryContent {
		t.Errorf("Memory: want %q, got %q", memoryContent, pc.Memory)
	}
	if pc.IsEmpty() {
		t.Error("context should not be empty")
	}
}

func TestLoadWorkspaceContextReadsDailyLogs(t *testing.T) {
	dir := t.TempDir()
	logDir := filepath.Join(dir, memoryLogDir)
	if err := os.MkdirAll(logDir, 0700); err != nil {
		t.Fatal(err)
	}

	now := time.Now()
	todayContent := "- solved the bug today\n"
	yesterdayContent := "- set up the project yesterday\n"

	todayPath := filepath.Join(logDir, now.Format("2006-01-02")+".md")
	yesterdayPath := filepath.Join(logDir, now.AddDate(0, 0, -1).Format("2006-01-02")+".md")

	if err := os.WriteFile(todayPath, []byte(todayContent), 0600); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(yesterdayPath, []byte(yesterdayContent), 0600); err != nil {
		t.Fatal(err)
	}

	pc := LoadWorkspaceContext(dir)

	if pc.TodayLog != todayContent {
		t.Errorf("TodayLog: want %q, got %q", todayContent, pc.TodayLog)
	}
	if pc.YesterdayLog != yesterdayContent {
		t.Errorf("YesterdayLog: want %q, got %q", yesterdayContent, pc.YesterdayLog)
	}
}

func TestReadFileCappedTruncates(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "big.txt")
	// Create a file larger than the cap
	content := strings.Repeat("hello world\n", 100)
	if err := os.WriteFile(path, []byte(content), 0600); err != nil {
		t.Fatal(err)
	}

	result := readFileCapped(path, 50)
	// Allow for the truncation marker overhead ("\n… [truncated]" ≈ 15 bytes).
	if len(result) > 80 {
		t.Errorf("result length %d exceeds cap+marker budget", len(result))
	}
	if !strings.Contains(result, "[truncated]") {
		t.Errorf("expected truncation marker, got %q", result)
	}
}

func TestReadFileCappedMissingFile(t *testing.T) {
	result := readFileCapped("/nonexistent/path/file.md", 1000)
	if result != "" {
		t.Errorf("expected empty string for missing file, got %q", result)
	}
}

func TestAppendDailyLog(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(workspaceDirEnv, dir)

	if err := AppendDailyLog(dir, "first entry"); err != nil {
		t.Fatalf("AppendDailyLog: %v", err)
	}
	if err := AppendDailyLog(dir, "second entry"); err != nil {
		t.Fatalf("AppendDailyLog: %v", err)
	}

	logPath := dailyLogPath(dir, time.Now())
	data, err := os.ReadFile(logPath)
	if err != nil {
		t.Fatalf("reading log: %v", err)
	}

	content := string(data)
	if !strings.Contains(content, "first entry") {
		t.Errorf("log missing first entry: %q", content)
	}
	if !strings.Contains(content, "second entry") {
		t.Errorf("log missing second entry: %q", content)
	}
}

func TestInjectPersistentContextAppendsMessages(t *testing.T) {
	memory := NewBufferMemory(32)
	memory.Append(Message{Role: "system", Content: []ContentBlock{TextBlock("base system")}})

	pc := PersistentContext{
		AgentsRules:  "rule1",
		Memory:       "note1",
		YesterdayLog: "yesterday log",
		TodayLog:     "today log",
	}
	injectPersistentContext(memory, pc)

	msgs := memory.Messages()
	if len(msgs) != 5 { // base + 4 injected
		t.Errorf("expected 5 messages, got %d", len(msgs))
	}

	combined := ""
	for _, m := range msgs {
		combined += extractTextContent(m.Content) + "\n"
	}
	for _, want := range []string{"rule1", "note1", "yesterday log", "today log"} {
		if !strings.Contains(combined, want) {
			t.Errorf("missing %q in injected messages", want)
		}
	}
}

func TestInjectPersistentContextSkipsEmpty(t *testing.T) {
	memory := NewBufferMemory(32)
	pc := PersistentContext{Memory: "only memory"} // others empty
	injectPersistentContext(memory, pc)
	msgs := memory.Messages()
	if len(msgs) != 1 {
		t.Errorf("expected 1 message (only non-empty field), got %d", len(msgs))
	}
}

func TestWorkspaceDirEnvOverride(t *testing.T) {
	dir := t.TempDir()
	t.Setenv(workspaceDirEnv, dir)
	got := workspaceDir()
	if got != dir {
		t.Errorf("workspaceDir: want %q, got %q", dir, got)
	}
}

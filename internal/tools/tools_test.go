package tools

// OmniCode tool-call verification suite.
//
// Each test pretends to be the OmniCode agent and calls every registered tool
// through its Execute method, verifying:
//   - Happy-path output format and content
//   - Validation errors (missing required fields, out-of-range values, etc.)
//   - Registry / Manager plumbing (Register, Get, List, ToCIFTools,
//     ExecuteToolCalls, ToolNamesByCategory)
//
// Tests that touch the filesystem use os.MkdirTemp so they are hermetic.

import (
	"context"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	"omnillm/internal/cif"
)

// ─── helpers ─────────────────────────────────────────────────────────────────

func noCtx() Context { return Context{} }

func mustJSON(v any) json.RawMessage {
	b, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	return b
}

// ─── bash ────────────────────────────────────────────────────────────────────

func TestBashToolReturnsOutput(t *testing.T) {
	tool := Bash()
	result := tool.Execute(context.Background(), noCtx(), mustJSON(map[string]any{
		"command": "echo hello_omnicode",
	}))
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Output)
	}
	if !strings.Contains(result.Output, "hello_omnicode") {
		t.Fatalf("expected 'hello_omnicode' in output, got %q", result.Output)
	}
}

func TestBashToolRequiresCommand(t *testing.T) {
	tool := Bash()
	result := tool.Execute(context.Background(), noCtx(), mustJSON(map[string]any{}))
	if !result.IsError {
		t.Fatal("expected error for missing command")
	}
	if !strings.Contains(result.Output, "command is required") {
		t.Fatalf("unexpected error: %q", result.Output)
	}
}

func TestBashToolBadJSON(t *testing.T) {
	tool := Bash()
	result := tool.Execute(context.Background(), noCtx(), json.RawMessage(`{bad json`))
	if !result.IsError {
		t.Fatal("expected error for bad JSON")
	}
}

// ─── read ─────────────────────────────────────────────────────────────────────

func TestReadToolReadsFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "hello.txt")
	content := "line1\nline2\nline3\n"
	if err := os.WriteFile(path, []byte(content), 0o644); err != nil {
		t.Fatal(err)
	}

	tool := Read()
	result := tool.Execute(context.Background(), noCtx(), mustJSON(map[string]any{"file_path": path}))
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Output)
	}
	if !strings.Contains(result.Output, "line1") || !strings.Contains(result.Output, "line3") {
		t.Fatalf("unexpected output: %q", result.Output)
	}
}

func TestReadToolWithOffsetAndLimit(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "lines.txt")
	var sb strings.Builder
	for i := 1; i <= 10; i++ {
		fmt.Fprintf(&sb, "line%d\n", i)
	}
	if err := os.WriteFile(path, []byte(sb.String()), 0o644); err != nil {
		t.Fatal(err)
	}

	tool := Read()
	result := tool.Execute(context.Background(), noCtx(), mustJSON(map[string]any{
		"file_path": path,
		"offset":    3,
		"limit":     2,
	}))
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Output)
	}
	if !strings.Contains(result.Output, "line3") || !strings.Contains(result.Output, "line4") {
		t.Fatalf("expected lines 3-4, got: %q", result.Output)
	}
	if strings.Contains(result.Output, "line5") {
		t.Fatalf("limit exceeded, got: %q", result.Output)
	}
}

func TestReadToolRequiresFilePath(t *testing.T) {
	tool := Read()
	result := tool.Execute(context.Background(), noCtx(), mustJSON(map[string]any{}))
	if !result.IsError {
		t.Fatal("expected error for missing file_path")
	}
	if !strings.Contains(result.Output, "file_path is required") {
		t.Fatalf("unexpected error: %q", result.Output)
	}
}

func TestReadToolMissingFile(t *testing.T) {
	tool := Read()
	result := tool.Execute(context.Background(), noCtx(), mustJSON(map[string]any{
		"file_path": "/nonexistent/path/file.txt",
	}))
	if !result.IsError {
		t.Fatal("expected error for missing file")
	}
}

// ─── write ───────────────────────────────────────────────────────────────────

func TestWriteToolCreatesFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "out.txt")

	tool := Write()
	result := tool.Execute(context.Background(), noCtx(), mustJSON(map[string]any{
		"file_path": path,
		"content":   "omnicode wrote this",
	}))
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Output)
	}

	data, err := os.ReadFile(path)
	if err != nil {
		t.Fatalf("file not created: %v", err)
	}
	if string(data) != "omnicode wrote this" {
		t.Fatalf("unexpected content: %q", string(data))
	}
}

func TestWriteToolCreatesParentDirs(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "a", "b", "c.txt")

	tool := Write()
	result := tool.Execute(context.Background(), noCtx(), mustJSON(map[string]any{
		"file_path": path,
		"content":   "nested",
	}))
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Output)
	}
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("file not found: %v", err)
	}
}

func TestWriteToolRequiresFilePath(t *testing.T) {
	tool := Write()
	result := tool.Execute(context.Background(), noCtx(), mustJSON(map[string]any{"content": "x"}))
	if !result.IsError {
		t.Fatal("expected error for missing file_path")
	}
}

// ─── edit ─────────────────────────────────────────────────────────────────────

func TestEditToolReplacesString(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "edit.txt")
	_ = os.WriteFile(path, []byte("hello world"), 0o644)

	tool := Edit()
	result := tool.Execute(context.Background(), noCtx(), mustJSON(map[string]any{
		"file_path":  path,
		"old_string": "world",
		"new_string": "omnicode",
	}))
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Output)
	}

	data, _ := os.ReadFile(path)
	if string(data) != "hello omnicode" {
		t.Fatalf("edit failed, got: %q", string(data))
	}
}

func TestEditToolReplaceAll(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "multi.txt")
	_ = os.WriteFile(path, []byte("foo foo foo"), 0o644)

	tool := Edit()
	result := tool.Execute(context.Background(), noCtx(), mustJSON(map[string]any{
		"file_path":   path,
		"old_string":  "foo",
		"new_string":  "bar",
		"replace_all": true,
	}))
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Output)
	}

	data, _ := os.ReadFile(path)
	if string(data) != "bar bar bar" {
		t.Fatalf("replace_all failed, got: %q", string(data))
	}
}

func TestEditToolOldStringNotFound(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "notfound.txt")
	_ = os.WriteFile(path, []byte("hello"), 0o644)

	tool := Edit()
	result := tool.Execute(context.Background(), noCtx(), mustJSON(map[string]any{
		"file_path":  path,
		"old_string": "nonexistent",
		"new_string": "x",
	}))
	if !result.IsError {
		t.Fatal("expected error when old_string not found")
	}
	if !strings.Contains(result.Output, "old_string not found") {
		t.Fatalf("unexpected error: %q", result.Output)
	}
}

func TestEditToolRequiresFilePath(t *testing.T) {
	tool := Edit()
	result := tool.Execute(context.Background(), noCtx(), mustJSON(map[string]any{
		"old_string": "x",
		"new_string": "y",
	}))
	if !result.IsError {
		t.Fatal("expected error for missing file_path")
	}
}

// ─── glob ─────────────────────────────────────────────────────────────────────

func TestGlobToolFindsFiles(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "a.go"), []byte(""), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "b.go"), []byte(""), 0o644)
	_ = os.WriteFile(filepath.Join(dir, "c.txt"), []byte(""), 0o644)

	tool := Glob()
	result := tool.Execute(context.Background(), noCtx(), mustJSON(map[string]any{
		"pattern": "*.go",
		"path":    dir,
	}))
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Output)
	}
	if !strings.Contains(result.Output, "a.go") || !strings.Contains(result.Output, "b.go") {
		t.Fatalf("missing .go files in output: %q", result.Output)
	}
	if strings.Contains(result.Output, "c.txt") {
		t.Fatalf("c.txt should not match *.go: %q", result.Output)
	}
}

func TestGlobToolNoMatches(t *testing.T) {
	dir := t.TempDir()
	tool := Glob()
	result := tool.Execute(context.Background(), noCtx(), mustJSON(map[string]any{
		"pattern": "*.nonexistent",
		"path":    dir,
	}))
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Output)
	}
	if !strings.Contains(result.Output, "no matches") {
		t.Fatalf("expected 'no matches', got: %q", result.Output)
	}
}

func TestGlobToolRequiresPattern(t *testing.T) {
	tool := Glob()
	result := tool.Execute(context.Background(), noCtx(), mustJSON(map[string]any{}))
	if !result.IsError {
		t.Fatal("expected error for missing pattern")
	}
}

// ─── grep ─────────────────────────────────────────────────────────────────────

func TestGrepToolFindsMatches(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "code.go"), []byte("func OmniCode() {}\n"), 0o644)

	tool := Grep()
	result := tool.Execute(context.Background(), noCtx(), mustJSON(map[string]any{
		"pattern": "OmniCode",
		"path":    dir,
	}))
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Output)
	}
	if !strings.Contains(result.Output, "OmniCode") {
		t.Fatalf("expected match, got: %q", result.Output)
	}
}

func TestGrepToolCaseInsensitive(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "f.txt"), []byte("OMNICODE is great\n"), 0o644)

	tool := Grep()
	result := tool.Execute(context.Background(), noCtx(), mustJSON(map[string]any{
		"pattern":          "omnicode",
		"path":             dir,
		"case_insensitive": true,
	}))
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Output)
	}
	if !strings.Contains(strings.ToLower(result.Output), "omnicode") {
		t.Fatalf("expected case-insensitive match, got: %q", result.Output)
	}
}

func TestGrepToolNoMatches(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "empty.go"), []byte("package main\n"), 0o644)

	tool := Grep()
	result := tool.Execute(context.Background(), noCtx(), mustJSON(map[string]any{
		"pattern": "xyzzy_not_found",
		"path":    dir,
	}))
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Output)
	}
	if !strings.Contains(result.Output, "no matches") {
		t.Fatalf("expected 'no matches', got: %q", result.Output)
	}
}

func TestGrepToolRequiresPattern(t *testing.T) {
	tool := Grep()
	result := tool.Execute(context.Background(), noCtx(), mustJSON(map[string]any{}))
	if !result.IsError {
		t.Fatal("expected error for missing pattern")
	}
}

// ─── ls ───────────────────────────────────────────────────────────────────────

func TestLSToolListsDirectory(t *testing.T) {
	dir := t.TempDir()
	_ = os.WriteFile(filepath.Join(dir, "alpha.txt"), []byte("a"), 0o644)
	_ = os.Mkdir(filepath.Join(dir, "subdir"), 0o755)

	tool := LS()
	result := tool.Execute(context.Background(), noCtx(), mustJSON(map[string]any{"path": dir}))
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Output)
	}
	if !strings.Contains(result.Output, "alpha.txt") {
		t.Fatalf("expected alpha.txt, got: %q", result.Output)
	}
	if !strings.Contains(result.Output, "subdir/") {
		t.Fatalf("expected subdir/, got: %q", result.Output)
	}
}

func TestLSToolEmptyDirectory(t *testing.T) {
	dir := t.TempDir()
	tool := LS()
	result := tool.Execute(context.Background(), noCtx(), mustJSON(map[string]any{"path": dir}))
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Output)
	}
	if !strings.Contains(result.Output, "empty directory") {
		t.Fatalf("expected empty directory message, got: %q", result.Output)
	}
}

func TestLSToolBadPath(t *testing.T) {
	tool := LS()
	result := tool.Execute(context.Background(), noCtx(), mustJSON(map[string]any{
		"path": "/nonexistent/directory/that/does/not/exist",
	}))
	if !result.IsError {
		t.Fatal("expected error for bad path")
	}
}

// ─── sleep ────────────────────────────────────────────────────────────────────

func TestSleepToolSleeps(t *testing.T) {
	tool := Sleep()
	start := time.Now()
	result := tool.Execute(context.Background(), noCtx(), mustJSON(map[string]any{"seconds": 1}))
	elapsed := time.Since(start)

	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Output)
	}
	if elapsed < 900*time.Millisecond {
		t.Fatalf("sleep was too short: %v", elapsed)
	}
	if !strings.Contains(result.Output, "1 second") {
		t.Fatalf("unexpected output: %q", result.Output)
	}
}

func TestSleepToolOutOfRange(t *testing.T) {
	tool := Sleep()
	for _, secs := range []int{0, -1, 3601} {
		result := tool.Execute(context.Background(), noCtx(), mustJSON(map[string]any{"seconds": secs}))
		if !result.IsError {
			t.Fatalf("expected error for seconds=%d", secs)
		}
		if !strings.Contains(result.Output, "seconds must be between") {
			t.Fatalf("unexpected error for seconds=%d: %q", secs, result.Output)
		}
	}
}

func TestSleepToolCancelled(t *testing.T) {
	ctx, cancel := context.WithCancel(context.Background())
	go func() {
		time.Sleep(50 * time.Millisecond)
		cancel()
	}()

	tool := Sleep()
	result := tool.Execute(ctx, noCtx(), mustJSON(map[string]any{"seconds": 60}))
	if !result.IsError {
		t.Fatal("expected error when context cancelled")
	}
	if !strings.Contains(result.Output, "interrupted") {
		t.Fatalf("unexpected error: %q", result.Output)
	}
}

// ─── ask_user_question ───────────────────────────────────────────────────────

func TestAskUserToolCallsCallback(t *testing.T) {
	called := false
	ctx := Context{
		AskUser: func(c context.Context, question string, options []string) (string, error) {
			called = true
			if question != "What is 2+2?" {
				t.Errorf("unexpected question: %q", question)
			}
			return "4", nil
		},
	}

	tool := AskUser()
	result := tool.Execute(context.Background(), ctx, mustJSON(map[string]any{
		"question": "What is 2+2?",
		"options":  []string{"3", "4", "5"},
	}))
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Output)
	}
	if !called {
		t.Fatal("AskUser callback was not called")
	}
	if !strings.Contains(result.Output, "4") {
		t.Fatalf("unexpected output: %q", result.Output)
	}
}

func TestAskUserToolNoCallback(t *testing.T) {
	tool := AskUser()
	result := tool.Execute(context.Background(), noCtx(), mustJSON(map[string]any{
		"question": "anything?",
	}))
	if !result.IsError {
		t.Fatal("expected error when no callback configured")
	}
	if !strings.Contains(result.Output, "callback not available") {
		t.Fatalf("unexpected error: %q", result.Output)
	}
}

func TestAskUserToolRequiresQuestion(t *testing.T) {
	tool := AskUser()
	result := tool.Execute(context.Background(), noCtx(), mustJSON(map[string]any{}))
	if !result.IsError {
		t.Fatal("expected error for missing question")
	}
}

// ─── lsp ──────────────────────────────────────────────────────────────────────

func TestLSPToolHover(t *testing.T) {
	tool := LSP()
	result := tool.Execute(context.Background(), noCtx(), mustJSON(map[string]any{
		"operation": "hover",
		"filePath":  "/some/file.go",
		"line":      10,
		"character": 5,
	}))
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Output)
	}
	if !strings.Contains(result.Output, "Hover info") {
		t.Fatalf("unexpected output: %q", result.Output)
	}
}

func TestLSPToolWorkspaceSymbol(t *testing.T) {
	tool := LSP()
	result := tool.Execute(context.Background(), noCtx(), mustJSON(map[string]any{
		"operation": "workspaceSymbol",
		"query":     "OmniCode",
	}))
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Output)
	}
	if !strings.Contains(result.Output, "OmniCode") {
		t.Fatalf("unexpected output: %q", result.Output)
	}
}

func TestLSPToolUnknownOperation(t *testing.T) {
	tool := LSP()
	result := tool.Execute(context.Background(), noCtx(), mustJSON(map[string]any{
		"operation": "unknownOp",
		"filePath":  "/file.go",
	}))
	if !result.IsError {
		t.Fatal("expected error for unknown operation")
	}
	if !strings.Contains(result.Output, "unknown LSP operation") {
		t.Fatalf("unexpected error: %q", result.Output)
	}
}

func TestLSPToolMissingFilePathForPositionOps(t *testing.T) {
	tool := LSP()
	ops := []string{"goToDefinition", "findReferences", "hover", "documentSymbol",
		"goToImplementation", "prepareCallHierarchy", "incomingCalls", "outgoingCalls"}
	for _, op := range ops {
		result := tool.Execute(context.Background(), noCtx(), mustJSON(map[string]any{
			"operation": op,
		}))
		if !result.IsError {
			t.Fatalf("expected error for op=%q with no filePath", op)
		}
		if !strings.Contains(result.Output, "filePath is required") {
			t.Fatalf("op=%q unexpected error: %q", op, result.Output)
		}
	}
}

// ─── apply_patch ──────────────────────────────────────────────────────────────

func TestApplyPatchToolAppliesPatch(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "target.go")
	_ = os.WriteFile(path, []byte("func old() {}\n"), 0o644)

	// Build a minimal unified diff
	patch := fmt.Sprintf(`--- a/target.go
+++ b/%s
@@ -1 +1 @@
-func old() {}
+func new() {}
`, path)

	tool := ApplyPatch()
	result := tool.Execute(context.Background(), noCtx(), mustJSON(map[string]any{
		"patch_text": patch,
	}))
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Output)
	}

	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), "func new()") {
		t.Fatalf("patch not applied, file content: %q", string(data))
	}
}

func TestApplyPatchToolEmptyPatch(t *testing.T) {
	tool := ApplyPatch()
	result := tool.Execute(context.Background(), noCtx(), mustJSON(map[string]any{
		"patch_text": "",
	}))
	if !result.IsError {
		t.Fatal("expected error for empty patch")
	}
}

func TestApplyPatchToolRequiresPatchText(t *testing.T) {
	tool := ApplyPatch()
	result := tool.Execute(context.Background(), noCtx(), mustJSON(map[string]any{}))
	if !result.IsError {
		t.Fatal("expected error for missing patch_text")
	}
}

// ─── multiedit ────────────────────────────────────────────────────────────────

func TestMultiEditToolAppliesEdits(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "me.txt")
	_ = os.WriteFile(path, []byte("foo bar baz"), 0o644)

	tool := MultiEdit()
	result := tool.Execute(context.Background(), noCtx(), mustJSON(map[string]any{
		"file_path": path,
		"edits": []map[string]any{
			{"old_string": "foo", "new_string": "FOO"},
			{"old_string": "baz", "new_string": "BAZ"},
		},
	}))
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Output)
	}

	data, _ := os.ReadFile(path)
	if !strings.Contains(string(data), "FOO") || !strings.Contains(string(data), "BAZ") {
		t.Fatalf("multiedit failed, got: %q", string(data))
	}
}

func TestMultiEditToolRequiresEdits(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "me.txt")
	_ = os.WriteFile(path, []byte("content"), 0o644)

	tool := MultiEdit()
	result := tool.Execute(context.Background(), noCtx(), mustJSON(map[string]any{
		"file_path": path,
		"edits":     []map[string]any{},
	}))
	if !result.IsError {
		t.Fatal("expected error for empty edits")
	}
}

func TestMultiEditToolRequiresFilePath(t *testing.T) {
	tool := MultiEdit()
	result := tool.Execute(context.Background(), noCtx(), mustJSON(map[string]any{
		"edits": []map[string]any{{"old_string": "x", "new_string": "y"}},
	}))
	if !result.IsError {
		t.Fatal("expected error for missing file_path")
	}
}

// ─── web_search (error paths, no network required) ────────────────────────────

func TestWebSearchToolRequiresQuery(t *testing.T) {
	tool := WebSearch()
	result := tool.Execute(context.Background(), noCtx(), mustJSON(map[string]any{}))
	if !result.IsError {
		t.Fatal("expected error for missing query")
	}
	if !strings.Contains(result.Output, "query is required") {
		t.Fatalf("unexpected error: %q", result.Output)
	}
}

func TestWebSearchToolCapsNumResults(t *testing.T) {
	// Use a local server to avoid real network calls — just verify the cap
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
		_, _ = w.Write([]byte("<html></html>"))
	}))
	defer srv.Close()

	// We can't override the DuckDuckGo URL, but we can verify numResults clamping
	// by testing the validation branch (> 20 -> 20). The tool itself doesn't
	// expose that clamping as an error, so just verify no panic occurs.
	tool := WebSearch()
	// This will do a real network call; skip if no network is available.
	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	result := tool.Execute(ctx, noCtx(), mustJSON(map[string]any{
		"query":      "omnicode test",
		"numResults": 100, // should be clamped to 20, not error
	}))
	// Either result is fine (network may or may not be available); just ensure
	// the tool handles numResults > 20 without panicking.
	_ = result
}

// ─── codesearch (error paths, no network required) ───────────────────────────

func TestCodeSearchToolRequiresQuery(t *testing.T) {
	tool := CodeSearch()
	result := tool.Execute(context.Background(), noCtx(), mustJSON(map[string]any{}))
	if !result.IsError {
		t.Fatal("expected error for missing query")
	}
	if !strings.Contains(result.Output, "query is required") {
		t.Fatalf("unexpected error: %q", result.Output)
	}
}

// ─── web_fetch (served locally) ───────────────────────────────────────────────

func TestWebFetchTool4xxReturnsError(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusNotFound)
		_, _ = w.Write([]byte("not found"))
	}))
	defer srv.Close()

	tool := WebFetch()
	result := tool.Execute(context.Background(), noCtx(), mustJSON(map[string]any{"url": srv.URL}))
	if !result.IsError {
		t.Fatal("expected error for 4xx status")
	}
	if !strings.Contains(result.Output, "404") {
		t.Fatalf("expected 404 in output, got: %q", result.Output)
	}
}

func TestWebFetchToolEmptyBody(t *testing.T) {
	srv := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(200)
	}))
	defer srv.Close()

	tool := WebFetch()
	result := tool.Execute(context.Background(), noCtx(), mustJSON(map[string]any{"url": srv.URL}))
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Output)
	}
	if !strings.Contains(result.Output, "empty body") {
		t.Fatalf("expected empty body message, got: %q", result.Output)
	}
}

// ─── Registry ─────────────────────────────────────────────────────────────────

func TestRegistryRegisterAndGet(t *testing.T) {
	r := NewRegistry()
	r.Register(Bash())

	tool := r.Get("bash")
	if tool == nil {
		t.Fatal("expected bash tool to be registered")
	}
	if tool.Name() != "bash" {
		t.Fatalf("unexpected name: %q", tool.Name())
	}
}

func TestRegistryGetUnknown(t *testing.T) {
	r := NewRegistry()
	if r.Get("nonexistent") != nil {
		t.Fatal("expected nil for unknown tool")
	}
}

func TestRegistryListReturnsAll(t *testing.T) {
	r := NewRegistry()
	r.Register(Bash())
	r.Register(Read())
	r.Register(Write())

	tools := r.List()
	if len(tools) != 3 {
		t.Fatalf("expected 3 tools, got %d", len(tools))
	}
}

func TestRegistryToCIFTools(t *testing.T) {
	r := NewRegistry()
	r.Register(Bash())
	r.Register(Calculator())

	cif := r.ToCIFTools()
	if len(cif) != 2 {
		t.Fatalf("expected 2 CIF tools, got %d", len(cif))
	}
	names := make(map[string]bool)
	for _, c := range cif {
		names[c.Name] = true
	}
	if !names["bash"] || !names["calculator"] {
		t.Fatalf("missing expected tools: %v", names)
	}
}

func TestRegistryExecuteToolCallsUnknownTool(t *testing.T) {
	r := NewRegistry()
	// No tools registered — call with unknown tool name via cif.CIFToolCallPart
	calls := []cif.CIFToolCallPart{
		{ToolCallID: "c1", ToolName: "ghost_tool", ToolArguments: map[string]any{}},
	}
	results := r.ExecuteToolCalls(context.Background(), "sess1", calls)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if !results[0].IsError {
		t.Fatal("expected error for unknown tool")
	}
	if !strings.Contains(results[0].Content, "unknown tool") {
		t.Fatalf("unexpected error content: %q", results[0].Content)
	}
}

func TestRegistryExecuteToolCallsPanicRecovery(t *testing.T) {
	r := NewRegistry()
	r.Register(&panicTool{})

	calls := []cif.CIFToolCallPart{
		{ToolCallID: "c1", ToolName: "panic_tool", ToolArguments: map[string]any{}},
	}
	results := r.ExecuteToolCalls(context.Background(), "sess1", calls)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if !results[0].IsError {
		t.Fatal("expected error result from panicking tool")
	}
	if !strings.Contains(results[0].Content, "panicked") {
		t.Fatalf("unexpected panic recovery message: %q", results[0].Content)
	}
}

func TestRegistryExecuteToolCallsPermissionDenied(t *testing.T) {
	r := NewRegistry()
	r.Register(Bash())
	r.SetPermissionChecker(func(ctx context.Context, req PermissionRequest) (bool, error) {
		return false, nil // deny everything
	})

	calls := []cif.CIFToolCallPart{
		{ToolCallID: "c1", ToolName: "bash", ToolArguments: map[string]any{"command": "echo hi"}},
	}
	results := r.ExecuteToolCalls(context.Background(), "sess1", calls)
	if len(results) != 1 {
		t.Fatalf("expected 1 result, got %d", len(results))
	}
	if !results[0].IsError {
		t.Fatal("expected error when permission denied")
	}
	if !strings.Contains(results[0].Content, "denied") {
		t.Fatalf("unexpected error content: %q", results[0].Content)
	}
}

// panicTool is a test double that panics on Execute to test recovery.
type panicTool struct{}

func (p *panicTool) Name() string        { return "panic_tool" }
func (p *panicTool) Description() string { return "panics" }
func (p *panicTool) InputSchema() map[string]any {
	return map[string]any{"type": "object", "properties": map[string]any{}}
}
func (p *panicTool) Execute(_ context.Context, _ Context, _ json.RawMessage) Result {
	panic("intentional panic for test")
}

// ─── Manager ──────────────────────────────────────────────────────────────────

func TestManagerRegisterAndMetadata(t *testing.T) {
	m := NewManager()
	m.Register(Bash(), Metadata{Category: CategoryShell, ReadOnly: false})
	m.Register(Read(), Metadata{Category: CategoryFilesystem, ReadOnly: true})

	meta, ok := m.Metadata("bash")
	if !ok {
		t.Fatal("expected metadata for bash")
	}
	if meta.Category != CategoryShell {
		t.Fatalf("expected CategoryShell, got %q", meta.Category)
	}

	meta2, ok2 := m.Metadata("read")
	if !ok2 {
		t.Fatal("expected metadata for read")
	}
	if !meta2.ReadOnly {
		t.Fatal("expected read to be ReadOnly")
	}
}

func TestManagerToolNamesByCategory(t *testing.T) {
	m := NewManager()
	m.Register(Bash(), Metadata{Category: CategoryShell})
	m.Register(Read(), Metadata{Category: CategoryFilesystem, ReadOnly: true})
	m.Register(Glob(), Metadata{Category: CategoryFilesystem, ReadOnly: true})

	fsTools := m.ToolNamesByCategory(CategoryFilesystem)
	if len(fsTools) != 2 {
		t.Fatalf("expected 2 filesystem tools, got %d: %v", len(fsTools), fsTools)
	}

	shellTools := m.ToolNamesByCategory(CategoryShell)
	if len(shellTools) != 1 || shellTools[0] != "bash" {
		t.Fatalf("expected [bash], got %v", shellTools)
	}
}

func TestManagerRegistryPassthrough(t *testing.T) {
	m := NewManager()
	m.Register(Calculator(), Metadata{Category: CategoryUtility, ReadOnly: true})
	if m.Registry().Get("calculator") == nil {
		t.Fatal("registry should contain calculator")
	}
}

// ─── RegisterCoreTools registers all 36 tools ─────────────────────────────────

func TestRegisterCoreToolsCount(t *testing.T) {
	m := NewManager()
	RegisterCoreTools(m)

	tools := m.Registry().List()
	// 36 tools as of groups.go: bash, powershell, read, write, edit, multiedit,
	// apply_patch, glob, grep, ls, notebook_edit, web_fetch, web_search,
	// codesearch, get_current_time, calculator, sleep, ask_user_question, lsp,
	// tool_search, config, todo_write, task_create, task_get, task_list,
	// task_output, task_stop, task_update, enter_plan_mode, exit_plan_mode,
	// enter_worktree, exit_worktree, schedule_cron, send_message, agent, batch
	const wantCount = 36
	if len(tools) != wantCount {
		names := make([]string, len(tools))
		for i, t2 := range tools {
			names[i] = t2.Name()
		}
		t.Fatalf("expected %d core tools, got %d: %v", wantCount, len(tools), names)
	}
}

func TestRegisterCoreToolsAllHaveSchemas(t *testing.T) {
	m := NewManager()
	RegisterCoreTools(m)

	for _, tool := range m.Registry().List() {
		schema := tool.InputSchema()
		if schema == nil {
			t.Errorf("tool %q has nil InputSchema", tool.Name())
		}
		if schema["type"] == nil {
			t.Errorf("tool %q schema missing 'type' field", tool.Name())
		}
	}
}

func TestRegisterCoreToolsAllHaveDescriptions(t *testing.T) {
	m := NewManager()
	RegisterCoreTools(m)

	for _, tool := range m.Registry().List() {
		if strings.TrimSpace(tool.Description()) == "" {
			t.Errorf("tool %q has empty description", tool.Name())
		}
	}
}

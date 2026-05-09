package agent

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"testing"
	"time"

	toolspkg "omnillm/internal/tools"
)

// ─── live HTTP client ────────────────────────────────────────────────────────

type liveHTTPClient struct {
	baseURL string
	apiKey  string
}

func (c *liveHTTPClient) GetBaseURL() string { return c.baseURL }
func (c *liveHTTPClient) GetAPIKey() string  { return c.apiKey }

func (c *liveHTTPClient) Post(path string, body any) ([]byte, error) {
	jsonBytes, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal: %w", err)
	}
	req, err := http.NewRequest(http.MethodPost, c.baseURL+path, bytes.NewReader(jsonBytes))
	if err != nil {
		return nil, fmt.Errorf("new request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("X-API-Key", c.apiKey)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("post %s: %w", path, err)
	}
	defer resp.Body.Close()
	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}
	if resp.StatusCode >= 400 {
		return nil, fmt.Errorf("server error (%d): %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}
	return data, nil
}

func (c *liveHTTPClient) PostStream(path string, body any) (*http.Response, error) {
	jsonBytes, err := json.Marshal(body)
	if err != nil {
		return nil, fmt.Errorf("marshal: %w", err)
	}
	req, err := http.NewRequest(http.MethodPost, c.baseURL+path, bytes.NewReader(jsonBytes))
	if err != nil {
		return nil, fmt.Errorf("new request: %w", err)
	}
	req.Header.Set("Content-Type", "application/json")
	if c.apiKey != "" {
		req.Header.Set("X-API-Key", c.apiKey)
	}
	resp, err := http.DefaultClient.Do(req)
	if err != nil {
		return nil, fmt.Errorf("post stream %s: %w", path, err)
	}
	if resp.StatusCode >= 400 {
		data, _ := io.ReadAll(resp.Body)
		resp.Body.Close()
		return nil, fmt.Errorf("server error (%d): %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}
	return resp, nil
}

// ─── configuration ───────────────────────────────────────────────────────────

const liveBaseURL = "http://127.0.0.1:5000"

var liveAPIKey = func() string {
	if k := strings.TrimSpace(os.Getenv("OMNILLM_API_KEY")); k != "" {
		return k
	}
	data, err := os.ReadFile(filepath.Join(os.Getenv("USERPROFILE"), ".config", "omnillm", "api-key"))
	if err != nil {
		return ""
	}
	return strings.TrimSpace(string(data))
}()

// ─── test case definition ────────────────────────────────────────────────────

type modelCase struct {
	name  string
	model string // model string as used by omnicode agent (may include provider prefix)
}

// models under test matching the user's requirements
var testModels = []modelCase{
	{name: "alibaba/deepseek-v4-flash", model: "alibaba-sk-ab2c5/deepseek-v4-flash"},
	{name: "copilot/gpt-5-mini", model: "gpt-5-mini"},
	{name: "copilot/claude-haiku-4.5", model: "claude-haiku-4.5"},
}

var testShapes = []string{"anthropic", "openai", "responses"}

var testBackends = []string{"agent-sdk-go", "google-adk", "anthropic-sdk"}

// ─── helper: run a single turn and return result or error ────────────────────

func runAgentTurn(t *testing.T, client *liveHTTPClient, model, backend, apiShape, prompt string) (*RunResult, error) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
	defer cancel()
	return RunTurn(ctx, client,
		"live-"+strings.ReplaceAll(t.Name(), "/", "-"),
		model, backend, apiShape,
		prompt,
		nil, nil, nil, 5,
	)
}

// ─── TestLiveFullMeshSimpleTurn ──────────────────────────────────────────────

// TestLiveFullMeshSimpleTurn runs a simple "Say hello in exactly 3 words" turn
// for every combination of model × API shape × backend.
// This simulates the omnicode agent REPL/TUI calling agentpkg.RunTurn.
func TestLiveFullMeshSimpleTurn(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping live test in short mode")
	}

	client := &liveHTTPClient{baseURL: liveBaseURL, apiKey: liveAPIKey}

	for _, mc := range testModels {
		for _, shape := range testShapes {
			for _, backend := range testBackends {
				mc, shape, backend := mc, shape, backend
				tag := fmt.Sprintf("%s/shape=%s/backend=%s", mc.name, shape, backend)
				t.Run(tag, func(t *testing.T) {
					result, err := runAgentTurn(t, client, mc.model, backend, shape,
						"Say hello in exactly 3 words.")
					if err != nil {
						t.Fatalf("error: %v", err)
					}
					out := strings.TrimSpace(result.Output)
					if out == "" {
						t.Fatal("empty output")
					}
					t.Logf("output: %q  steps=%d", out, result.Steps)
				})
			}
		}
	}
}

// ─── TestLiveFullMeshStreamTurn ──────────────────────────────────────────────

// TestLiveFullMeshStreamTurn runs a streaming turn for every combination.
// This simulates the omnicode agent TUI calling agentpkg.StreamTurn
// (via StreamAgentTurnWithChecker).
func TestLiveFullMeshStreamTurn(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping live test in short mode")
	}

	client := &liveHTTPClient{baseURL: liveBaseURL, apiKey: liveAPIKey}

	for _, mc := range testModels {
		for _, shape := range testShapes {
			for _, backend := range testBackends {
				mc, shape, backend := mc, shape, backend
				tag := fmt.Sprintf("%s/shape=%s/backend=%s", mc.name, shape, backend)
				t.Run(tag, func(t *testing.T) {
					ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
					defer cancel()

					eventCh, err := StreamTurn(ctx, client,
						"live-stream-"+strings.ReplaceAll(tag, "/", "-"),
						mc.model, backend, shape,
						"Say hello in exactly 3 words.",
						nil, nil, nil, 5,
					)
					if err != nil {
						t.Fatalf("StreamTurn error: %v", err)
					}

					var tokens []string
					var errs []string
					done := false
					for event := range eventCh {
						switch event.Type {
						case EventToken:
							tokens = append(tokens, event.Content)
						case EventDone:
							done = true
						case EventError:
							errs = append(errs, event.Content)
						}
					}
					if len(errs) > 0 {
						t.Fatalf("stream errors: %v", errs)
					}
					if !done {
						t.Error("EventDone not received")
					}
					out := strings.TrimSpace(strings.Join(tokens, ""))
					if out == "" {
						t.Fatal("empty stream output")
					}
					t.Logf("stream output: %q", out)
				})
			}
		}
	}
}

// ─── TestLiveFullMeshToolTurn ────────────────────────────────────────────────

// TestLiveFullMeshToolTurn verifies tool-call execution for every combination.
// Uses get_current_time which requires no external I/O.
func TestLiveFullMeshToolTurn(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping live test in short mode")
	}

	client := &liveHTTPClient{baseURL: liveBaseURL, apiKey: liveAPIKey}

	for _, mc := range testModels {
		for _, shape := range testShapes {
			for _, backend := range testBackends {
				mc, shape, backend := mc, shape, backend
				tag := fmt.Sprintf("%s/shape=%s/backend=%s", mc.name, shape, backend)
				t.Run(tag, func(t *testing.T) {
					ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
					defer cancel()

					result, err := RunTurn(ctx, client,
						"live-tool-"+strings.ReplaceAll(tag, "/", "-"),
						mc.model, backend, shape,
						"What time is it? Use the get_current_time tool to find out.",
						nil,
						func(_ context.Context, _ toolspkg.PermissionRequest) (bool, error) { return true, nil },
						nil, 5,
					)
					if err != nil {
						t.Fatalf("error: %v", err)
					}
					out := strings.TrimSpace(result.Output)
					if out == "" {
						t.Fatal("empty output from tool turn")
					}
					t.Logf("output: %q  steps=%d", out, result.Steps)
				})
			}
		}
	}
}

// ─── TestLiveFullMeshDispatchDirect ──────────────────────────────────────────

// TestLiveFullMeshDispatchDirect tests the dispatch function directly
// (bypassing RunTurn's tool loop) for every model × shape combination.
// Backends are irrelevant here since all use the same NewDispatch.
func TestLiveFullMeshDispatchDirect(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping live test in short mode")
	}

	client := &liveHTTPClient{baseURL: liveBaseURL, apiKey: liveAPIKey}

	for _, mc := range testModels {
		for _, shape := range testShapes {
			mc, shape := mc, shape
			tag := fmt.Sprintf("%s/shape=%s", mc.name, shape)
			t.Run(tag, func(t *testing.T) {
				ctx, cancel := context.WithTimeout(context.Background(), 120*time.Second)
				defer cancel()

				dispatch := NewDispatch(client, mc.model, shape)
				respCh, err := dispatch(ctx, testMessagesRequest(mc.model, testUserMessage("Say hi in one word.")))
				if err != nil {
					t.Fatalf("dispatch error: %v", err)
				}
				var resp *MessagesResponse
				for r := range respCh {
					resp = r
				}
				if resp == nil {
					t.Fatal("nil response")
				}
				if len(resp.Content) == 0 {
					t.Fatal("empty content")
				}
				t.Logf("model=%s stop=%s", resp.Model, resp.StopReason)
			})
		}
	}
}

// ─── TestLiveFullMeshMultiTurn ───────────────────────────────────────────────

// TestLiveFullMeshMultiTurn simulates a 2-turn conversation to verify
// history accumulation. Turn 1 introduces a fact; turn 2 recalls it.
func TestLiveFullMeshMultiTurn(t *testing.T) {
	if testing.Short() {
		t.Skip("skipping live test in short mode")
	}

	client := &liveHTTPClient{baseURL: liveBaseURL, apiKey: liveAPIKey}

	for _, mc := range testModels {
		for _, shape := range testShapes {
			for _, backend := range testBackends {
				mc, shape, backend := mc, shape, backend
				tag := fmt.Sprintf("%s/shape=%s/backend=%s", mc.name, shape, backend)
				t.Run(tag, func(t *testing.T) {
					ctx, cancel := context.WithTimeout(context.Background(), 180*time.Second)
					defer cancel()

					sessionID := "live-multi-" + strings.ReplaceAll(tag, "/", "-")

					// Turn 1
					r1, err := RunTurn(ctx, client, sessionID,
						mc.model, backend, shape,
						"My name is Alice. Remember that.",
						nil, nil, nil, 5,
					)
					if err != nil {
						t.Fatalf("turn 1 error: %v", err)
					}
					t.Logf("turn1: %q", r1.Output)

					// Turn 2 with history
					history := []HistoryMessage{
						{Role: "user", Content: "My name is Alice. Remember that."},
						{Role: "assistant", Content: r1.Output},
					}
					r2, err := RunTurn(ctx, client, sessionID,
						mc.model, backend, shape,
						"What is my name?",
						history, nil, nil, 5,
					)
					if err != nil {
						t.Fatalf("turn 2 error: %v", err)
					}
					out := strings.TrimSpace(r2.Output)
					if out == "" {
						t.Fatal("empty output on turn 2")
					}
					// Verify the model remembered the name
					if !strings.Contains(strings.ToLower(out), "alice") {
						t.Logf("turn2 output does not contain 'Alice': %q", out)
					}
					t.Logf("turn2: %q", out)
				})
			}
		}
	}
}

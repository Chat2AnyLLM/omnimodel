package tools

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"
)

func TestCurrentTimeToolReturnsRFC3339(t *testing.T) {
	tool := CurrentTime()
	result := tool.Execute(context.Background(), Context{}, json.RawMessage(`{}`))
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Output)
	}
	if _, err := time.Parse(time.RFC3339, result.Output); err != nil {
		t.Fatalf("expected RFC3339 time, got %q: %v", result.Output, err)
	}
}

func TestCalculatorToolEvaluatesExpression(t *testing.T) {
	tool := Calculator()
	result := tool.Execute(context.Background(), Context{}, json.RawMessage(`{"expression":"(10 + 5) / 3"}`))
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Output)
	}
	if result.Output != "5" {
		t.Fatalf("result = %q, want 5", result.Output)
	}
}

func TestCalculatorToolRejectsDivisionByZero(t *testing.T) {
	tool := Calculator()
	result := tool.Execute(context.Background(), Context{}, json.RawMessage(`{"expression":"1 / 0"}`))
	if !result.IsError {
		t.Fatal("expected error")
	}
	if !strings.Contains(result.Output, "division by zero") {
		t.Fatalf("unexpected error: %q", result.Output)
	}
}

func TestWebFetchToolFetchesBody(t *testing.T) {
	server := httptest.NewServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write([]byte("hello from server"))
	}))
	defer server.Close()

	tool := WebFetch()
	payload := json.RawMessage(`{"url":"` + server.URL + `"}`)
	result := tool.Execute(context.Background(), Context{}, payload)
	if result.IsError {
		t.Fatalf("unexpected error: %s", result.Output)
	}
	if !strings.Contains(result.Output, "status: 200 OK") {
		t.Fatalf("unexpected status output: %q", result.Output)
	}
	if !strings.Contains(result.Output, "hello from server") {
		t.Fatalf("unexpected body output: %q", result.Output)
	}
}

func TestWebFetchToolRequiresURL(t *testing.T) {
	tool := WebFetch()
	result := tool.Execute(context.Background(), Context{}, json.RawMessage(`{}`))
	if !result.IsError {
		t.Fatal("expected error")
	}
	if !strings.Contains(result.Output, "url is required") {
		t.Fatalf("unexpected error: %q", result.Output)
	}
}

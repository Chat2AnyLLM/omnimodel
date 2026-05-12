package agent

import (
	"context"
	"errors"
	"strings"
	"testing"
	"time"
)

func TestClassifyDispatchError(t *testing.T) {
	if got := classifyDispatchError(errors.New("server error 503")); got != ErrorClassTransient {
		t.Fatalf("classify 503 = %s, want %s", got, ErrorClassTransient)
	}
	if got := classifyDispatchError(errors.New("file not found")); got != ErrorClassPermanent {
		t.Fatalf("classify permanent = %s, want %s", got, ErrorClassPermanent)
	}
}

func TestRetryDispatchRetriesTransientAndSucceeds(t *testing.T) {
	attempts := 0
	base := func(ctx context.Context, req *MessagesRequest) (<-chan *MessagesResponse, error) {
		attempts++
		if attempts < 3 {
			return nil, errors.New("temporary 503 upstream")
		}
		ch := make(chan *MessagesResponse, 1)
		ch <- &MessagesResponse{Content: []ContentBlock{TextBlock("ok")}}
		close(ch)
		return ch, nil
	}

	wrapped := retryDispatch(base, 3, 1*time.Millisecond, 5*time.Millisecond)
	ch, err := wrapped(context.Background(), &MessagesRequest{})
	if err != nil {
		t.Fatalf("wrapped returned error: %v", err)
	}
	if attempts != 3 {
		t.Fatalf("attempts = %d, want 3", attempts)
	}
	var got *MessagesResponse
	for resp := range ch {
		got = resp
	}
	if got == nil || len(got.Content) == 0 || got.Content[0].Text != "ok" {
		t.Fatalf("unexpected response: %#v", got)
	}
}

func TestRetryDispatchDoesNotRetryPermanent(t *testing.T) {
	attempts := 0
	base := func(ctx context.Context, req *MessagesRequest) (<-chan *MessagesResponse, error) {
		attempts++
		return nil, errors.New("invalid input payload")
	}

	wrapped := retryDispatch(base, 3, 1*time.Millisecond, 5*time.Millisecond)
	_, err := wrapped(context.Background(), &MessagesRequest{})
	if err == nil {
		t.Fatal("expected error")
	}
	if attempts != 1 {
		t.Fatalf("attempts = %d, want 1", attempts)
	}
	if !strings.Contains(err.Error(), "invalid input payload") {
		t.Fatalf("unexpected error: %v", err)
	}
}

package agent

import (
	"context"
	"errors"
	"fmt"
	"math/rand"
	"strings"
	"time"
)

type ErrorClass string

const (
	ErrorClassTransient ErrorClass = "transient"
	ErrorClassPermanent ErrorClass = "permanent"
)

func classifyDispatchError(err error) ErrorClass {
	if err == nil {
		return ErrorClassPermanent
	}

	if errors.Is(err, context.Canceled) || errors.Is(err, context.DeadlineExceeded) {
		return ErrorClassTransient
	}

	msg := strings.ToLower(err.Error())
	transientSignals := []string{
		"timeout", "timed out", "connection", "rate limit", "429", "502", "503", "504",
		"temporary", "retry", "eof", "reset by peer", "unavailable",
	}
	for _, signal := range transientSignals {
		if strings.Contains(msg, signal) {
			return ErrorClassTransient
		}
	}

	return ErrorClassPermanent
}

func retryDispatch(base DispatchFn, maxAttempts int, baseDelay, maxDelay time.Duration) DispatchFn {
	if maxAttempts <= 1 {
		return base
	}
	if baseDelay <= 0 {
		baseDelay = 500 * time.Millisecond
	}
	if maxDelay <= 0 {
		maxDelay = 8 * time.Second
	}

	return func(ctx context.Context, req *MessagesRequest) (<-chan *MessagesResponse, error) {
		var lastErr error
		for attempt := 1; attempt <= maxAttempts; attempt++ {
			respCh, err := base(ctx, req)
			if err == nil {
				return respCh, nil
			}
			lastErr = err

			if ctx.Err() != nil {
				return nil, ctx.Err()
			}
			if classifyDispatchError(err) != ErrorClassTransient {
				return nil, err
			}
			if attempt == maxAttempts {
				break
			}

			delay := retryDelayWithJitter(attempt, baseDelay, maxDelay)
			timer := time.NewTimer(delay)
			select {
			case <-ctx.Done():
				if !timer.Stop() {
					<-timer.C
				}
				return nil, ctx.Err()
			case <-timer.C:
			}
		}

		if lastErr == nil {
			lastErr = errors.New("dispatch failed")
		}
		return nil, fmt.Errorf("dispatch failed after %d attempts: %w", maxAttempts, lastErr)
	}
}

func retryDelayWithJitter(attempt int, baseDelay, maxDelay time.Duration) time.Duration {
	if attempt < 1 {
		attempt = 1
	}
	base := baseDelay << (attempt - 1)
	if base > maxDelay {
		base = maxDelay
	}
	// Add 0-250ms jitter to avoid synchronized retries.
	jitter := time.Duration(rand.Int63n(int64(250 * time.Millisecond)))
	delay := base + jitter
	if delay > maxDelay {
		return maxDelay
	}
	return delay
}

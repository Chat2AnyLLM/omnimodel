package agent

import (
	"context"
	"strings"
	"sync"
)

// SummarizerFn takes a list of messages and returns a summary string.
type SummarizerFn func(ctx context.Context, msgs []Message) (string, error)

// Memory is the interface for agent conversation memory.
type Memory interface {
	Append(msg Message)
	Messages() []Message
	Compact(ctx context.Context, summarizer SummarizerFn) error
}

// BufferMemory keeps the last N messages.
type BufferMemory struct {
	mu       sync.RWMutex
	messages []Message
	maxSize  int
}

// NewBufferMemory creates a BufferMemory with the given max size.
// Default is 20 if maxSize <= 0.
func NewBufferMemory(maxSize int) *BufferMemory {
	if maxSize <= 0 {
		maxSize = 20
	}
	return &BufferMemory{
		messages: make([]Message, 0),
		maxSize:  maxSize,
	}
}

func (b *BufferMemory) Append(msg Message) {
	b.mu.Lock()
	defer b.mu.Unlock()
	b.messages = append(b.messages, msg)
	if len(b.messages) > b.maxSize {
		b.messages = b.messages[len(b.messages)-b.maxSize:]
	}
}

func (b *BufferMemory) Messages() []Message {
	b.mu.RLock()
	defer b.mu.RUnlock()
	result := make([]Message, len(b.messages))
	copy(result, b.messages)
	return result
}

func (b *BufferMemory) Compact(_ context.Context, _ SummarizerFn) error {
	// BufferMemory doesn't need compaction; it just truncates on Append.
	return nil
}

// SummaryMemory tracks estimated token budget and summarizes when over budget.
type SummaryMemory struct {
	mu          sync.RWMutex
	messages    []Message
	tokenBudget int
}

// NewSummaryMemory creates a SummaryMemory with the given token budget.
// Token estimation uses word count × 1.3.
func NewSummaryMemory(tokenBudget int) *SummaryMemory {
	if tokenBudget <= 0 {
		tokenBudget = 4000
	}
	return &SummaryMemory{
		messages:    make([]Message, 0),
		tokenBudget: tokenBudget,
	}
}

func (s *SummaryMemory) Append(msg Message) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.messages = append(s.messages, msg)
}

func (s *SummaryMemory) Messages() []Message {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]Message, len(s.messages))
	copy(result, s.messages)
	return result
}

// Compact checks if messages exceed the token budget. If so, it summarizes
// the oldest half and replaces them with a single system message.
func (s *SummaryMemory) Compact(ctx context.Context, summarizer SummarizerFn) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if summarizer == nil {
		return nil
	}

	estimated := s.estimateTokens()
	if estimated <= s.tokenBudget {
		return nil
	}

	// Summarize the oldest half
	half := len(s.messages) / 2
	if half == 0 {
		return nil
	}

	toSummarize := s.messages[:half]
	summary, err := summarizer(ctx, toSummarize)
	if err != nil {
		return err
	}

	// Replace oldest half with summary system message
	summaryMsg := Message{
		Role:    "system",
		Content: []ContentBlock{TextBlock("[Conversation Summary] " + summary)},
	}

	remaining := make([]Message, 0, 1+len(s.messages)-half)
	remaining = append(remaining, summaryMsg)
	remaining = append(remaining, s.messages[half:]...)
	s.messages = remaining

	return nil
}

// estimateTokens estimates token count using word count × 1.3.
func (s *SummaryMemory) estimateTokens() int {
	total := 0
	for _, msg := range s.messages {
		total += estimateMessageTokens(msg)
	}
	return total
}


func estimateMessageTokens(msg Message) int {
	text := extractTextContent(msg.Content)
	words := len(strings.Fields(text))
	return int(float64(words) * 1.3)
}

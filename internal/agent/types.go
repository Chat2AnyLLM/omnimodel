package agent

import toolspkg "omnillm/internal/tools"

type ContentBlock struct {
	Type      string         `json:"type"`
	Text      string         `json:"text,omitempty"`
	Thinking  string         `json:"thinking,omitempty"`
	Signature *string        `json:"signature,omitempty"`
	ID        string         `json:"id,omitempty"`
	ToolUseID string         `json:"tool_use_id,omitempty"`
	Name      string         `json:"name,omitempty"`
	Input     map[string]any `json:"input,omitempty"`
	Content   string         `json:"content,omitempty"`
	IsError   *bool          `json:"is_error,omitempty"`
}

type Message struct {
	Role    string         `json:"role"`
	Content []ContentBlock `json:"content"`
}

type Usage struct {
	InputTokens  int `json:"input_tokens"`
	OutputTokens int `json:"output_tokens"`
}

type StopReason string

const (
	StopReasonEndTurn      StopReason = "end_turn"
	StopReasonMaxTokens    StopReason = "max_tokens"
	StopReasonToolUse      StopReason = "tool_use"
	StopReasonStopSequence StopReason = "stop_sequence"
	StopReasonContentFilter StopReason = "content_filter"
)

type MessagesRequest struct {
	Model      string                   `json:"model"`
	MaxTokens  int                      `json:"max_tokens"`
	System     []ContentBlock           `json:"system,omitempty"`
	Messages   []Message                `json:"messages"`
	Tools      []toolspkg.ToolDefinition `json:"tools,omitempty"`
	ToolChoice any                      `json:"tool_choice,omitempty"`
	Stream     bool                     `json:"stream,omitempty"`
}

type MessagesResponse struct {
	ID           string         `json:"id"`
	Model        string         `json:"model"`
	Content      []ContentBlock `json:"content"`
	StopReason   StopReason     `json:"stop_reason"`
	StopSequence *string        `json:"stop_sequence,omitempty"`
	Usage        *Usage         `json:"usage,omitempty"`
}

func TextBlock(text string) ContentBlock {
	return ContentBlock{Type: "text", Text: text}
}

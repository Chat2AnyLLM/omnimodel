package chat

import (
	"io"
	"net/http"
	"os"

	agentpkg "omnillm/internal/agent"
)

type permissionRequestMsg struct {
	req    agentpkg.PermissionRequest
	respCh chan bool
}

type Client interface {
	Get(path string) ([]byte, error)
	Post(path string, body any) ([]byte, error)
	Put(path string, body any) ([]byte, error)
	Delete(path string) ([]byte, error)
	PostStream(path string, body any) (*http.Response, error)
}

type CommandContext interface {
	InOrStdin() io.Reader
	OutOrStdout() io.Writer
	ErrOrStderr() io.Writer
}

type stdioCommandContext struct{}

func (stdioCommandContext) InOrStdin() io.Reader   { return os.Stdin }
func (stdioCommandContext) OutOrStdout() io.Writer { return os.Stdout }
func (stdioCommandContext) ErrOrStderr() io.Writer { return os.Stderr }

type SessionState struct {
	ID           string
	Model        string
	Mode         string
	AgentBackend string
	IsTTY        bool
	Picker       ModelPickerFunc
}

type ModelPickerFunc func(string, []ModelInfo) (string, error)

type Message struct {
	Role    string `json:"role"`
	Content string `json:"content"`
}

type ModelInfo struct {
	ID           string
	Owner        string
	OwnerName    string
	Name         string
	Selector     string
	ProviderID   string
	ProviderName string
}

type replCommandResult struct {
	handled      bool
	exit         bool
	model        string
	agentBackend string
}

type sseEvent struct {
	data []byte
}

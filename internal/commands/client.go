package commands

import (
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
	"strings"

	"github.com/manifoldco/promptui"
	"github.com/mattn/go-isatty"
	"github.com/spf13/cobra"
)

// Client is a lightweight HTTP client for the OmniLLM admin API.
type Client struct {
	BaseURL    string
	APIKey     string
	OutputMode string
	http       *http.Client
	stdin      io.Reader
	stdout     io.Writer
	stderr     io.Writer
}

// NewClient builds a Client from cobra persistent flags and environment variables.
func NewClient(cmd *cobra.Command) *Client {
	server := os.Getenv("OMNILLM_SERVER")
	if v, err := cmd.Root().PersistentFlags().GetString("server"); err == nil && v != "" {
		server = v
	}
	if server == "" {
		server = "http://127.0.0.1:5000"
	}
	server = strings.TrimRight(server, "/")

	apiKey := os.Getenv("OMNILLM_API_KEY")
	if v, err := cmd.Root().PersistentFlags().GetString("api-key"); err == nil && v != "" {
		apiKey = v
	}

	output := "table"
	if v, err := cmd.Root().PersistentFlags().GetString("output"); err == nil && v != "" {
		output = v
	}

	return &Client{
		BaseURL:    server,
		APIKey:     apiKey,
		OutputMode: output,
		http:       &http.Client{},
	}
}

func (c *Client) newRequest(method, path string, body io.Reader) (*http.Request, error) {
	url := c.BaseURL + path
	req, err := http.NewRequest(method, url, body)
	if err != nil {
		return nil, err
	}
	if body != nil {
		req.Header.Set("Content-Type", "application/json")
	}
	if c.APIKey != "" {
		req.Header.Set("Authorization", "Bearer "+c.APIKey)
	}
	return req, nil
}

func (c *Client) do(method, path string, bodyObj any) ([]byte, error) {
	var bodyReader io.Reader
	if bodyObj != nil {
		b, err := json.Marshal(bodyObj)
		if err != nil {
			return nil, fmt.Errorf("marshal request: %w", err)
		}
		bodyReader = bytes.NewReader(b)
	}

	req, err := c.newRequest(method, path, bodyReader)
	if err != nil {
		return nil, err
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request to %s failed: %w\n(Is the server running at %s?)", path, err, c.BaseURL)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode >= 400 {
		// Try to extract "error" or "message" field from JSON
		var errResp map[string]interface{}
		if jsonErr := json.Unmarshal(data, &errResp); jsonErr == nil {
			if msg, ok := errResp["error"].(string); ok {
				return nil, fmt.Errorf("server error (%d): %s", resp.StatusCode, msg)
			}
			if msg, ok := errResp["message"].(string); ok {
				return nil, fmt.Errorf("server error (%d): %s", resp.StatusCode, msg)
			}
		}
		return nil, fmt.Errorf("server error (%d): %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}

	return data, nil
}

func (c *Client) Get(path string) ([]byte, error)             { return c.do("GET", path, nil) }
func (c *Client) Post(path string, body any) ([]byte, error)  { return c.do("POST", path, body) }
func (c *Client) Put(path string, body any) ([]byte, error)   { return c.do("PUT", path, body) }
func (c *Client) Patch(path string, body any) ([]byte, error) { return c.do("PATCH", path, body) }
func (c *Client) Delete(path string) ([]byte, error)          { return c.do("DELETE", path, nil) }

// DoRaw performs a request with a raw reader body (e.g. multipart form).
func (c *Client) DoRaw(method, path, contentType string, body io.Reader) ([]byte, error) {
	req, err := c.newRequest(method, path, body)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", contentType)

	resp, err := c.http.Do(req)
	if err != nil {
		return nil, fmt.Errorf("request to %s failed: %w", path, err)
	}
	defer resp.Body.Close()

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read response: %w", err)
	}

	if resp.StatusCode >= 400 {
		var errResp map[string]interface{}
		if jsonErr := json.Unmarshal(data, &errResp); jsonErr == nil {
			if msg, ok := errResp["error"].(string); ok {
				return nil, fmt.Errorf("server error (%d): %s", resp.StatusCode, msg)
			}
		}
		return nil, fmt.Errorf("server error (%d): %s", resp.StatusCode, strings.TrimSpace(string(data)))
	}
	return data, nil
}

// GetStream returns the raw response body for streaming (caller must close).
func (c *Client) GetStream(path string) (*http.Response, error) {
	req, err := c.newRequest("GET", path, nil)
	if err != nil {
		return nil, err
	}
	return c.http.Do(req)
}

// PostStream performs a POST and returns the raw response body for streaming (caller must close).
func (c *Client) PostStream(path string, body any) (*http.Response, error) {
	var bodyReader io.Reader
	if body != nil {
		b, err := json.Marshal(body)
		if err != nil {
			return nil, fmt.Errorf("marshal request: %w", err)
		}
		bodyReader = bytes.NewReader(b)
	}
	req, err := c.newRequest("POST", path, bodyReader)
	if err != nil {
		return nil, err
	}
	return c.http.Do(req)
}

// IsJSON returns true when --output json was requested.
func (c *Client) IsJSON() bool { return c.OutputMode == "json" }

// parseJSON unmarshals JSON data into a value.
func (c *Client) parseJSON(data []byte, v interface{}) error {
	return json.Unmarshal(data, v)
}

// PrintJSON pretty-prints raw JSON to stdout, or prints as-is on parse error.
func (c *Client) PrintJSON(data []byte) {
	PrintJSON(os.Stdout, data)
}

func PrintJSON(w io.Writer, data []byte) {
	var v interface{}
	if err := json.Unmarshal(data, &v); err != nil {
		fmt.Fprintln(w, string(data))
		return
	}
	out, _ := json.MarshalIndent(v, "", "  ")
	fmt.Fprintln(w, string(out))
}

func IsTerminalWriter(w io.Writer) bool {
	file, ok := w.(*os.File)
	if !ok {
		return false
	}
	return isatty.IsTerminal(file.Fd()) || isatty.IsCygwinTerminal(file.Fd())
}

func FormatChatPrompt(role string, tty bool) string {
	if !tty {
		return role + "> "
	}

	switch role {
	case "You":
		return "\x1b[36mYou>\x1b[0m "
	case "Assistant":
		return "\x1b[35mAssistant>\x1b[0m "
	default:
		return role + "> "
	}
}

func FormatChatHeader(role string, tty bool) string {
	if !tty {
		return role + ">"
	}

	switch role {
	case "Assistant":
		return "\x1b[35mAssistant>\x1b[0m"
	case "You":
		return "\x1b[36mYou>\x1b[0m"
	default:
		return role + ">"
	}
}

// Confirm asks the user for a yes/no confirmation; returns true if yes.
func Confirm(cmd *cobra.Command, prompt string) bool {
	fmt.Fprintf(cmd.ErrOrStderr(), "%s [y/N] ", prompt)
	var ans string
	fmt.Fscan(cmd.InOrStdin(), &ans)
	return strings.ToLower(strings.TrimSpace(ans)) == "y"
}

func SelectAuthProvider(prompt string, providers []authProviderOption) (string, error) {
	if len(providers) == 0 {
		return "", fmt.Errorf("no items to select from")
	}

	templates := &promptui.SelectTemplates{
		Label:    "{{ . }}",
		Active:   "▸ {{ .Label | cyan }}",
		Inactive: "  {{ .Label }}",
		Selected: "✓ {{ .Label }}",
		Details:  "{{ \"Provider:\" | faint }} {{ .Type }}",
	}

	searcher := func(input string, index int) bool {
		provider := providers[index]
		needle := strings.ToLower(strings.TrimSpace(input))
		if needle == "" {
			return true
		}
		return strings.Contains(strings.ToLower(provider.Label), needle) || strings.Contains(strings.ToLower(provider.Type), needle)
	}

	selectPrompt := promptui.Select{
		Label:             prompt,
		Items:             providers,
		Templates:         templates,
		Size:              10,
		Searcher:          searcher,
		StartInSearchMode: true,
	}

	index, _, err := selectPrompt.Run()
	if err != nil {
		return "", err
	}
	return providers[index].Type, nil
}

func SelectFromOptions(prompt string, options []string) (string, error) {
	if len(options) == 0 {
		return "", fmt.Errorf("no items to select from")
	}

	selectPrompt := promptui.Select{
		Label: prompt,
		Items: options,
		Size:  len(options),
		Searcher: func(input string, index int) bool {
			needle := strings.ToLower(strings.TrimSpace(input))
			if needle == "" {
				return true
			}
			return strings.Contains(strings.ToLower(options[index]), needle)
		},
		StartInSearchMode: true,
	}

	index, _, err := selectPrompt.Run()
	if err != nil {
		return "", err
	}
	return options[index], nil
}

func PromptText(label string, required bool, defaultValue string) (string, error) {
	prompt := promptui.Prompt{Label: label, Default: defaultValue}
	if required {
		prompt.Validate = func(input string) error {
			if strings.TrimSpace(input) == "" {
				return fmt.Errorf("%s is required", strings.ToLower(label))
			}
			return nil
		}
	}
	return prompt.Run()
}

func PromptSecret(label string, required bool) (string, error) {
	prompt := promptui.Prompt{Label: label, Mask: '*'}
	if required {
		prompt.Validate = func(input string) error {
			if strings.TrimSpace(input) == "" {
				return fmt.Errorf("%s is required", strings.ToLower(label))
			}
			return nil
		}
	}
	return prompt.Run()
}

func SelectFromList(prompt string, items []string, input io.Reader) (string, error) {
	if len(items) == 0 {
		return "", fmt.Errorf("no items to select from")
	}
	if input == nil {
		input = os.Stdin
	}

	fmt.Println(prompt)
	for i, item := range items {
		fmt.Printf("%d. %s\n", i+1, item)
	}
	fmt.Print("> ")

	var ans string
	if _, err := fmt.Fscan(input, &ans); err != nil {
		return "", fmt.Errorf("read selection: %w", err)
	}

	selection, err := strconv.Atoi(strings.TrimSpace(ans))
	if err != nil {
		return "", fmt.Errorf("invalid selection: must be a number")
	}
	if selection < 1 || selection > len(items) {
		return "", fmt.Errorf("selection out of range: must be between 1 and %d", len(items))
	}

	return items[selection-1], nil
}

// SuccessMsg prints a bold-green ✓ success message.
func SuccessMsg(cmd *cobra.Command, format string, a ...any) {
	fmt.Fprintf(cmd.OutOrStdout(), "✓ "+format+"\n", a...)
}

// ErrorMsg prints an error message to stderr.
func ErrorMsg(cmd *cobra.Command, format string, a ...any) {
	fmt.Fprintf(cmd.ErrOrStderr(), "Error: "+format+"\n", a...)
}

// padRight pads a string to at least n characters.
func padRight(s string, n int) string {
	if len(s) >= n {
		return s
	}
	return s + strings.Repeat(" ", n-len(s))
}

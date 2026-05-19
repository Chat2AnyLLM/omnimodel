package commands

import (
	"bufio"
	"bytes"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"time"

	"github.com/spf13/cobra"
)

var LogsCmd = &cobra.Command{
	Use:   "logs",
	Short: "Stream or view server logs",
}

func init() {
	logsTailCmd.Flags().String("level", "", "Filter: only show messages at this level or above (error|warn|info|debug|trace)")
	LogsCmd.AddCommand(logsTailCmd)
}

var logsTailCmd = &cobra.Command{
	Use:   "tail",
	Short: "Stream live server logs (SSE)",
	RunE: func(cmd *cobra.Command, args []string) error {
		levelFilter, _ := cmd.Flags().GetString("level")

		c := NewClient(cmd)
		resp, err := c.GetStream("/api/admin/logs/stream")
		if err != nil {
			return fmt.Errorf("connect to log stream: %w", err)
		}
		defer resp.Body.Close()

		if resp.StatusCode != http.StatusOK {
			body, _ := io.ReadAll(resp.Body)
			return fmt.Errorf("server returned %d: %s", resp.StatusCode, strings.TrimSpace(string(body)))
		}

		fmt.Fprintf(cmd.ErrOrStderr(), "Connected to log stream (Ctrl+C to stop)\n\n")

		out := cmd.OutOrStdout()
		scanner := bufio.NewScanner(resp.Body)
		for scanner.Scan() {
			line := scanner.Text()

			// SSE format: lines starting with "data: " contain the payload
			if !strings.HasPrefix(line, "data: ") {
				continue
			}
			payload := line[6:] // strip "data: "

			// Try to parse as JSON for filtering and pretty display
			var entry map[string]interface{}
			if err := json.Unmarshal([]byte(payload), &entry); err != nil {
				// Not JSON — print raw
				fmt.Fprintln(out, payload)
				continue
			}

			// Level filtering
			if levelFilter != "" {
				level, _ := entry["level"].(string)
				if !isLevelAtOrAbove(level, levelFilter) {
					continue
				}
			}

			// Formatted output
			ts, _ := entry["time"].(string)
			level, _ := entry["level"].(string)
			message, _ := entry["message"].(string)

			if ts == "" {
				ts = time.Now().Format("15:04:05")
			} else if t, err := time.Parse(time.RFC3339, ts); err == nil {
				ts = t.Format("15:04:05")
			}

			levelStr := padRight(strings.ToUpper(level), 5)
			fmt.Fprintf(out, "%s  %s  %s\n", ts, levelStr, message)
		}

		if err := scanner.Err(); err != nil {
			return fmt.Errorf("log stream error: %w", err)
		}
		return nil
	},
}

// levelOrder maps log level names to severity (lower = more severe).
var levelOrder = map[string]int{
	"fatal": 0,
	"error": 1,
	"warn":  2,
	"info":  3,
	"debug": 4,
	"trace": 5,
}

// isLevelAtOrAbove returns true if msgLevel is at least as severe as filterLevel.
func isLevelAtOrAbove(msgLevel, filterLevel string) bool {
	m, mOK := levelOrder[strings.ToLower(msgLevel)]
	f, fOK := levelOrder[strings.ToLower(filterLevel)]
	if !mOK || !fOK {
		return true // unknown levels pass through
	}
	return m <= f
}

// ─── placeholder so bytes/io are not unused if stream is empty ───────────────
var _ = bytes.NewBuffer
var _ = io.Discard
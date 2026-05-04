package tools

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"strings"
)

func listDirectory(dir string) Result {
	entries, err := os.ReadDir(dir)
	if err != nil {
		return Result{Output: "error: " + err.Error(), IsError: true}
	}
	if len(entries) == 0 {
		return Result{Output: "(empty directory)"}
	}

	var buf strings.Builder
	for _, e := range entries {
		if e.IsDir() {
			fmt.Fprintf(&buf, "%s/\n", e.Name())
		} else {
			info, _ := e.Info()
			if info != nil {
				fmt.Fprintf(&buf, "%s (%d bytes)\n", e.Name(), info.Size())
			} else {
				fmt.Fprintf(&buf, "%s\n", e.Name())
			}
		}
	}
	return Result{Output: strings.TrimRight(buf.String(), "\n")}
}

func decodeOptionalJSON[T any](input json.RawMessage, out *T) error {
	trimmed := strings.TrimSpace(string(input))
	if trimmed == "" || trimmed == "null" {
		return nil
	}
	return json.Unmarshal(input, out)
}

func noOpContext(_ context.Context) {}

package tools

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strings"
)

func walkGlob(base, pattern string) ([]string, error) {
	pattern = filepath.FromSlash(pattern)
	var matches []string
	err := filepath.WalkDir(base, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // skip inaccessible entries
		}
		if d.IsDir() {
			name := d.Name()
			if name == ".git" || name == "node_modules" || name == "vendor" {
				return filepath.SkipDir
			}
			return nil
		}
		rel, relErr := filepath.Rel(base, path)
		if relErr != nil {
			return nil
		}
		ok, _ := filepath.Match(pattern, rel)
		if !ok && strings.Contains(pattern, "**") {
			idx := strings.LastIndex(pattern, "**/")
			if idx >= 0 {
				suffix := pattern[idx+3:]
				ok, _ = filepath.Match(suffix, filepath.Base(rel))
			}
		}
		if ok {
			matches = append(matches, rel)
		}
		return nil
	})
	return matches, err
}

func walkGrep(ctx context.Context, base, pattern, glob string, ci bool, max int) (string, error) {
	needle := pattern
	if ci {
		needle = strings.ToLower(pattern)
	}

	var results []string
	err := filepath.WalkDir(base, func(path string, d os.DirEntry, err error) error {
		if err != nil {
			return nil // skip inaccessible entries
		}
		if d.IsDir() {
			name := d.Name()
			if name == ".git" || name == "node_modules" || name == "vendor" {
				return filepath.SkipDir
			}
			return nil
		}
		if glob != "" {
			matched, _ := filepath.Match(glob, filepath.Base(path))
			if !matched {
				return nil
			}
		}

		f, err := os.Open(path)
		if err != nil {
			return nil
		}
		defer f.Close()

		scanner := bufio.NewScanner(f)
		scanner.Buffer(make([]byte, 0, 64*1024), 1024*1024)
		lineNo := 0
		for scanner.Scan() {
			select {
			case <-ctx.Done():
				return ctx.Err()
			default:
			}
			lineNo++
			line := scanner.Text()
			haystack := line
			if ci {
				haystack = strings.ToLower(line)
			}
			if strings.Contains(haystack, needle) {
				rel, _ := filepath.Rel(base, path)
				results = append(results, fmt.Sprintf("%s:%d: %s", rel, lineNo, line))
				if len(results) >= max {
					return errStopWalk
				}
			}
		}
		return nil
	})

	if err != nil && err != errStopWalk {
		return "", err
	}
	if len(results) == 0 {
		return "(no matches)", nil
	}
	return strings.Join(results, "\n"), nil
}

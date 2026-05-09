package omnicode

import (
	"fmt"
	"os"
	"runtime"
	"strings"
)

const logo = `
  ___                  _  ____          _
 / _ \ _ __ ___  _ __ (_)/ ___|___   __| | ___
| | | | '_ ` + "`" + ` _ \| '_ \| | |   / _ \ / _` + "`" + ` |/ _ \
| |_| | | | | | | | | | | |__| (_) | (_| |  __/
 \___/|_| |_| |_|_| |_|_|\____\___/ \__,_|\___|
`

// getVersion reads VERSION from the repo root, with a fallback.
func getVersion() string {
	data, err := os.ReadFile("VERSION")
	if err == nil {
		v := strings.TrimSpace(string(data))
		if v != "" {
			return v
		}
	}
	return "v0.0.1"
}

// PrintBanner writes the startup banner to stdout.
func PrintBanner() {
	version := getVersion()
	cwd, _ := os.Getwd()

	fmt.Print(logo)
	fmt.Printf("  Version:  %s\n", version)
	fmt.Printf("  CWD:      %s\n", cwd)
	fmt.Printf("  Platform: %s/%s\n", runtime.GOOS, runtime.GOARCH)
	fmt.Printf("  Go:       %s\n", runtime.Version())
	fmt.Println()
}

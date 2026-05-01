package commands

import (
	"os/exec"
	"runtime"
)

// openBrowserFunc is the function used to open a browser URL.
// It can be replaced in tests.
var openBrowserFunc = defaultOpenBrowser

// defaultOpenBrowser opens the given URL in the default browser.
// Returns true if the browser process was started successfully.
func defaultOpenBrowser(url string) bool {
	var cmd *exec.Cmd
	switch runtime.GOOS {
	case "darwin":
		cmd = exec.Command("open", url)
	case "windows":
		cmd = exec.Command("rundll32", "url.dll,FileProtocolHandler", url)
	default: // linux and other Unix-like systems
		cmd = exec.Command("xdg-open", url)
	}
	return cmd.Start() == nil
}

// tryOpenBrowser attempts to open the given URL in the default browser.
// Returns true if the browser was successfully launched.
func tryOpenBrowser(url string) bool {
	return openBrowserFunc(url)
}

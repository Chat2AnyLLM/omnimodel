//go:build windows

package chat

import "golang.org/x/sys/windows"

const windowsUTF8CodePage uint32 = 65001

// configureUTF8Console sets the Windows console OUTPUT code page to UTF-8 so
// that Bubble Tea's UTF-8 output renders correctly.
//
// The INPUT code page is intentionally left at the system default (e.g. CP936
// on Chinese Windows). Bubble Tea's WithInputTTY option opens CONIN$ as a
// separate file handle, which bypasses the ReadConsoleInput (Unicode) path and
// falls back to byte-based ReadFile. Those bytes are then fed through
// go-localereader, which converts from CP_ACP (the system ANSI code page) to
// UTF-8. Setting the input CP to 65001 would cause ReadFile to emit UTF-8
// bytes that localereader then misinterprets as GBK, producing mojibake.
func configureUTF8Console() error {
	return windows.SetConsoleOutputCP(windowsUTF8CodePage)
}

func init() {
	_ = configureUTF8Console()
}

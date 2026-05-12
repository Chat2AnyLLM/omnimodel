//go:build windows

package chat

import "golang.org/x/sys/windows"

// init forces the Windows console input and output code pages to UTF-8 (65001).
// Without this, Chinese Windows defaults to CP936 (GBK), and multi-byte input
// typed into the TUI textarea is decoded as GBK while bubbletea expects UTF-8,
// producing mojibake like "浣犲ソ" for "你好".
func init() {
	const cpUTF8 uint32 = 65001
	_ = windows.SetConsoleCP(cpUTF8)
	_ = windows.SetConsoleOutputCP(cpUTF8)
}

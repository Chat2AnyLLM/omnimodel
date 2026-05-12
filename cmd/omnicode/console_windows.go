//go:build windows

package main

import "golang.org/x/sys/windows"

// init forces the Windows console code pages to UTF-8 (65001) BEFORE any other
// package (notably bubbletea / readline) opens the console handles. On Chinese
// Windows the default is CP936 (GBK), which causes typed Chinese characters in
// the TUI textarea to render as mojibake like "浣犲ソ" for "你好".
//
// Placing this init in the main package guarantees it runs before package
// imports like "internal/chat" or "github.com/charmbracelet/bubbletea" pull
// in their own init chains.
func init() {
	const cpUTF8 uint32 = 65001
	_ = windows.SetConsoleCP(cpUTF8)
	_ = windows.SetConsoleOutputCP(cpUTF8)
}

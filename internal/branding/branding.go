// Package branding holds shared branding assets (logo, product name) that
// can be imported by any internal package without creating import cycles.
package branding

// Logo is the ASCII art banner displayed in the TUI when no messages exist yet.
const Logo = `
  ___                  _  ____          _
 / _ \ _ __ ___  _ __ (_)/ ___|___   __| | ___
| | | | '_ ` + "`" + ` _ \| '_ \| | |   / _ \ / _` + "`" + ` |/ _ \
| |_| | | | | | | | | | | |__| (_) | (_| |  __/
 \___/|_| |_| |_|_| |_|_|\____\___/ \__,_|\___|
`

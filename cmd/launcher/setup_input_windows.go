//go:build windows

package main

import (
	"io"
	"os"
)

// consoleInput returns a reader attached to the physical console input buffer.
// A windowsgui process has no inherited stdin; after AllocConsole the input
// buffer is reachable via the special CONIN$ device (echo + line editing are
// handled by the console itself).
func consoleInput() io.Reader {
	f, err := os.OpenFile("CONIN$", os.O_RDWR, 0)
	if err != nil {
		return os.Stdin
	}
	return f
}

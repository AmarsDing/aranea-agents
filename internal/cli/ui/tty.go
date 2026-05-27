package ui

import (
	"io"
	"os"
	"strconv"

	"github.com/mattn/go-isatty"
)

// UI holds terminal capability flags and I/O handles.
type UI struct {
	IsTTY   bool
	Width   int
	NoColor bool
	Verbose bool
	In      io.Reader
	Out     io.Writer
	Err     io.Writer
}

// Detect inspects the given streams and environment variables to produce a UI.
func Detect(in io.Reader, out, errW io.Writer, noColorFlag bool) UI {
	isTTY := isTerminal(out)

	noColor := noColorFlag ||
		os.Getenv("NO_COLOR") != "" ||
		os.Getenv("ARANEA_NO_COLOR") == "1"

	width := 80
	if isTTY {
		if w := termWidth(); w > 0 {
			width = w
		}
	}

	return UI{
		IsTTY:   isTTY,
		Width:   width,
		NoColor: noColor,
		In:      in,
		Out:     out,
		Err:     errW,
	}
}

// isTerminal returns true if w is backed by a real TTY file descriptor.
func isTerminal(w io.Writer) bool {
	if f, ok := w.(*os.File); ok {
		return isatty.IsTerminal(f.Fd()) || isatty.IsCygwinTerminal(f.Fd())
	}
	return false
}

// termWidth attempts to read terminal width from environment.
func termWidth() int {
	if v := os.Getenv("COLUMNS"); v != "" {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			return n
		}
	}
	return 80
}

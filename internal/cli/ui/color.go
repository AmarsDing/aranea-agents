package ui

import (
	"fmt"
	"io"

	"github.com/fatih/color"
)

// ColorFn is a function that applies ANSI color to a string.
type ColorFn func(format string, a ...any) string

// Color returns a ColorFn for the given named style.
// If the UI has NoColor set, all color functions are no-ops.
func (u UI) Color(name string) ColorFn {
	if u.NoColor {
		color.NoColor = true
	}
	switch name {
	case "red":
		return color.New(color.FgRed, color.Bold).SprintfFunc()
	case "yellow":
		return color.New(color.FgYellow).SprintfFunc()
	case "green":
		return color.New(color.FgGreen).SprintfFunc()
	case "dim":
		return color.New(color.Faint).SprintfFunc()
	case "bold":
		return color.New(color.Bold).SprintfFunc()
	case "cyan":
		return color.New(color.FgCyan).SprintfFunc()
	default:
		return fmt.Sprintf
	}
}

// Fprintf writes colorized output to w.
func colorFprintf(w io.Writer, fn ColorFn, format string, a ...any) {
	fmt.Fprint(w, fn(format, a...))
}

// Fprint writes plain output to w.
func plainFprintf(w io.Writer, format string, a ...any) {
	fmt.Fprintf(w, format, a...)
}

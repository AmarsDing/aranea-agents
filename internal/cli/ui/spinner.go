package ui

import (
	"fmt"
	"io"
	"time"
)

// StopFunc stops a spinner when called.
type StopFunc func()

// Spinner starts a spinner animation on the UI's Err stream.
// It only displays if the UI is a TTY.
// Blocks >200ms before displaying; after >5s shows elapsed time.
// Returns a StopFunc that must be called to stop the animation.
func (u UI) Spinner(label string) StopFunc {
	if !u.IsTTY {
		return func() {}
	}

	done := make(chan struct{})
	go func() {
		frames := []string{"|", "/", "-", "\\"}
		i := 0
		start := time.Now()
		// Wait 200ms before showing spinner.
		select {
		case <-done:
			return
		case <-time.After(200 * time.Millisecond):
		}

		ticker := time.NewTicker(100 * time.Millisecond)
		defer ticker.Stop()

		for {
			select {
			case <-done:
				// Clear spinner line.
				fmt.Fprintf(u.Err, "\r\033[K")
				return
			case <-ticker.C:
				elapsed := time.Since(start)
				if elapsed > 5*time.Second {
					fmt.Fprintf(u.Err, "\r%s %s (%.0fs)...",
						frames[i%len(frames)], label, elapsed.Seconds())
				} else {
					fmt.Fprintf(u.Err, "\r%s %s...", frames[i%len(frames)], label)
				}
				i++
			}
		}
	}()

	return func() {
		close(done)
	}
}

// PrintfErr writes a formatted message to the Err stream.
func (u UI) PrintfErr(format string, args ...any) {
	fmt.Fprintf(u.Err, format, args...)
}

// Fprintln writes a line to the Out stream.
func (u UI) Fprintln(w io.Writer, args ...any) {
	fmt.Fprintln(w, args...)
}

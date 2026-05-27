package output

import (
	"fmt"
	"io"
	"strings"
)

// kvPrinter renders key=value pairs (non-TTY fallback).
type kvPrinter struct {
	w io.Writer
}

// PrintKV writes key=value pairs.
func (p *kvPrinter) PrintKV(pairs ...string) error {
	var sb strings.Builder
	for i := 0; i+1 < len(pairs); i += 2 {
		sb.WriteString(pairs[i])
		sb.WriteByte('=')
		sb.WriteString(pairs[i+1])
		sb.WriteByte('\n')
	}
	_, err := fmt.Fprint(p.w, sb.String())
	return err
}

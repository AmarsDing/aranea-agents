package output

import (
	"io"

	"aranea-agents/internal/cli/clierr"
)

// Format enumerates supported output formats.
type Format string

const (
	FormatText Format = "text"
	FormatJSON Format = "json"
)

// Printer is the top-level output interface for CLI commands.
type Printer interface {
	// PrintList renders a list of items ([]proto.Message or []map[string]string).
	PrintList(items any, total int) error
	// PrintDetail renders a single item.
	PrintDetail(item any) error
	// PrintError renders a CLIError.
	PrintError(e *clierr.CLIError) error
	// PrintSuccess renders a success message with optional key=value pairs.
	PrintSuccess(message string, kv ...string) error
	// PrintKeyValue renders key=value pairs.
	PrintKeyValue(pairs ...string) error
}

// NewPrinter creates a Printer for the given format and options.
func NewPrinter(format Format, quiet, noColor bool, w io.Writer) Printer {
	switch format {
	case FormatJSON:
		return &jsonPrinter{w: w, quiet: quiet}
	default:
		return &textPrinter{w: w, quiet: quiet, noColor: noColor}
	}
}

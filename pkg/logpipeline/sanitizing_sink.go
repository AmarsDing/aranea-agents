package logpipeline

import (
	"aranea-agents/internal/tools/preview"
)

// SanitizingSink wraps another Sink and redacts secrets from log messages
// and string fields before writing. This prevents API keys and other
// sensitive data from leaking into log files, stdout, or event bus.
//
// The sanitizer uses preview.RedactAndTruncate which covers:
// - API key patterns (OpenAI sk-, xAI xai-, AWS AKIA, GitHub ghp_, Google AIza)
// - Authorization: Bearer tokens
// - JWT tokens (eyJ...)
// - Key-value secret assignments (api_key=..., token=...)
// - Email addresses and phone numbers
type SanitizingSink struct {
	base Sink
}

// NewSanitizingSink creates a SanitizingSink wrapping the given base sink.
func NewSanitizingSink(base Sink) *SanitizingSink {
	return &SanitizingSink{base: base}
}

// Write sanitizes the entry then delegates to the base sink.
func (s *SanitizingSink) Write(entry LogEntry) {
	entry.Message = preview.RedactAndTruncate(entry.Message, 0)
	for k, v := range entry.Fields {
		if str, ok := v.(string); ok {
			entry.Fields[k] = preview.RedactAndTruncate(str, 0)
		}
	}
	s.base.Write(entry)
}

// Flush delegates to the base sink.
func (s *SanitizingSink) Flush() {
	s.base.Flush()
}

// Close delegates to the base sink.
func (s *SanitizingSink) Close() error {
	return s.base.Close()
}

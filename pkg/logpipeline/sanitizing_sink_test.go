package logpipeline

import (
	"strings"
	"testing"
	"time"
)

// fakeSink records written entries for verification.
type fakeSink struct {
	entries []LogEntry
	flushed bool
	closed  bool
}

func (f *fakeSink) Write(entry LogEntry) {
	f.entries = append(f.entries, entry)
}
func (f *fakeSink) Flush()      { f.flushed = true }
func (f *fakeSink) Close() error { f.closed = true; return nil }

func TestSanitizingSink_RedactsMessage(t *testing.T) {
	base := &fakeSink{}
	sink := NewSanitizingSink(base)

	sink.Write(LogEntry{
		Kind:      KindLog,
		Level:     "info",
		Message:   "calling api with key sk-abc123def456ghi789",
		Timestamp: time.Now(),
	})

	if len(base.entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(base.entries))
	}
	entry := base.entries[0]
	if entry.Message == "calling api with key sk-abc123def456ghi789" {
		t.Errorf("message not redacted: %q", entry.Message)
	}
	if !containsStr(entry.Message, "[secret redacted]") {
		t.Errorf("expected [secret redacted] in message, got: %q", entry.Message)
	}
}

func TestSanitizingSink_RedactsStringFields(t *testing.T) {
	base := &fakeSink{}
	sink := NewSanitizingSink(base)

	sink.Write(LogEntry{
		Kind:   KindLog,
		Level:  "info",
		Message: "tool result",
		Fields: map[string]any{
			"api_key": "sk-abc123def456ghi789",
			"status":  "ok",
			"count":   42,
		},
		Timestamp: time.Now(),
	})

	if len(base.entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(base.entries))
	}
	entry := base.entries[0]
	apiKey, ok := entry.Fields["api_key"].(string)
	if !ok {
		t.Fatalf("api_key field not a string: %v", entry.Fields["api_key"])
	}
	if apiKey == "sk-abc123def456ghi789" {
		t.Errorf("api_key field not redacted: %q", apiKey)
	}
	// Non-string fields should pass through unchanged.
	if entry.Fields["count"] != 42 {
		t.Errorf("non-string field was modified: %v", entry.Fields["count"])
	}
}

func TestSanitizingSink_PassesCleanEntriesThrough(t *testing.T) {
	base := &fakeSink{}
	sink := NewSanitizingSink(base)

	sink.Write(LogEntry{
		Kind:      KindLog,
		Level:     "info",
		Message:   "normal log message without secrets",
		Timestamp: time.Now(),
	})

	if len(base.entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(base.entries))
	}
	entry := base.entries[0]
	if entry.Message != "normal log message without secrets" {
		t.Errorf("clean message was modified: %q", entry.Message)
	}
}

func TestSanitizingSink_FlushAndClose(t *testing.T) {
	base := &fakeSink{}
	sink := NewSanitizingSink(base)

	sink.Flush()
	if !base.flushed {
		t.Error("Flush not delegated")
	}
	sink.Close()
	if !base.closed {
		t.Error("Close not delegated")
	}
}

func containsStr(s, substr string) bool {
	return strings.Contains(s, substr)
}

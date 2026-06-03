package logpipeline

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"
)

type publishCall struct {
	kind      EntryKind
	level     string
	message   string
	sessionID string
	fields    map[string]any
}

type mockPublisher struct {
	mu    sync.Mutex
	calls []publishCall
	delay time.Duration
}

func newMockPublisher() *mockPublisher {
	return &mockPublisher{}
}

func (p *mockPublisher) Publish(ctx context.Context, kind EntryKind, level, message, sessionID string, fields map[string]any) {
	if p.delay > 0 {
		select {
		case <-time.After(p.delay):
		case <-ctx.Done():
			return
		}
	}
	p.mu.Lock()
	p.calls = append(p.calls, publishCall{kind, level, message, sessionID, fields})
	p.mu.Unlock()
}

func (p *mockPublisher) Calls() []publishCall {
	p.mu.Lock()
	defer p.mu.Unlock()
	cp := make([]publishCall, len(p.calls))
	copy(cp, p.calls)
	return cp
}

func TestFileSink_Write(t *testing.T) {
	dir := t.TempDir()
	cfg := FileSinkConfig{
		OutputDir: dir,
		Filename:  "test.log",
	}
	sink := NewFileSink(cfg)
	defer sink.Close()

	entry := LogEntry{
		Kind:      KindLog,
		Level:     "info",
		Message:   "file-sink-test",
		SessionID: "sess-1",
		Timestamp: time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC),
		Fields:    map[string]any{"key": "val"},
	}
	sink.Write(entry)
	sink.Flush()

	data, err := os.ReadFile(filepath.Join(dir, "test.log"))
	if err != nil {
		t.Fatalf("failed to read log file: %v", err)
	}

	// FileSink now uses zapcore JSON encoder, parse the output
	var got map[string]any
	if err := json.Unmarshal(data, &got); err != nil {
		t.Fatalf("failed to unmarshal log entry: %v\nraw: %s", err, string(data))
	}
	if got["message"] != "file-sink-test" {
		t.Fatalf("expected message 'file-sink-test', got %v", got["message"])
	}
	if got["kind"] != "log" {
		t.Fatalf("expected kind 'log', got %v", got["kind"])
	}
	if got["session_id"] != "sess-1" {
		t.Fatalf("expected session_id 'sess-1', got %v", got["session_id"])
	}
}

func TestFileSink_Dropped(t *testing.T) {
	dir := t.TempDir()
	cfg := FileSinkConfig{
		OutputDir: dir,
		Filename:  "dropped.log",
		MaxSizeMB: 1,
	}
	sink := NewFileSink(cfg)
	defer sink.Close()

	bigEntry := LogEntry{
		Kind:    KindLog,
		Level:   "info",
		Message: strings.Repeat("x", 2*1024*1024),
	}
	sink.Write(bigEntry)

	if sink.Dropped() != 1 {
		t.Fatalf("expected 1 dropped, got %d", sink.Dropped())
	}
}

func TestFileSink_Close(t *testing.T) {
	dir := t.TempDir()
	cfg := FileSinkConfig{
		OutputDir: dir,
		Filename:  "close.log",
	}
	sink := NewFileSink(cfg)

	sink.Write(LogEntry{Kind: KindLog, Level: "info", Message: "before-close"})

	if err := sink.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}
}

func TestStdoutSink_LevelAllowed(t *testing.T) {
	tests := []struct {
		sinkLevel  string
		entryLevel string
		allowed    bool
	}{
		{"error", "error", true},
		{"error", "warn", false},
		{"error", "info", false},
		{"error", "debug", false},
		{"warn", "error", true},
		{"warn", "warn", true},
		{"warn", "info", false},
		{"info", "error", true},
		{"info", "warn", true},
		{"info", "info", true},
		{"info", "debug", false},
		{"debug", "error", true},
		{"debug", "warn", true},
		{"debug", "info", true},
		{"debug", "debug", true},
		{"", "debug", true},
		{"unknown", "info", true},
	}

	for _, tt := range tests {
		sink := NewStdoutSink(tt.sinkLevel)
		got := sink.levelAllowed(tt.entryLevel)
		if got != tt.allowed {
			t.Errorf("levelAllowed(sink=%q, entry=%q) = %v, want %v",
				tt.sinkLevel, tt.entryLevel, got, tt.allowed)
		}
	}
}

func TestEventBusSink_LevelFilter(t *testing.T) {
	pub := newMockPublisher()
	sink := NewEventBusSink(pub, "warn")

	levels := []string{"debug", "info", "warn", "error"}
	for _, lvl := range levels {
		sink.Write(LogEntry{
			Kind:    KindLog,
			Level:   lvl,
			Message: "msg-" + lvl,
		})
	}

	calls := pub.Calls()
	if len(calls) != 2 {
		t.Fatalf("expected 2 calls (warn+error), got %d", len(calls))
	}

	gotLevels := make([]string, len(calls))
	for i, c := range calls {
		gotLevels[i] = c.level
	}
	if gotLevels[0] != "warn" || gotLevels[1] != "error" {
		t.Fatalf("expected [warn error], got %v", gotLevels)
	}
}

func TestEventBusSink_Timeout(t *testing.T) {
	pub := &mockPublisher{delay: 200 * time.Millisecond}
	sink := NewEventBusSink(pub, "info")

	start := time.Now()
	sink.Write(LogEntry{
		Kind:    KindLog,
		Level:   "info",
		Message: "timeout-test",
	})
	elapsed := time.Since(start)

	if elapsed > 150*time.Millisecond {
		t.Fatalf("Write took %v, expected ~50ms timeout", elapsed)
	}

	calls := pub.Calls()
	if len(calls) != 0 {
		t.Fatalf("expected 0 calls due to timeout, got %d", len(calls))
	}
}

func TestEventBusSink_NilPublisher(t *testing.T) {
	sink := NewEventBusSink(nil, "info")

	sink.Write(LogEntry{
		Kind:    KindLog,
		Level:   "info",
		Message: "nil-pub",
	})
}

func TestMockSink_BasicUsage(t *testing.T) {
	sink := newMockSink()

	entry := LogEntry{Kind: KindLog, Level: "info", Message: "mock-test"}
	sink.Write(entry)

	entries := sink.Entries()
	if len(entries) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(entries))
	}
	if entries[0].Message != "mock-test" {
		t.Fatalf("expected message 'mock-test', got %q", entries[0].Message)
	}

	sink.Flush()
	if sink.flushCount != 1 {
		t.Fatalf("expected 1 flush, got %d", sink.flushCount)
	}

	if err := sink.Close(); err != nil {
		t.Fatalf("Close returned error: %v", err)
	}
}

func TestFileSink_DefaultConfig(t *testing.T) {
	dir := t.TempDir()
	t.Setenv("MONITOR_FLOW_LOG_DIR", dir)

	cfg := FileSinkConfig{}
	sink := NewFileSink(cfg)
	defer sink.Close()

	sink.Write(LogEntry{Kind: KindLog, Level: "info", Message: "default-config"})

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("failed to read dir: %v", err)
	}
	if len(entries) == 0 {
		t.Fatal("expected log file to be created in default dir")
	}

	found := false
	for _, e := range entries {
		if strings.HasPrefix(e.Name(), "aranea-pipeline") {
			found = true
			break
		}
	}
	if !found {
		t.Fatal("expected aranea-pipeline.log file")
	}
}

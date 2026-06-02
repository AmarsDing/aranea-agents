package loggateway

import (
	"os"
	"path/filepath"
	"sync"
	"testing"
	"time"

	"aranea-agents/internal/conf"
	"aranea-agents/pkg/logpipeline"

	"go.uber.org/zap/zapcore"
)

func newTestLogging(t *testing.T) *conf.Logging {
	t.Helper()
	dir, err := os.MkdirTemp("", "loggateway-test-*")
	if err != nil {
		t.Fatal(err)
	}
	return &conf.Logging{
		Level:         "debug",
		OutputDir:     dir,
		MaxSizeMb:     10,
		MaxBackups:    2,
		MaxAgeDays:    1,
		Compress:      false,
		StdoutEnabled: false,
	}
}

func TestNew(t *testing.T) {
	cfg := newTestLogging(t)
	defer os.RemoveAll(cfg.OutputDir)

	g := New(cfg)
	if g == nil {
		t.Fatal("New() returned nil")
	}
	if g.OutputDir() == "" {
		t.Error("OutputDir() is empty")
	}
}

func TestNewCreatesLogFile(t *testing.T) {
	cfg := newTestLogging(t)
	defer os.RemoveAll(cfg.OutputDir)

	g := New(cfg)
	if g == nil {
		t.Fatal("New() returned nil")
	}
	g.Info("test-message")
	_ = g.logger.Sync()

	entries, err := os.ReadDir(cfg.OutputDir)
	if err != nil {
		t.Fatalf("read output dir: %v", err)
	}
	if len(entries) == 0 {
		t.Error("expected log file to be created")
	}
}

func TestNewInvalidOutputDir(t *testing.T) {
	cfg := &conf.Logging{
		Level:     "info",
		OutputDir: filepath.Join(string(os.PathSeparator), "nonexistent", "deep", "path", "that", "cannot", "be", "created"),
	}
	origGlobal := Global()
	defer SetGlobal(origGlobal)

	g := New(cfg)
	if g == nil {
		t.Fatal("New() with invalid dir should return noop, not nil")
	}
}

func TestNewNoop(t *testing.T) {
	g := NewNoop()
	if g == nil {
		t.Fatal("NewNoop() returned nil")
	}
	g.Debug("noop-debug")
	g.Info("noop-info")
	g.Warn("noop-warn")
	g.Error("noop-error")
}

func TestLogMethodsNoPanic(t *testing.T) {
	g := NewNoop()

	tests := []struct {
		name string
		fn   func()
	}{
		{"Debug", func() { g.Debug("debug-msg", Str("k", "v")) }},
		{"Info", func() { g.Info("info-msg", Str("k", "v")) }},
		{"Warn", func() { g.Warn("warn-msg", Str("k", "v")) }},
		{"Error", func() { g.Error("error-msg", Str("k", "v")) }},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.fn()
		})
	}
}

func TestLogMethodsWithRealGateway(t *testing.T) {
	cfg := newTestLogging(t)
	defer os.RemoveAll(cfg.OutputDir)

	g := New(cfg)
	g.Debug("debug-msg", Str("k", "v"))
	g.Info("info-msg", Str("k", "v"))
	g.Warn("warn-msg", Str("k", "v"))
	g.Error("error-msg", Str("k", "v"))
}

func TestWith(t *testing.T) {
	g := NewNoop()

	logger := g.With(Str("base1", "v1"))
	if logger == nil {
		t.Fatal("With() returned nil")
	}

	logger.Info("with-msg", Str("extra", "v2"))
	logger.Debug("with-debug")
	logger.Warn("with-warn")
	logger.Error("with-error")
}

func TestWithChained(t *testing.T) {
	g := NewNoop()

	l1 := g.With(Str("a", "1"))
	l2 := l1.With(Str("b", "2"))
	if l2 == nil {
		t.Fatal("chained With() returned nil")
	}
	l2.Info("chained-msg")
}

func TestNilGateway(t *testing.T) {
	var g *Gateway

	tests := []struct {
		name string
		fn   func()
	}{
		{"Debug", func() { g.Debug("msg") }},
		{"Info", func() { g.Info("msg") }},
		{"Warn", func() { g.Warn("msg") }},
		{"Error", func() { g.Error("msg") }},
		{"With", func() {
			l := g.With(Str("k", "v"))
			if l == nil {
				t.Error("nil Gateway With() returned nil")
			}
		}},
		{"SetLevel", func() { g.SetLevel("debug") }},
		{"SetPipeline", func() { g.SetPipeline(nil) }},
		{"OutputDir", func() {
			dir := g.OutputDir()
			if dir == "" {
				t.Error("nil Gateway OutputDir() returned empty")
			}
		}},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			tt.fn()
		})
	}
}

func TestNoopLogger(t *testing.T) {
	var l noopLogger

	l.Debug("msg", Str("k", "v"))
	l.Info("msg", Str("k", "v"))
	l.Warn("msg", Str("k", "v"))
	l.Error("msg", Str("k", "v"))

	result := l.With(Str("k", "v"))
	if result == nil {
		t.Error("noopLogger.With() returned nil")
	}
}

func TestNoopLoggerImplementsInterface(t *testing.T) {
	var _ Logger = noopLogger{}
}

func TestLoggerWithImplementsInterface(t *testing.T) {
	g := NewNoop()
	var _ Logger = &loggerWith{g: g, base: nil}
}

func TestGlobalSetGlobal(t *testing.T) {
	orig := Global()
	defer SetGlobal(orig)

	g := NewNoop()
	SetGlobal(g)
	got := Global()
	if got != g {
		t.Error("Global() did not return the gateway set by SetGlobal()")
	}
}

func TestGlobalConcurrent(t *testing.T) {
	orig := Global()
	defer SetGlobal(orig)

	g := NewNoop()

	var wg sync.WaitGroup
	for i := 0; i < 10; i++ {
		wg.Add(2)
		go func() {
			defer wg.Done()
			SetGlobal(g)
		}()
		go func() {
			defer wg.Done()
			_ = Global()
		}()
	}
	wg.Wait()
}

func TestSetLevel(t *testing.T) {
	g := NewNoop()

	levels := []string{"debug", "info", "warn", "error", "unknown"}
	for _, lvl := range levels {
		g.SetLevel(lvl)
	}

	g.SetLevel("debug")
	g.Debug("after-setlevel")
}

func TestSetLevelNilGateway(t *testing.T) {
	var g *Gateway
	g.SetLevel("info")
}

func TestParseLevel(t *testing.T) {
	tests := []struct {
		input    string
		expected zapcore.Level
	}{
		{"debug", zapcore.DebugLevel},
		{"info", zapcore.InfoLevel},
		{"warn", zapcore.WarnLevel},
		{"error", zapcore.ErrorLevel},
		{"", zapcore.InfoLevel},
		{"unknown", zapcore.InfoLevel},
		{"DEBUG", zapcore.InfoLevel},
	}
	for _, tt := range tests {
		t.Run(tt.input, func(t *testing.T) {
			got := parseLevel(tt.input, zapcore.InfoLevel)
			if got != tt.expected {
				t.Errorf("parseLevel(%q, InfoLevel) = %v, want %v", tt.input, got, tt.expected)
			}
		})
	}
}

func TestWithBase(t *testing.T) {
	g := NewNoop()

	gw := g.With(Str("base_key", "base_val"))
	lw, ok := gw.(*loggerWith)
	if !ok {
		t.Fatal("With() did not return *loggerWith")
	}
	if len(lw.base) != 1 {
		t.Fatalf("loggerWith.base len = %d, want 1", len(lw.base))
	}

	gw2 := gw.With(Str("extra_key", "extra_val"))
	lw2, ok := gw2.(*loggerWith)
	if !ok {
		t.Fatal("chained With() did not return *loggerWith")
	}
	if len(lw2.base) != 2 {
		t.Fatalf("chained loggerWith.base len = %d, want 2", len(lw2.base))
	}
}

func TestWithBaseEmptyGateway(t *testing.T) {
	g := NewNoop()

	result := g.withBase([]Field{Str("a", "1")})
	if len(result) != 1 {
		t.Errorf("withBase with empty base returned %d fields, want 1", len(result))
	}
}

func TestWithBaseNilGateway(t *testing.T) {
	var g *Gateway
	result := g.withBase([]Field{Str("a", "1")})
	if len(result) != 1 {
		t.Errorf("nil Gateway withBase returned %d fields, want 1", len(result))
	}
}

type mockSink struct {
	mu      sync.Mutex
	entries []logpipeline.LogEntry
}

func (s *mockSink) Write(entry logpipeline.LogEntry) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.entries = append(s.entries, entry)
}

func (s *mockSink) Flush() {}

func (s *mockSink) Close() error { return nil }

func (s *mockSink) getEntries() []logpipeline.LogEntry {
	s.mu.Lock()
	defer s.mu.Unlock()
	out := make([]logpipeline.LogEntry, len(s.entries))
	copy(out, s.entries)
	return out
}

func TestEmitToPipeline(t *testing.T) {
	g := NewNoop()

	p := logpipeline.NewPipeline(64)
	defer p.Close()

	sink := &mockSink{}
	p.AddSink(sink)

	g.SetPipeline(p)
	g.Info("pipeline-msg", Str("key", "value"), SessionID("sess-1"))

	time.Sleep(200 * time.Millisecond)

	entries := sink.getEntries()
	if len(entries) == 0 {
		t.Fatal("expected at least one log entry in pipeline sink")
	}

	e := entries[len(entries)-1]
	if e.Message != "pipeline-msg" {
		t.Errorf("entry.Message = %q, want %q", e.Message, "pipeline-msg")
	}
	if e.Kind != logpipeline.KindLog {
		t.Errorf("entry.Kind = %q, want %q", e.Kind, logpipeline.KindLog)
	}
	if e.Level != "info" {
		t.Errorf("entry.Level = %q, want %q", e.Level, "info")
	}
	if e.SessionID != "sess-1" {
		t.Errorf("entry.SessionID = %q, want %q", e.SessionID, "sess-1")
	}
}

func TestEmitToPipelineWithStepID(t *testing.T) {
	g := NewNoop()

	p := logpipeline.NewPipeline(64)
	defer p.Close()

	sink := &mockSink{}
	p.AddSink(sink)

	g.SetPipeline(p)
	g.Info("step-msg", StepID("step-1"), SessionID("sess-2"))

	time.Sleep(200 * time.Millisecond)

	entries := sink.getEntries()
	if len(entries) == 0 {
		t.Fatal("expected at least one log entry in pipeline sink")
	}

	e := entries[len(entries)-1]
	if e.StepID != "step-1" {
		t.Errorf("entry.StepID = %q, want %q", e.StepID, "step-1")
	}
	if e.SessionID != "sess-2" {
		t.Errorf("entry.SessionID = %q, want %q", e.SessionID, "sess-2")
	}
}

func TestEmitToPipelineNoPipeline(t *testing.T) {
	g := NewNoop()
	g.Info("no-pipeline-msg")
}

func TestEmitToPipelineNilPipeline(t *testing.T) {
	g := NewNoop()
	g.SetPipeline(nil)
	g.Info("nil-pipeline-msg")
}

func TestEmitToPipelineWithLoggerWith(t *testing.T) {
	g := NewNoop()

	p := logpipeline.NewPipeline(64)
	defer p.Close()

	sink := &mockSink{}
	p.AddSink(sink)

	g.SetPipeline(p)

	l := g.With(SessionID("sess-3"), StepID("step-3"))
	l.Info("loggerwith-msg", Str("extra", "val"))

	time.Sleep(200 * time.Millisecond)

	entries := sink.getEntries()
	if len(entries) == 0 {
		t.Fatal("expected at least one log entry in pipeline sink")
	}

	e := entries[len(entries)-1]
	if e.SessionID != "sess-3" {
		t.Errorf("entry.SessionID = %q, want %q", e.SessionID, "sess-3")
	}
	if e.StepID != "step-3" {
		t.Errorf("entry.StepID = %q, want %q", e.StepID, "step-3")
	}
}

func TestOutputDir(t *testing.T) {
	cfg := newTestLogging(t)
	defer os.RemoveAll(cfg.OutputDir)

	g := New(cfg)
	dir := g.OutputDir()
	if dir != cfg.OutputDir {
		t.Errorf("OutputDir() = %q, want %q", dir, cfg.OutputDir)
	}
}

func TestDefaultOutputDir(t *testing.T) {
	dir := defaultOutputDir()
	if dir == "" {
		t.Error("defaultOutputDir() returned empty string")
	}
}

func TestNewWithStdout(t *testing.T) {
	cfg := newTestLogging(t)
	defer os.RemoveAll(cfg.OutputDir)

	cfg.StdoutEnabled = true
	g := New(cfg)
	if g == nil {
		t.Fatal("New() with stdout returned nil")
	}
	g.Info("stdout-msg")
}

func TestFieldConstructors(t *testing.T) {
	g := NewNoop()

	g.Debug("fields",
		StepID("s1"),
		SessionID("sess1"),
		TraceID("t1"),
		RunID("r1"),
		Domain("d1"),
		AgentKey("ak1"),
		Phase("p1"),
		Duration(100),
		Source("src1"),
		Err(os.ErrNotExist),
		Str("k", "v"),
		Int("n", 42),
		Int64("n64", 64),
		Float64("f", 3.14),
		Bool("b", true),
		Any("a", map[string]string{"x": "y"}),
	)
}

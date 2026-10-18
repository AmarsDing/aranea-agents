package adapter

import (
	"context"
	"errors"
	"log/slog"
	"testing"
	"time"

	"aranea-agents/pkg/loggateway"
)

type capturedCall struct {
	level  string
	msg    string
	fields []loggateway.Field
}

type slogCaptureLogger struct {
	calls []capturedCall
}

func (c *slogCaptureLogger) record(level, msg string, fields []loggateway.Field) {
	c.calls = append(c.calls, capturedCall{level, msg, fields})
}

func (c *slogCaptureLogger) Debug(msg string, f ...loggateway.Field) { c.record("debug", msg, f) }
func (c *slogCaptureLogger) Info(msg string, f ...loggateway.Field)  { c.record("info", msg, f) }
func (c *slogCaptureLogger) Warn(msg string, f ...loggateway.Field)  { c.record("warn", msg, f) }
func (c *slogCaptureLogger) Error(msg string, f ...loggateway.Field) { c.record("error", msg, f) }
func (c *slogCaptureLogger) With(f ...loggateway.Field) loggateway.Logger {
	c.record("with", "", f)
	return c
}

var _ loggateway.Logger = (*slogCaptureLogger)(nil)

// TestSlogBridge_LevelAndAttrs checks the audit P3-1 bridge: slog records are
// forwarded at the mapped level with attrs converted to structured fields.
func TestSlogBridge_LevelAndAttrs(t *testing.T) {
	lg := &slogCaptureLogger{}
	h := NewSlogBridge(lg)
	var _ slog.Handler = h

	rec := slog.NewRecord(time.Now(), slog.LevelWarn, "parser skipped", 0)
	rec.AddAttrs(slog.String("file_type", "go"), slog.Int("files", 3))
	if err := h.Handle(context.Background(), rec); err != nil {
		t.Fatalf("Handle failed: %v", err)
	}

	if len(lg.calls) != 1 {
		t.Fatalf("expected 1 forwarded record, got %d", len(lg.calls))
	}
	call := lg.calls[0]
	if call.level != "warn" || call.msg != "parser skipped" {
		t.Fatalf("unexpected forward: %+v", call)
	}
	if len(call.fields) != 2 {
		t.Fatalf("expected 2 structured fields, got %d", len(call.fields))
	}
	if call.fields[0].Key != "file_type" || call.fields[1].Key != "files" {
		t.Fatalf("field keys lost: %v %v", call.fields[0].Key, call.fields[1].Key)
	}

	// Error values must route through loggateway.Err (error chain unwrapping).
	rec2 := slog.NewRecord(time.Now(), slog.LevelError, "read failed", 0)
	rec2.AddAttrs(slog.Any("err", errors.New("disk gone")))
	_ = h.Handle(context.Background(), rec2)
	if lg.calls[1].level != "error" {
		t.Fatalf("expected error level, got %s", lg.calls[1].level)
	}
}

// TestSlogBridge_GroupFlattening checks WithGroup flattens into dotted keys.
func TestSlogBridge_GroupFlattening(t *testing.T) {
	lg := &slogCaptureLogger{}
	h := NewSlogBridge(lg).WithGroup("source")
	rec := slog.NewRecord(time.Now(), slog.LevelInfo, "m", 0)
	rec.AddAttrs(slog.String("lang", "py"))
	_ = h.Handle(context.Background(), rec)
	if lg.calls[0].fields[0].Key != "source.lang" {
		t.Fatalf("group not flattened: %v", lg.calls[0].fields[0].Key)
	}
}

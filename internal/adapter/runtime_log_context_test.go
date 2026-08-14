package adapter

import (
	"context"
	"testing"

	"aranea-agents/pkg/loggateway"

	"go.opentelemetry.io/otel/trace"
	a2alog "trpc.group/trpc-go/trpc-a2a-go/log"
	agentlog "trpc.group/trpc-go/trpc-agent-go/log"
)

// captureLogger is a loggateway.Logger that records messages and fields into
// storage shared across With() children.
type captureLogger struct {
	msgs   *[]string
	fields *[]loggateway.Field
}

func newCaptureLogger() *captureLogger {
	return &captureLogger{msgs: &[]string{}, fields: &[]loggateway.Field{}}
}

func (c *captureLogger) Debug(msg string, fields ...loggateway.Field) { c.record(msg, fields) }
func (c *captureLogger) Info(msg string, fields ...loggateway.Field)  { c.record(msg, fields) }
func (c *captureLogger) Warn(msg string, fields ...loggateway.Field)  { c.record(msg, fields) }
func (c *captureLogger) Error(msg string, fields ...loggateway.Field) { c.record(msg, fields) }

func (c *captureLogger) record(msg string, fields []loggateway.Field) {
	*c.msgs = append(*c.msgs, msg)
	*c.fields = append(*c.fields, fields...)
}

func (c *captureLogger) With(fields ...loggateway.Field) loggateway.Logger {
	*c.fields = append(*c.fields, fields...)
	return &captureLogger{msgs: c.msgs, fields: c.fields}
}

func (c *captureLogger) fieldValue(key string) (string, bool) {
	for _, f := range *c.fields {
		if f.Key == key {
			return f.String, true
		}
	}
	return "", false
}

// saveAgentlogVars snapshots the agentlog context-function variables so a test
// can restore them afterwards (they are process-global).
func saveAgentlogVars(t *testing.T) func() {
	t.Helper()
	orig := []struct {
		dst *func(context.Context, ...any)
		val func(context.Context, ...any)
	}{
		{&agentlog.DebugContext, agentlog.DebugContext},
		{&agentlog.InfoContext, agentlog.InfoContext},
		{&agentlog.WarnContext, agentlog.WarnContext},
		{&agentlog.ErrorContext, agentlog.ErrorContext},
		{&agentlog.FatalContext, agentlog.FatalContext},
	}
	origF := []struct {
		dst *func(context.Context, string, ...any)
		val func(context.Context, string, ...any)
	}{
		{&agentlog.DebugfContext, agentlog.DebugfContext},
		{&agentlog.InfofContext, agentlog.InfofContext},
		{&agentlog.WarnfContext, agentlog.WarnfContext},
		{&agentlog.ErrorfContext, agentlog.ErrorfContext},
		{&agentlog.FatalfContext, agentlog.FatalfContext},
		{&agentlog.TracefContext, agentlog.TracefContext},
	}
	origA2A := a2alog.Default
	return func() {
		for _, o := range orig {
			*o.dst = o.val
		}
		for _, o := range origF {
			*o.dst = o.val
		}
		a2alog.Default = origA2A
	}
}

func spanCtx() context.Context {
	sc := trace.NewSpanContext(trace.SpanContextConfig{
		TraceID: trace.TraceID{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09, 0x0a, 0x0b, 0x0c, 0x0d, 0x0e, 0x0f, 0x10},
		SpanID:  trace.SpanID{0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17, 0x18},
	})
	return trace.ContextWithSpanContext(context.Background(), sc)
}

func TestInstallRuntimeLogContextFuncs_InjectsTraceFields(t *testing.T) {
	defer saveAgentlogVars(t)()
	cl := newCaptureLogger()
	InstallRuntimeLogContextFuncs(NewRuntimeLogAdapter(cl))

	agentlog.InfoContext(spanCtx(), "hello")

	if len(*cl.msgs) != 1 || (*cl.msgs)[0] != "hello" {
		t.Fatalf("msgs = %v, want [hello]", *cl.msgs)
	}
	tid, ok := cl.fieldValue("trace_id")
	if !ok || tid != "0102030405060708090a0b0c0d0e0f10" {
		t.Fatalf("trace_id = %q (ok=%v)", tid, ok)
	}
	sid, ok := cl.fieldValue("span_id")
	if !ok || sid != "1112131415161718" {
		t.Fatalf("span_id = %q (ok=%v)", sid, ok)
	}
}

func TestInstallRuntimeLogContextFuncs_NoSpanNoTraceFields(t *testing.T) {
	defer saveAgentlogVars(t)()
	cl := newCaptureLogger()
	InstallRuntimeLogContextFuncs(NewRuntimeLogAdapter(cl))

	agentlog.InfoContext(context.Background(), "plain")

	if len(*cl.msgs) != 1 || (*cl.msgs)[0] != "plain" {
		t.Fatalf("msgs = %v, want [plain]", *cl.msgs)
	}
	if _, ok := cl.fieldValue("trace_id"); ok {
		t.Fatalf("unexpected trace_id in fields: %v", *cl.fields)
	}
}

func TestInstallRuntimeLogContextFuncs_FormatsAndBridgesA2A(t *testing.T) {
	defer saveAgentlogVars(t)()
	cl := newCaptureLogger()
	rla := NewRuntimeLogAdapter(cl)
	InstallRuntimeLogContextFuncs(rla)

	agentlog.ErrorfContext(spanCtx(), "boom %d", 42)
	if len(*cl.msgs) != 1 || (*cl.msgs)[0] != "boom 42" {
		t.Fatalf("msgs = %v, want [boom 42]", *cl.msgs)
	}

	if a2alog.Default != agentlog.Logger(rla) {
		t.Fatalf("a2a log.Default not bridged to RuntimeLogAdapter")
	}
}

func TestInstallRuntimeLogContextFuncs_TracefDisabledByDefault(t *testing.T) {
	defer saveAgentlogVars(t)()
	cl := newCaptureLogger()
	InstallRuntimeLogContextFuncs(NewRuntimeLogAdapter(cl))

	agentlog.TracefContext(spanCtx(), "noisy %s", "x")
	if len(*cl.msgs) != 0 {
		t.Fatalf("msgs = %v, want none (trace disabled)", *cl.msgs)
	}
}

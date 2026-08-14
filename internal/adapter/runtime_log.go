package adapter

import (
	"context"
	"fmt"
	"os"

	loggateway "aranea-agents/pkg/loggateway"

	"go.opentelemetry.io/otel/trace"
	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	a2alog "trpc.group/trpc-go/trpc-a2a-go/log"
	agentlog "trpc.group/trpc-go/trpc-agent-go/log"
)

// RuntimeLogAdapter implements agentlog.Logger by delegating to a
// loggateway.Logger. It bridges trpc-agent-go runtime logs into the
// loggateway Pipeline.
//
// Fatal/Fatalf are special: they write synchronously to stderr and a
// dedicated zap.Logger, then call os.Exit(1) — they do NOT go through
// the async Pipeline.
type RuntimeLogAdapter struct {
	lg    loggateway.Logger
	base  []loggateway.Field
	fatal *zap.SugaredLogger
}

// NewRuntimeLogAdapter creates a RuntimeLogAdapter wrapping the given
// loggateway.Logger.
func NewRuntimeLogAdapter(lg loggateway.Logger) *RuntimeLogAdapter {
	fatalCore := zapcore.NewCore(
		zapcore.NewConsoleEncoder(zapcore.EncoderConfig{
			TimeKey:        "ts",
			LevelKey:       "level",
			MessageKey:     "msg",
			StacktraceKey:  "stacktrace",
			LineEnding:     zapcore.DefaultLineEnding,
			EncodeLevel:    zapcore.CapitalLevelEncoder,
			EncodeTime:     zapcore.ISO8601TimeEncoder,
			EncodeDuration: zapcore.MillisDurationEncoder,
		}),
		zapcore.AddSync(os.Stderr),
		zapcore.FatalLevel,
	)
	fatalLogger := zap.New(fatalCore, zap.AddCaller(), zap.AddCallerSkip(2)).Sugar()

	return &RuntimeLogAdapter{
		lg:    lg,
		fatal: fatalLogger,
	}
}

// Verify interface compliance at compile time.
var _ agentlog.Logger = (*RuntimeLogAdapter)(nil)

func (a *RuntimeLogAdapter) Debug(args ...any) {
	a.lg.Debug(fmt.Sprint(args...), a.base...)
}

func (a *RuntimeLogAdapter) Debugf(format string, args ...any) {
	a.lg.Debug(fmt.Sprintf(format, args...), a.base...)
}

func (a *RuntimeLogAdapter) Info(args ...any) {
	a.lg.Info(fmt.Sprint(args...), a.base...)
}

func (a *RuntimeLogAdapter) Infof(format string, args ...any) {
	a.lg.Info(fmt.Sprintf(format, args...), a.base...)
}

func (a *RuntimeLogAdapter) Warn(args ...any) {
	a.lg.Warn(fmt.Sprint(args...), a.base...)
}

func (a *RuntimeLogAdapter) Warnf(format string, args ...any) {
	a.lg.Warn(fmt.Sprintf(format, args...), a.base...)
}

func (a *RuntimeLogAdapter) Error(args ...any) {
	a.lg.Error(fmt.Sprint(args...), a.base...)
}

func (a *RuntimeLogAdapter) Errorf(format string, args ...any) {
	a.lg.Error(fmt.Sprintf(format, args...), a.base...)
}

func (a *RuntimeLogAdapter) Fatal(args ...any) {
	a.fatal.Fatal(args...)
}

func (a *RuntimeLogAdapter) Fatalf(format string, args ...any) {
	a.fatal.Fatalf(format, args...)
}

// With returns a new RuntimeLogAdapter with the given fields pre-set.
// The original adapter is not modified (immutable pattern).
func (a *RuntimeLogAdapter) With(fields ...loggateway.Field) *RuntimeLogAdapter {
	newBase := make([]loggateway.Field, 0, len(a.base)+len(fields))
	newBase = append(newBase, a.base...)
	newBase = append(newBase, fields...)
	return &RuntimeLogAdapter{
		lg:    a.lg,
		base:  newBase,
		fatal: a.fatal,
	}
}

// runtimeTraceFields extracts the OTel span context from ctx as structured
// fields. Returns nil when the context carries no valid span context.
func runtimeTraceFields(ctx context.Context) []loggateway.Field {
	if ctx == nil {
		return nil
	}
	sc := trace.SpanContextFromContext(ctx)
	if !sc.IsValid() {
		return nil
	}
	return []loggateway.Field{
		loggateway.TraceID(sc.TraceID().String()),
		loggateway.Str("span_id", sc.SpanID().String()),
	}
}

// InstallRuntimeLogContextFuncs replaces the agentlog *Context package-level
// function variables so context-aware framework logs are routed through the
// adapter with OTel trace_id/span_id extracted from the context (the stock
// implementations silently drop the context). It also bridges the trpc-a2a-go
// package logger, which the framework's init() had bound to the pre-replacement
// zap logger and would otherwise bypass the loggateway Pipeline.
//
// Call once during startup after agentlog.Default/ContextDefault are replaced.
func InstallRuntimeLogContextFuncs(a *RuntimeLogAdapter) {
	agentlog.DebugContext = func(ctx context.Context, args ...any) {
		a.With(runtimeTraceFields(ctx)...).Debug(args...)
	}
	agentlog.DebugfContext = func(ctx context.Context, format string, args ...any) {
		a.With(runtimeTraceFields(ctx)...).Debugf(format, args...)
	}
	agentlog.InfoContext = func(ctx context.Context, args ...any) {
		a.With(runtimeTraceFields(ctx)...).Info(args...)
	}
	agentlog.InfofContext = func(ctx context.Context, format string, args ...any) {
		a.With(runtimeTraceFields(ctx)...).Infof(format, args...)
	}
	agentlog.WarnContext = func(ctx context.Context, args ...any) {
		a.With(runtimeTraceFields(ctx)...).Warn(args...)
	}
	agentlog.WarnfContext = func(ctx context.Context, format string, args ...any) {
		a.With(runtimeTraceFields(ctx)...).Warnf(format, args...)
	}
	agentlog.ErrorContext = func(ctx context.Context, args ...any) {
		a.With(runtimeTraceFields(ctx)...).Error(args...)
	}
	agentlog.ErrorfContext = func(ctx context.Context, format string, args ...any) {
		a.With(runtimeTraceFields(ctx)...).Errorf(format, args...)
	}
	agentlog.FatalContext = func(ctx context.Context, args ...any) {
		a.With(runtimeTraceFields(ctx)...).Fatal(args...)
	}
	agentlog.FatalfContext = func(ctx context.Context, format string, args ...any) {
		a.With(runtimeTraceFields(ctx)...).Fatalf(format, args...)
	}
	agentlog.TracefContext = func(ctx context.Context, format string, args ...any) {
		if !agentlog.IsTraceEnabled() {
			return
		}
		a.With(runtimeTraceFields(ctx)...).Debugf("[TRACE] "+format, args...)
	}
	a2alog.Default = a
}

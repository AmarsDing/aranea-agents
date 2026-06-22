package loggateway

import (
	"fmt"
	"os"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"aranea-agents/pkg/logpipeline"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

// LoggingConfig is a pkg-level config struct that decouples loggateway from
// internal/conf. The caller (cmd/admin/main.go) converts conf.Logging to this.
type LoggingConfig struct {
	Level      string
	OutputDir  string
	MaxSizeMB  int
	MaxBackups int
	MaxAgeDays int
	Compress   bool
	Stdout     bool
}

type Gateway struct {
	base        []Field
	outputDir   string
	pipeline    atomic.Pointer[logpipeline.Pipeline]
	atomicLevel zap.AtomicLevel
}

var (
	globalMu sync.RWMutex
	global   = NewNoop()
)

func New(c LoggingConfig, pipeline logpipeline.Pipeline) *Gateway {
	level := parseLevel(c.Level, zapcore.InfoLevel)
	outputDir := c.OutputDir
	if outputDir == "" {
		outputDir = defaultOutputDir()
	}

	atomicLevel := zap.NewAtomicLevelAt(level)
	g := &Gateway{
		outputDir:   outputDir,
		atomicLevel: atomicLevel,
	}

	if pipeline != nil {
		g.pipeline.Store(&pipeline)
	}

	globalMu.Lock()
	global = g
	globalMu.Unlock()

	return g
}

// Deprecated: use constructor injection instead of global singleton.
func Global() *Gateway {
	globalMu.RLock()
	defer globalMu.RUnlock()
	return global
}

func SetGlobal(g *Gateway) {
	globalMu.Lock()
	defer globalMu.Unlock()
	global = g
}

func (g *Gateway) OutputDir() string {
	if g == nil {
		return defaultOutputDir()
	}
	return g.outputDir
}

func (g *Gateway) SetLevel(level string) {
	if g == nil {
		return
	}
	g.atomicLevel.SetLevel(parseLevel(level, zapcore.InfoLevel))
}

func NewNoop() *Gateway {
	return &Gateway{
		atomicLevel: zap.NewAtomicLevelAt(zapcore.DebugLevel),
	}
}

func (g *Gateway) Debug(msg string, fields ...Field) {
	if g == nil {
		return
	}
	all := g.withBase(fields)
	g.emitToPipeline(zapcore.DebugLevel, msg, all)
}

func (g *Gateway) Info(msg string, fields ...Field) {
	if g == nil {
		return
	}
	all := g.withBase(fields)
	g.emitToPipeline(zapcore.InfoLevel, msg, all)
}

func (g *Gateway) Warn(msg string, fields ...Field) {
	if g == nil {
		return
	}
	all := g.withBase(fields)
	g.emitToPipeline(zapcore.WarnLevel, msg, all)
}

func (g *Gateway) Error(msg string, fields ...Field) {
	if g == nil {
		return
	}
	all := g.withBase(fields)
	g.emitToPipeline(zapcore.ErrorLevel, msg, all)
}

func (g *Gateway) With(fields ...Field) Logger {
	if g == nil {
		return noopLogger{}
	}
	newBase := make([]Field, 0, len(g.base)+len(fields))
	newBase = append(newBase, g.base...)
	newBase = append(newBase, fields...)
	return &loggerWith{g: g, base: newBase}
}

// SetPipeline is kept for backward compatibility but should not be needed
// when Pipeline is injected at construction time via New().
func (g *Gateway) SetPipeline(p logpipeline.Pipeline) {
	if g == nil {
		return
	}
	g.pipeline.Store(&p)
}

func (g *Gateway) emitToPipeline(level zapcore.Level, msg string, fields []Field) {
	if g == nil {
		return
	}
	pp := g.pipeline.Load()
	if pp == nil || *pp == nil {
		return
	}
	func() {
		defer func() {
			if r := recover(); r != nil {
				fmt.Fprintf(os.Stderr, "[loggateway] emitToPipeline panic: %v\n", r)
			}
		}()
		enc := zapcore.NewMapObjectEncoder()
		for _, f := range fields {
			f.AddTo(enc)
		}
		enc.Fields["level"] = level.String()
		sessionID, _ := enc.Fields["session_id"].(string)
		stepID, _ := enc.Fields["step_id"].(string)
		traceID, _ := enc.Fields["trace_id"].(string)
		runID, _ := enc.Fields["run_id"].(string)
		(*pp).Emit(logpipeline.LogEntry{
			Kind:      logpipeline.KindLog,
			Level:     level.String(),
			Message:   msg,
			Fields:    enc.Fields,
			Timestamp: time.Now(),
			SessionID: sessionID,
			StepID:    stepID,
			TraceID:   traceID,
			RunID:     runID,
		})
	}()
}

func (g *Gateway) withBase(fields []Field) []Field {
	if g == nil {
		return fields
	}
	if len(g.base) == 0 {
		return fields
	}
	all := make([]Field, 0, len(g.base)+len(fields))
	all = append(all, g.base...)
	all = append(all, fields...)
	return all
}

func parseLevel(s string, fallback zapcore.Level) zapcore.Level {
	switch s {
	case "debug":
		return zapcore.DebugLevel
	case "info":
		return zapcore.InfoLevel
	case "warn":
		return zapcore.WarnLevel
	case "error":
		return zapcore.ErrorLevel
	default:
		return fallback
	}
}

func defaultOutputDir() string {
	if dir := os.Getenv("MONITOR_FLOW_LOG_DIR"); dir != "" {
		return dir
	}
	if runtime.GOOS == "windows" {
		return "./logs"
	}
	return "/var/log/aranea"
}

type EnvelopeLog struct {
	Level     string
	Message   string
	Fields    map[string]interface{}
	Timestamp time.Time
}

type loggerWith struct {
	g    *Gateway
	base []Field
}

func (l *loggerWith) Debug(msg string, fields ...Field) {
	all := make([]Field, 0, len(l.base)+len(fields))
	all = append(all, l.base...)
	all = append(all, fields...)
	l.g.emitToPipeline(zapcore.DebugLevel, msg, all)
}

func (l *loggerWith) Info(msg string, fields ...Field) {
	all := make([]Field, 0, len(l.base)+len(fields))
	all = append(all, l.base...)
	all = append(all, fields...)
	l.g.emitToPipeline(zapcore.InfoLevel, msg, all)
}

func (l *loggerWith) Warn(msg string, fields ...Field) {
	all := make([]Field, 0, len(l.base)+len(fields))
	all = append(all, l.base...)
	all = append(all, fields...)
	l.g.emitToPipeline(zapcore.WarnLevel, msg, all)
}

func (l *loggerWith) Error(msg string, fields ...Field) {
	all := make([]Field, 0, len(l.base)+len(fields))
	all = append(all, l.base...)
	all = append(all, fields...)
	l.g.emitToPipeline(zapcore.ErrorLevel, msg, all)
}

func (l *loggerWith) With(fields ...Field) Logger {
	newBase := make([]Field, 0, len(l.base)+len(fields))
	newBase = append(newBase, l.base...)
	newBase = append(newBase, fields...)
	return &loggerWith{g: l.g, base: newBase}
}

type noopLogger struct{}

func (noopLogger) Debug(string, ...Field) {}
func (noopLogger) Info(string, ...Field)  {}
func (noopLogger) Warn(string, ...Field)  {}
func (noopLogger) Error(string, ...Field) {}
func (noopLogger) With(...Field) Logger   { return noopLogger{} }

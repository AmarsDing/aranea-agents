package loggateway

import (
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"sync/atomic"
	"time"

	"aranea-agents/internal/conf"
	"aranea-agents/pkg/logpipeline"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

type Gateway struct {
	core        zapcore.Core
	logger      *zap.Logger
	sugar       *zap.SugaredLogger
	kratosAdp   *KratosAdapter
	base        []Field
	outputDir   string
	pipeline    atomic.Pointer[logpipeline.Pipeline]
	atomicLevel zap.AtomicLevel
}

var (
	globalMu sync.RWMutex
	global   = NewNoop()
)

func New(c *conf.Logging) *Gateway {
	level := parseLevel(c.GetLevel(), zapcore.InfoLevel)
	outputDir := c.GetOutputDir()
	if outputDir == "" {
		outputDir = defaultOutputDir()
	}
	os.MkdirAll(outputDir, 0755)

	maxSize := c.GetMaxSizeMb()
	if maxSize <= 0 {
		maxSize = 100
	}
	maxBackups := c.GetMaxBackups()
	if maxBackups <= 0 {
		maxBackups = 10
	}
	maxAge := c.GetMaxAgeDays()
	if maxAge <= 0 {
		maxAge = 30
	}

	lw := &lumberjack.Logger{
		Filename:   filepath.Join(outputDir, "aranea.log"),
		MaxSize:    int(maxSize),
		MaxBackups: int(maxBackups),
		MaxAge:     int(maxAge),
		Compress:   c.GetCompress(),
		LocalTime:  true,
	}

	var ws zapcore.WriteSyncer
	fileSyncer := zapcore.AddSync(lw)
	if c.GetStdoutEnabled() {
		ws = zapcore.NewMultiWriteSyncer(fileSyncer, zapcore.AddSync(os.Stdout))
	} else {
		ws = fileSyncer
	}

	encoderCfg := zapcore.EncoderConfig{
		TimeKey:        "ts",
		LevelKey:       "level",
		NameKey:        "logger",
		CallerKey:      "caller",
		MessageKey:     "msg",
		StacktraceKey:  "stacktrace",
		LineEnding:     zapcore.DefaultLineEnding,
		EncodeLevel:    zapcore.LowercaseLevelEncoder,
		EncodeTime:     zapcore.ISO8601TimeEncoder,
		EncodeDuration: zapcore.MillisDurationEncoder,
		EncodeCaller:   zapcore.ShortCallerEncoder,
	}

	atomicLevel := zap.NewAtomicLevelAt(level)
	core := zapcore.NewCore(
		zapcore.NewJSONEncoder(encoderCfg),
		ws,
		atomicLevel,
	)

	logger := zap.New(core, zap.AddCaller(), zap.AddCallerSkip(1))
	sugar := logger.Sugar()
	g := &Gateway{
		core:        core,
		logger:      logger,
		sugar:       sugar,
		kratosAdp:   &KratosAdapter{sugar: sugar},
		outputDir:   outputDir,
		atomicLevel: atomicLevel,
	}

	globalMu.Lock()
	global = g
	globalMu.Unlock()

	return g
}

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
	core := zapcore.NewCore(
		zapcore.NewJSONEncoder(zapcore.EncoderConfig{}),
		zapcore.AddSync(io.Discard),
		zapcore.DebugLevel,
	)
	logger := zap.New(core)
	sugar := logger.Sugar()
	return &Gateway{
		core:        core,
		logger:      logger,
		sugar:       sugar,
		kratosAdp:   &KratosAdapter{sugar: sugar},
		atomicLevel: zap.NewAtomicLevelAt(zapcore.DebugLevel),
	}
}

func (g *Gateway) Debug(msg string, fields ...Field) {
	if g == nil {
		return
	}
	all := g.withBase(fields)
	g.logger.Debug(msg, all...)
	g.emitToPipeline(zapcore.DebugLevel, msg, all)
}

func (g *Gateway) Info(msg string, fields ...Field) {
	if g == nil {
		return
	}
	all := g.withBase(fields)
	g.logger.Info(msg, all...)
	g.emitToPipeline(zapcore.InfoLevel, msg, all)
}

func (g *Gateway) Warn(msg string, fields ...Field) {
	if g == nil {
		return
	}
	all := g.withBase(fields)
	g.logger.Warn(msg, all...)
	g.emitToPipeline(zapcore.WarnLevel, msg, all)
}

func (g *Gateway) Error(msg string, fields ...Field) {
	if g == nil {
		return
	}
	all := g.withBase(fields)
	g.logger.Error(msg, all...)
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

func (g *Gateway) BeginStep(stepID, msg string, fields ...Field) *Step {
	if g == nil {
		return &Step{g: g, stepID: stepID, start: time.Now()}
	}
	all := make([]Field, 0, len(g.base)+len(fields)+2)
	all = append(all, g.base...)
	all = append(all, StepID(stepID), Phase("start"))
	all = append(all, fields...)
	g.Info(msg, all...)
	return &Step{g: g, stepID: stepID, start: time.Now()}
}

func (g *Gateway) KratosLogger(kv ...interface{}) *KratosAdapter {
	if g == nil {
		return &KratosAdapter{}
	}
	return g.kratosAdp.WithFields(kv...)
}

func (g *Gateway) ZapSugar() *zap.SugaredLogger {
	if g == nil {
		return zap.NewNop().Sugar()
	}
	return g.sugar
}

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
		defer func() { recover() }()
		enc := zapcore.NewMapObjectEncoder()
		for _, f := range fields {
			f.AddTo(enc)
		}
		enc.Fields["level"] = level.String()
		sessionID, _ := enc.Fields["session_id"].(string)
		stepID, _ := enc.Fields["step_id"].(string)
		(*pp).Emit(logpipeline.LogEntry{
			Kind:      logpipeline.KindLog,
			Level:     level.String(),
			Message:   msg,
			Fields:    enc.Fields,
			Timestamp: time.Now(),
			SessionID: sessionID,
			StepID:    stepID,
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
	l.g.logger.Debug(msg, all...)
	l.g.emitToPipeline(zapcore.DebugLevel, msg, all)
}

func (l *loggerWith) Info(msg string, fields ...Field) {
	all := make([]Field, 0, len(l.base)+len(fields))
	all = append(all, l.base...)
	all = append(all, fields...)
	l.g.logger.Info(msg, all...)
	l.g.emitToPipeline(zapcore.InfoLevel, msg, all)
}

func (l *loggerWith) Warn(msg string, fields ...Field) {
	all := make([]Field, 0, len(l.base)+len(fields))
	all = append(all, l.base...)
	all = append(all, fields...)
	l.g.logger.Warn(msg, all...)
	l.g.emitToPipeline(zapcore.WarnLevel, msg, all)
}

func (l *loggerWith) Error(msg string, fields ...Field) {
	all := make([]Field, 0, len(l.base)+len(fields))
	all = append(all, l.base...)
	all = append(all, fields...)
	l.g.logger.Error(msg, all...)
	l.g.emitToPipeline(zapcore.ErrorLevel, msg, all)
}

func (l *loggerWith) With(fields ...Field) Logger {
	newBase := make([]Field, 0, len(l.base)+len(fields))
	newBase = append(newBase, l.base...)
	newBase = append(newBase, fields...)
	return &loggerWith{g: l.g, base: newBase}
}

func (l *loggerWith) BeginStep(stepID, msg string, fields ...Field) *Step {
	all := make([]Field, 0, len(l.base)+len(fields)+2)
	all = append(all, l.base...)
	all = append(all, StepID(stepID), Phase("start"))
	all = append(all, fields...)
	l.Info(msg, all...)
	return &Step{g: l.g, stepID: stepID, start: time.Now()}
}

type noopLogger struct{}

func (noopLogger) Debug(string, ...Field) {}
func (noopLogger) Info(string, ...Field)  {}
func (noopLogger) Warn(string, ...Field)  {}
func (noopLogger) Error(string, ...Field) {}
func (noopLogger) With(...Field) Logger   { return noopLogger{} }
func (noopLogger) BeginStep(stepID, _ string, _ ...Field) *Step {
	return &Step{stepID: stepID, start: time.Now()}
}

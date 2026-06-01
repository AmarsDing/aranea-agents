package loggateway

import (
	"io"
	"os"
	"path/filepath"
	"runtime"
	"sync"
	"time"

	"aranea-agents/internal/conf"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

type Gateway struct {
	core      zapcore.Core
	logger    *zap.Logger
	sugar     *zap.SugaredLogger
	hook      *busHook
	kratosAdp *KratosAdapter
	base      []Field
	outputDir string
}

var (
	globalMu sync.RWMutex
	global   = NewNoop()
)

func New(c *conf.Logging) *Gateway {
	level := parseLevel(c.GetLevel(), zapcore.InfoLevel)
	hookLevel := parseLevel(c.GetHookLevel(), zapcore.InfoLevel)
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

	hook := &busHook{hookLevel: hookLevel}

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

	core := &hookedCore{
		Core: zapcore.NewCore(
			zapcore.NewJSONEncoder(encoderCfg),
			ws,
			level,
		),
		hook: hook,
	}

	logger := zap.New(core, zap.AddCaller(), zap.AddCallerSkip(1))
	sugar := logger.Sugar()
	g := &Gateway{
		core:      core,
		logger:    logger,
		sugar:     sugar,
		hook:      hook,
		kratosAdp: &KratosAdapter{sugar: sugar},
		outputDir: outputDir,
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

func NewNoop() *Gateway {
	core := zapcore.NewCore(
		zapcore.NewJSONEncoder(zapcore.EncoderConfig{}),
		zapcore.AddSync(io.Discard),
		zapcore.DebugLevel,
	)
	logger := zap.New(core)
	sugar := logger.Sugar()
	return &Gateway{
		core:      core,
		logger:    logger,
		sugar:     sugar,
		hook:      &busHook{},
		kratosAdp: &KratosAdapter{sugar: sugar},
	}
}

func (g *Gateway) Debug(msg string, fields ...Field) {
	if g == nil {
		return
	}
	all := g.withBase(fields)
	g.logger.Debug(msg, all...)
}

func (g *Gateway) Info(msg string, fields ...Field) {
	if g == nil {
		return
	}
	all := g.withBase(fields)
	g.logger.Info(msg, all...)
}

func (g *Gateway) Warn(msg string, fields ...Field) {
	if g == nil {
		return
	}
	all := g.withBase(fields)
	g.logger.Warn(msg, all...)
}

func (g *Gateway) Error(msg string, fields ...Field) {
	if g == nil {
		return
	}
	all := g.withBase(fields)
	g.logger.Error(msg, all...)
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

func (g *Gateway) SetBusPublish(fn func(env EnvelopeLog)) {
	if g == nil {
		return
	}
	g.hook.setPublisher(fn)
}

func (g *Gateway) SetHookLevel(level string) {
	if g == nil {
		return
	}
	g.hook.setLevel(parseLevel(level, zapcore.InfoLevel))
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

func SetHookLevel(level string) {
	if g := Global(); g != nil {
		g.SetHookLevel(level)
	}
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
	l.g.Debug(msg, l.g.withBase(append(l.base, fields...))...)
}

func (l *loggerWith) Info(msg string, fields ...Field) {
	l.g.Info(msg, l.g.withBase(append(l.base, fields...))...)
}

func (l *loggerWith) Warn(msg string, fields ...Field) {
	l.g.Warn(msg, l.g.withBase(append(l.base, fields...))...)
}

func (l *loggerWith) Error(msg string, fields ...Field) {
	l.g.Error(msg, l.g.withBase(append(l.base, fields...))...)
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

package logpipeline

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"sync/atomic"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
	"gopkg.in/natefinch/lumberjack.v2"
)

type FileSinkConfig struct {
	OutputDir  string
	MaxSizeMB  int
	MaxBackups int
	MaxAgeDays int
	Compress   bool
	Filename   string
}

type FileSink struct {
	encoder zapcore.Encoder
	lj      *lumberjack.Logger
	dropped atomic.Uint64
}

func NewFileSink(cfg FileSinkConfig) *FileSink {
	outputDir := cfg.OutputDir
	if outputDir == "" {
		outputDir = defaultOutputDir()
	}
	maxSize := cfg.MaxSizeMB
	if maxSize <= 0 {
		maxSize = 100
	}
	maxBackups := cfg.MaxBackups
	if maxBackups <= 0 {
		maxBackups = 10
	}
	maxAge := cfg.MaxAgeDays
	if maxAge <= 0 {
		maxAge = 30
	}
	filename := cfg.Filename
	if filename == "" {
		filename = "aranea-pipeline.log"
	}

	// BLK-2 fix: ensure output directory exists
	if err := os.MkdirAll(outputDir, 0o755); err != nil {
		fmt.Fprintf(os.Stderr, "[file_sink] failed to create output dir %s: %v\n", outputDir, err)
	}

	encConfig := zap.NewProductionEncoderConfig()
	encConfig.TimeKey = "ts"
	encConfig.EncodeTime = zapcore.ISO8601TimeEncoder
	encoder := zapcore.NewJSONEncoder(encConfig)

	lj := &lumberjack.Logger{
		Filename:   filepath.Join(outputDir, filename),
		MaxSize:    maxSize,
		MaxBackups: maxBackups,
		MaxAge:     maxAge,
		Compress:   cfg.Compress,
		LocalTime:  true,
	}

	return &FileSink{
		encoder: encoder,
		lj:      lj,
	}
}

func (s *FileSink) Write(entry LogEntry) {
	fields := []zap.Field{
		zap.String("kind", string(entry.Kind)),
		zap.String("level", entry.Level),
		zap.String("message", entry.Message),
	}
	if entry.SessionID != "" {
		fields = append(fields, zap.String("session_id", entry.SessionID))
	}
	if entry.StepID != "" {
		fields = append(fields, zap.String("step_id", entry.StepID))
	}
	if entry.TraceID != "" {
		fields = append(fields, zap.String("trace_id", entry.TraceID))
	}
	if entry.RunID != "" {
		fields = append(fields, zap.String("run_id", entry.RunID))
	}
	if entry.Phase != "" {
		fields = append(fields, zap.String("phase", entry.Phase))
	}
	if entry.Severity != "" {
		fields = append(fields, zap.String("severity", entry.Severity))
	}
	if entry.DurationMS != 0 {
		fields = append(fields, zap.Int64("duration_ms", entry.DurationMS))
	}
	if entry.SpanID != "" {
		fields = append(fields, zap.String("span_id", entry.SpanID))
	}
	if entry.Fields != nil {
		fields = append(fields, zap.Any("fields", entry.Fields))
	}

	// Encode the entry using zapcore
	buf, err := s.encoder.EncodeEntry(zapcore.Entry{
		Level:   zapcore.DebugLevel,
		Time:    entry.Timestamp,
		Message: entry.Message,
	}, fields)
	if err != nil {
		// BLK-2 fix: no longer silently swallow encoding errors
		s.dropped.Add(1)
		fmt.Fprintf(os.Stderr, "[file_sink] encode error: %v (total_dropped=%d)\n", err, s.dropped.Load())
		return
	}
	if _, err := s.lj.Write(buf.Bytes()); err != nil {
		s.dropped.Add(1)
	}
	buf.Free()
}

func (s *FileSink) Flush() {}

func (s *FileSink) Close() error {
	return s.lj.Close()
}

func (s *FileSink) Dropped() uint64 {
	return s.dropped.Load()
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

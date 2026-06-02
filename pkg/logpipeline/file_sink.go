package logpipeline

import (
	"encoding/json"
	"os"
	"path/filepath"
	"runtime"

	"gopkg.in/natefinch/lumberjack.v2"
)

type FileSinkConfig struct {
	OutputDir   string
	MaxSizeMB   int
	MaxBackups  int
	MaxAgeDays  int
	Compress    bool
	Filename    string
}

type FileSink struct {
	lj      *lumberjack.Logger
	dropped uint64
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
	lj := &lumberjack.Logger{
		Filename:   filepath.Join(outputDir, filename),
		MaxSize:    maxSize,
		MaxBackups: maxBackups,
		MaxAge:     maxAge,
		Compress:   cfg.Compress,
		LocalTime:  true,
	}
	return &FileSink{lj: lj}
}

func (s *FileSink) Write(entry LogEntry) {
	data, err := json.Marshal(entry)
	if err != nil {
		return
	}
	if _, err := s.lj.Write(append(data, '\n')); err != nil {
		s.dropped++
	}
}

func (s *FileSink) Flush() {}

func (s *FileSink) Close() error {
	return s.lj.Close()
}

func (s *FileSink) Dropped() uint64 {
	return s.dropped
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

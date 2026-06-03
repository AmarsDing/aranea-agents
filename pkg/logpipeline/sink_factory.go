package logpipeline

import (
	"fmt"
	"strconv"
	"strings"
)

// SinkConfig is a pkg-level configuration struct for creating Sinks.
// It decouples the factory from internal/conf proto types.
type SinkConfig struct {
	Name       string
	Type       string // "file", "stdout", "eventbus"
	BufferSize int
	DropPolicy DropPolicy
	Config     map[string]string
}

// SinkFactoryDeps holds external dependencies needed by the sink factory.
// EventBus sinks require a Publisher which cannot be derived from config alone.
type SinkFactoryDeps struct {
	EventBusPublisher Publisher
}

// NewSinkFromConfig creates a Sink based on SinkConfig.
// For "eventbus" type, deps.EventBusPublisher must be non-nil.
func NewSinkFromConfig(cfg SinkConfig, deps SinkFactoryDeps) (Sink, error) {
	switch strings.ToLower(cfg.Type) {
	case "file":
		return newFileSinkFromConfig(cfg)
	case "stdout":
		return newStdoutSinkFromConfig(cfg)
	case "eventbus":
		return newEventBusSinkFromConfig(cfg, deps)
	default:
		return nil, fmt.Errorf("logpipeline: unknown sink type %q", cfg.Type)
	}
}

func newFileSinkFromConfig(cfg SinkConfig) (*FileSink, error) {
	fileCfg := FileSinkConfig{
		OutputDir:  cfg.Config["output_dir"],
		Filename:   cfg.Config["filename"],
		Compress:   cfg.Config["compress"] == "true",
	}
	if v, ok := cfg.Config["max_size_mb"]; ok {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			fileCfg.MaxSizeMB = n
		}
	}
	if v, ok := cfg.Config["max_backups"]; ok {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			fileCfg.MaxBackups = n
		}
	}
	if v, ok := cfg.Config["max_age_days"]; ok {
		if n, err := strconv.Atoi(v); err == nil && n > 0 {
			fileCfg.MaxAgeDays = n
		}
	}
	return NewFileSink(fileCfg), nil
}

func newStdoutSinkFromConfig(cfg SinkConfig) (*StdoutSink, error) {
	level := cfg.Config["level"]
	if level == "" {
		level = "debug"
	}
	return NewStdoutSink(level), nil
}

func newEventBusSinkFromConfig(cfg SinkConfig, deps SinkFactoryDeps) (*EventBusSink, error) {
	if deps.EventBusPublisher == nil {
		return nil, fmt.Errorf("logpipeline: eventbus sink requires EventBusPublisher")
	}
	hookLevel := cfg.Config["hook_level"]
	if hookLevel == "" {
		hookLevel = "info"
	}
	return NewEventBusSink(deps.EventBusPublisher, hookLevel), nil
}

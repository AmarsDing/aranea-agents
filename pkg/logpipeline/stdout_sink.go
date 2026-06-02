package logpipeline

import (
	"encoding/json"
	"os"
	"strings"
)

type StdoutSink struct {
	level string
}

func NewStdoutSink(level string) *StdoutSink {
	return &StdoutSink{level: strings.ToLower(level)}
}

func (s *StdoutSink) Write(entry LogEntry) {
	if !s.levelAllowed(entry.Level) {
		return
	}
	data, err := json.Marshal(entry)
	if err != nil {
		return
	}
	os.Stdout.Write(append(data, '\n'))
}

func (s *StdoutSink) Flush() {}

func (s *StdoutSink) Close() error {
	return nil
}

func (s *StdoutSink) levelAllowed(entryLevel string) bool {
	el := strings.ToLower(entryLevel)
	switch s.level {
	case "error":
		return el == "error"
	case "warn":
		return el == "error" || el == "warn"
	case "info":
		return el == "error" || el == "warn" || el == "info"
	default:
		return true
	}
}

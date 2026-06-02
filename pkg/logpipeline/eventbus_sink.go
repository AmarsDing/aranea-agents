package logpipeline

import (
	"context"
	"time"
)

type Publisher interface {
	Publish(ctx context.Context, kind EntryKind, level, message, sessionID string, fields map[string]any)
}

type EventBusSink struct {
	pub     Publisher
	level   logLevel
	author  string
	channel string
}

func NewEventBusSink(pub Publisher, hookLevel string) *EventBusSink {
	return &EventBusSink{
		pub:     pub,
		level:   parseLogLevel(hookLevel),
		author:  "system",
		channel: "monitor",
	}
}

func (s *EventBusSink) Write(entry LogEntry) {
	if s == nil || s.pub == nil {
		return
	}
	entryLevel := parseLogLevel(entry.Level)
	if entryLevel < s.level {
		return
	}
	ctx, cancel := context.WithTimeout(context.Background(), 50*time.Millisecond)
	defer cancel()
	s.pub.Publish(ctx, entry.Kind, entry.Level, entry.Message, entry.SessionID, entry.Fields)
}

func (s *EventBusSink) Flush() {}

func (s *EventBusSink) Close() error { return nil }

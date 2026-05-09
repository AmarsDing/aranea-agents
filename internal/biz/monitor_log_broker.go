package biz

import (
	"context"
	"sync"
	"time"

	"github.com/google/uuid"
)

// MonitorLogLine is the JSON shape pushed on monitor SSE event "log" (see internal/server/sse.go).
type MonitorLogLine struct {
	ID        string `json:"id"`
	Time      string `json:"time"`
	Level     string `json:"level"`
	Message   string `json:"message"`
	Source    string `json:"source"`
	CreatedAt string `json:"created_at"`
}

// MonitorLogBroker fans out human-readable operational lines to the monitor Logs SSE stream.
// The SSE server registers the publisher when enabled; otherwise Publish is a no-op.
type MonitorLogBroker struct {
	mu  sync.RWMutex
	pub func(context.Context, MonitorLogLine)
}

func NewMonitorLogBroker() *MonitorLogBroker {
	return &MonitorLogBroker{}
}

// SetPublisher installs the sink (typically tx7do SSE). Safe to call once at startup.
func (b *MonitorLogBroker) SetPublisher(fn func(context.Context, MonitorLogLine)) {
	if b == nil {
		return
	}
	b.mu.Lock()
	b.pub = fn
	b.mu.Unlock()
}

// Publish enqueues a monitor log line when a publisher is configured.
func (b *MonitorLogBroker) Publish(ctx context.Context, level, message, source string) {
	b.PublishWithID(ctx, "", level, message, source)
}

// PublishWithID like Publish but preserves a stable id (e.g. for correlating retries).
func (b *MonitorLogBroker) PublishWithID(ctx context.Context, id, level, message, source string) {
	if b == nil {
		return
	}
	b.mu.RLock()
	fn := b.pub
	b.mu.RUnlock()
	if fn == nil {
		return
	}
	if ctx == nil {
		ctx = context.Background()
	}
	now := time.Now().UTC().Format(time.RFC3339)
	if id == "" {
		id = uuid.NewString()
	}
	fn(ctx, MonitorLogLine{
		ID:        id,
		Time:      now,
		Level:     level,
		Message:   message,
		Source:    source,
		CreatedAt: now,
	})
}

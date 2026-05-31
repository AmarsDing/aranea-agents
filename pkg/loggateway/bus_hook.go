package loggateway

import (
	"sync"
	"time"

	"go.uber.org/zap"
	"go.uber.org/zap/zapcore"
)

type busHook struct {
	mu        sync.RWMutex
	hookLevel zapcore.Level
	publish   func(env EnvelopeLog)
}

func (h *busHook) setPublisher(fn func(env EnvelopeLog)) {
	h.mu.Lock()
	h.publish = fn
	h.mu.Unlock()
}

func (h *busHook) setLevel(level zapcore.Level) {
	h.mu.Lock()
	h.hookLevel = level
	h.mu.Unlock()
}

func (h *busHook) onWrite(entry zapcore.Entry, fields []zap.Field) {
	h.mu.RLock()
	pub := h.publish
	threshold := h.hookLevel
	h.mu.RUnlock()

	if pub == nil || entry.Level < threshold {
		return
	}

	meta := make(map[string]interface{}, len(fields)+2)
	for _, f := range fields {
		meta[f.Key] = f.Interface
	}
	meta["level"] = entry.Level.String()
	meta["ts"] = entry.Time.Format(time.RFC3339Nano)

	pub(EnvelopeLog{
		Level:     entry.Level.String(),
		Message:   entry.Message,
		Fields:    meta,
		Timestamp: entry.Time,
	})
}

type hookedCore struct {
	zapcore.Core
	hook *busHook
}

func (c *hookedCore) With(fields []zap.Field) zapcore.Core {
	return &hookedCore{
		Core: c.Core.With(fields),
		hook: c.hook,
	}
}

func (c *hookedCore) Check(entry zapcore.Entry, ce *zapcore.CheckedEntry) *zapcore.CheckedEntry {
	if c.Enabled(entry.Level) {
		return ce.AddCore(entry, c)
	}
	return ce
}

func (c *hookedCore) Write(entry zapcore.Entry, fields []zap.Field) error {
	func() {
		defer func() { recover() }()
		c.hook.onWrite(entry, fields)
	}()
	return c.Core.Write(entry, fields)
}

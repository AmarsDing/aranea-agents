package event

import (
	"context"
	"log/slog"
	"os"
	"runtime"
	"strings"
	"sync"
)

type SlogBridge struct {
	bus    Bus
	inner  slog.Handler
	mu     sync.Mutex
	attrs  []slog.Attr
	groups []string
	level  slog.Level
}

func NewSlogBridge(bus Bus, inner slog.Handler) *SlogBridge {
	level := slog.LevelInfo
	if v := strings.TrimSpace(os.Getenv("LOG_BRIDGE_LEVEL")); v != "" {
		var l slog.Level
		if err := l.UnmarshalText([]byte(v)); err == nil {
			level = l
		}
	}
	return &SlogBridge{
		bus:   bus,
		inner: inner,
		level: level,
	}
}

func (b *SlogBridge) Enabled(ctx context.Context, level slog.Level) bool {
	return level >= b.level
}

func (b *SlogBridge) Handle(ctx context.Context, r slog.Record) error {
	if b.bus != nil && r.Level >= b.level {
		env := NewEnvelope(EnvelopeTypeLog, "system", "")
		env.Channel = "monitor"
		env.Metadata = map[string]any{
			"level":  r.Level.String(),
			"source": sourceFromRecord(r),
		}
		if len(b.groups) > 0 {
			env.Metadata["group"] = strings.Join(b.groups, ".")
		}
		var msg strings.Builder
		msg.WriteString(r.Message)
		r.Attrs(func(a slog.Attr) bool {
			msg.WriteByte(' ')
			msg.WriteString(a.Key)
			msg.WriteByte('=')
			msg.WriteString(a.Value.String())
			return true
		})
		for _, a := range b.attrs {
			msg.WriteByte(' ')
			msg.WriteString(a.Key)
			msg.WriteByte('=')
			msg.WriteString(a.Value.String())
		}
		env.Content = &EnvelopeContent{
			Text:      msg.String(),
			IsPartial: false,
		}
		b.bus.Publish(ctx, env)
	}
	if b.inner != nil {
		return b.inner.Handle(ctx, r)
	}
	return nil
}

func (b *SlogBridge) WithAttrs(attrs []slog.Attr) slog.Handler {
	inner := b.inner
	if inner != nil {
		inner = inner.WithAttrs(attrs)
	}
	clone := &SlogBridge{
		bus:    b.bus,
		inner:  inner,
		level:  b.level,
		groups: make([]string, len(b.groups)),
	}
	copy(clone.groups, b.groups)
	clone.attrs = make([]slog.Attr, len(b.attrs), len(b.attrs)+len(attrs))
	copy(clone.attrs, b.attrs)
	clone.attrs = append(clone.attrs, attrs...)
	return clone
}

func (b *SlogBridge) WithGroup(name string) slog.Handler {
	inner := b.inner
	if inner != nil {
		inner = inner.WithGroup(name)
	}
	clone := &SlogBridge{
		bus:    b.bus,
		inner:  inner,
		level:  b.level,
		attrs:  make([]slog.Attr, len(b.attrs)),
		groups: make([]string, len(b.groups), len(b.groups)+1),
	}
	copy(clone.attrs, b.attrs)
	copy(clone.groups, b.groups)
	clone.groups = append(clone.groups, name)
	return clone
}

func sourceFromRecord(r slog.Record) string {
	if r.PC == 0 {
		return ""
	}
	fs := runtime.CallersFrames([]uintptr{r.PC})
	f, _ := fs.Next()
	if f.Function == "" {
		return ""
	}
	return f.Function
}

func InstallSlogBridge(bus Bus) bool {
	if strings.TrimSpace(os.Getenv("LOG_BRIDGE_ENABLED")) != "1" {
		return false
	}
	current := slog.Default()
	bridge := NewSlogBridge(bus, current.Handler())
	slog.SetDefault(slog.New(bridge))
	return true
}

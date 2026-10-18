package adapter

import (
	"context"
	"log/slog"
	"time"

	"aranea-agents/pkg/loggateway"
)

// SlogBridge implements slog.Handler by forwarding records into a
// loggateway.Logger. It exists so vendored framework code that logs via
// log/slog (e.g. pkg/trpc-agent-go/knowledge/source, which has no injectable
// logger) lands in the loggateway Pipeline — file sink, sanitizing and level
// control included — instead of the bare default stderr handler
// (2026-10-18 observability audit P3-1).
//
// Install once at startup: slog.SetDefault(slog.New(NewSlogBridge(gw))).
// The bridge never logs through slog itself, so there is no recursion.
type SlogBridge struct {
	lg     loggateway.Logger
	attrs  []loggateway.Field
	prefix string // flattened WithGroup path ("a.b."), applied to attr keys
}

// NewSlogBridge creates a SlogBridge forwarding to lg.
func NewSlogBridge(lg loggateway.Logger) *SlogBridge {
	return &SlogBridge{lg: lg}
}

// Enabled implements slog.Handler. Level gating is delegated to the pipeline
// sinks so slog records follow the same policy as all other logs.
func (b *SlogBridge) Enabled(_ context.Context, _ slog.Level) bool {
	return b != nil && b.lg != nil
}

// Handle implements slog.Handler, mapping slog levels onto the gateway and
// converting record attrs to structured fields.
func (b *SlogBridge) Handle(_ context.Context, r slog.Record) error {
	if b == nil || b.lg == nil {
		return nil
	}
	fields := make([]loggateway.Field, 0, len(b.attrs)+r.NumAttrs())
	fields = append(fields, b.attrs...)
	r.Attrs(func(a slog.Attr) bool {
		fields = append(fields, slogAttrToField(b.prefix, a))
		return true
	})
	msg := r.Message
	switch {
	case r.Level >= slog.LevelError:
		b.lg.Error(msg, fields...)
	case r.Level >= slog.LevelWarn:
		b.lg.Warn(msg, fields...)
	case r.Level >= slog.LevelInfo:
		b.lg.Info(msg, fields...)
	default:
		b.lg.Debug(msg, fields...)
	}
	return nil
}

// WithAttrs implements slog.Handler.
func (b *SlogBridge) WithAttrs(attrs []slog.Attr) slog.Handler {
	out := *b
	out.attrs = make([]loggateway.Field, 0, len(b.attrs)+len(attrs))
	out.attrs = append(out.attrs, b.attrs...)
	for _, a := range attrs {
		out.attrs = append(out.attrs, slogAttrToField(b.prefix, a))
	}
	return &out
}

// WithGroup implements slog.Handler. Groups are flattened into the field key
// ("group.key") since loggateway fields are a flat list.
func (b *SlogBridge) WithGroup(name string) slog.Handler {
	out := *b
	if name != "" {
		out.prefix = b.prefix + name + "."
	}
	return &out
}

func slogAttrToField(prefix string, a slog.Attr) loggateway.Field {
	a.Value = a.Value.Resolve()
	key := prefix + a.Key
	switch a.Value.Kind() {
	case slog.KindString:
		return loggateway.Str(key, a.Value.String())
	case slog.KindInt64:
		return loggateway.Int64(key, a.Value.Int64())
	case slog.KindUint64:
		return loggateway.Int64(key, int64(a.Value.Uint64()))
	case slog.KindFloat64:
		return loggateway.Float64(key, a.Value.Float64())
	case slog.KindBool:
		return loggateway.Bool(key, a.Value.Bool())
	case slog.KindDuration:
		return loggateway.Int64(key+"_ms", a.Value.Duration().Milliseconds())
	case slog.KindTime:
		return loggateway.Str(key, a.Value.Time().UTC().Format(time.RFC3339Nano))
	case slog.KindGroup:
		// Nested groups flatten one level deeper and must stay a single field
		// list entry per attr — handled by returning the whole group as Any.
		return loggateway.Any(key, a.Value.Group())
	case slog.KindLogValuer, slog.KindAny:
		fallthrough
	default:
		if err, ok := a.Value.Any().(error); ok {
			return loggateway.Err(err)
		}
		return loggateway.Any(key, a.Value.Any())
	}
}

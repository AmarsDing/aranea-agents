package plugintrpc

import (
	"context"
	"fmt"
	"strings"

	"aranea-agents/internal/event"
	"aranea-agents/pkg/safego"
)

var hookLogger = NewPluginSafeLogger("hook", nil)

func InitHookLogger(bus event.Bus) {
	hookLogger = NewPluginSafeLogger("hook", bus)
}

type PluginSafeLogger struct {
	pluginName string
	bus        event.Bus
}

func NewPluginSafeLogger(pluginName string, bus event.Bus) *PluginSafeLogger {
	return &PluginSafeLogger{pluginName: pluginName, bus: bus}
}

func (l *PluginSafeLogger) Info(msg string, attrs ...any) {
	l.write("INFO", msg, attrs...)
}

func (l *PluginSafeLogger) Warn(msg string, attrs ...any) {
	l.write("WARN", msg, attrs...)
}

func (l *PluginSafeLogger) Error(msg string, attrs ...any) {
	l.write("ERROR", msg, attrs...)
}

func (l *PluginSafeLogger) Debug(msg string, attrs ...any) {
	l.write("DEBUG", msg, attrs...)
}

func (l *PluginSafeLogger) write(level, msg string, attrs ...any) {
	pairs := make([]event.Pair, 0, len(attrs)/2)
	for i := 0; i+1 < len(attrs); i += 2 {
		key, _ := attrs[i].(string)
		if key != "" {
			pairs = append(pairs, event.P(key, attrs[i+1]))
		}
	}

	stepID := "plugin." + l.pluginName
	switch level {
	case "ERROR":
		event.SysLogError(stepID, msg, pairs...)
	case "WARN":
		event.SysLogWarn(stepID, msg, pairs...)
	default:
		event.SysLogInfo(stepID, msg, pairs...)
	}

	if l.bus != nil {
		var buf strings.Builder
		buf.WriteString("[")
		buf.WriteString(level)
		buf.WriteString("][")
		buf.WriteString(l.pluginName)
		buf.WriteString("] ")
		buf.WriteString(msg)
		for i := 0; i+1 < len(attrs); i += 2 {
			k, _ := attrs[i].(string)
			buf.WriteString(" ")
			buf.WriteString(k)
			buf.WriteString("=")
			fmt.Fprintf(&buf, "%v", attrs[i+1])
		}
		text := buf.String()
		env := event.NewEnvelope(event.EnvelopeTypeLog, l.pluginName, "")
		env.Channel = "monitor"
		env.Metadata = map[string]any{"level": level, "source": l.pluginName}
		env.Content = &event.EnvelopeContent{Text: text, IsPartial: false}
		bus := l.bus
		safego.Go(context.Background(), "plugin-log-"+l.pluginName, func() {
			bus.Publish(context.Background(), env)
		})
	}
}

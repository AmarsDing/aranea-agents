package plugintrpc

import (
	"context"
	"fmt"
	"strings"
	"sync"

	"aranea-agents/internal/event"
	"aranea-agents/pkg/loggateway"
	"aranea-agents/pkg/safego"
)

var (
	hookLoggerMu sync.RWMutex
	hookLogger   *PluginSafeLogger
)

func InitHookLogger(bus event.Bus, lg loggateway.Logger) {
	hookLoggerMu.Lock()
	hookLogger = NewPluginSafeLogger("hook", bus, lg)
	hookLoggerMu.Unlock()
}

func getHookLogger() *PluginSafeLogger {
	hookLoggerMu.RLock()
	l := hookLogger
	hookLoggerMu.RUnlock()
	return l
}

type PluginSafeLogger struct {
	pluginName string
	bus        event.Bus
	lg         loggateway.Logger
}

func NewPluginSafeLogger(pluginName string, bus event.Bus, lg loggateway.Logger) *PluginSafeLogger {
	return &PluginSafeLogger{pluginName: pluginName, bus: bus, lg: lg}
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
	stepID := "plugin." + l.pluginName
	allFields := make([]loggateway.Field, 0, 1+len(attrs)/2)
	allFields = append(allFields, loggateway.StepID(stepID))
	for i := 0; i+1 < len(attrs); i += 2 {
		key, _ := attrs[i].(string)
		if key != "" {
			allFields = append(allFields, loggateway.Any(key, attrs[i+1]))
		}
	}

	switch level {
	case "ERROR":
		l.lg.Error(msg, allFields...)
	case "WARN":
		l.lg.Warn(msg, allFields...)
	case "DEBUG":
		l.lg.Debug(msg, allFields...)
	default:
		l.lg.Info(msg, allFields...)
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
